// Package game is the Ebitengine presentation layer: camera, input, tools and
// drawing. Street rules live in street, vehicle movement in traffic.
package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/internal/platforminput"
	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// page is one of the three screens, as in 2d-bus-game: building track
// and stops, planning lines, and running the service.
type page int

const (
	pageBuild page = iota
	pagePlan
	pageRun
)

func pageNames() []string {
	return locale.L([]string{"建设", "规划", "运行"}, []string{"Build", "Plan", "Run"})
}

type tool int

const (
	toolTrack tool = iota
	toolStop
	toolCrossover
	toolRemove
	toolRoad
)

func toolNames() []string {
	return locale.L([]string{"轨道", "车站", "渡线", "拆除", "道路"}, []string{"Track", "Stop", "Crossover", "Remove", "Road"})
}

// layouts are the track options the Track tool cycles through with M.
var layouts = []struct {
	zh, en string
	mode   street.TramMode
	single bool
}{
	{"双线 · 独立路权", "Double track · own right of way", street.Dedicated, false},
	{"双线 · 混行", "Double track · shared with cars", street.Shared, false},
	{"单线 · 往返（拉风箱）", "Single track · back and forth", street.Dedicated, true},
}

func layoutName(i int) string { return locale.T(layouts[i].zh, layouts[i].en) }

// levels are the city's five vertical levels, top to bottom (see
// levels.go). Only the ground can be built on today.
var levels = []sim.Level{sim.HighElevated, sim.Elevated, sim.Ground, sim.Underground, sim.DeepUnderground}

func levelNames() []string {
	return locale.L([]string{"超高层", "高架层", "地面层", "地下层", "深地下层"}, []string{"High level", "Elevated", "Ground", "Underground", "Deep level"})
}

const groundLevel = 2 // index of the ground in levels

var speeds = []int{1, 2, 4, 8, 16}

// densities are the numbers of AI cars to choose from.
var densities = []int{200, 500, 1000, 2000, 3000}

const defaultDensity = 1

// passNames() name traffic.PassengerRates, as in the bus game.
func passNames() []string {
	return locale.L([]string{"偏少", "适中", "偏多"}, []string{"Few", "Moderate", "Many"})
}

var (
	colAccent  = rgb(84, 186, 204)
	colOK      = color.RGBA{0x2f, 0xa8, 0x5a, 0xb0}
	colBad     = color.RGBA{0xd9, 0x3b, 0x3b, 0xd0}
	colHover   = color.RGBA{0xf2, 0x8c, 0x28, 0xff}
	colHUD     = color.RGBA{0x1b, 0x1f, 0x27, 0xe0}
	colOutside = color.RGBA{0x2a, 0x2e, 0x35, 0xff}
	colRed     = color.RGBA{0xe0, 0x30, 0x30, 0xff}
	colAmber   = color.RGBA{0xf0, 0xa8, 0x20, 0xff}
	colGreen   = color.RGBA{0x30, 0xc0, 0x60, 0xff}
	colPlat    = color.RGBA{0xf4, 0xf4, 0xf0, 0xff}
	colStopRim = color.RGBA{0x22, 0x26, 0x2e, 0xff}
	lineColors = []color.RGBA{
		{0xd6, 0x2f, 0x45, 0xff},
		{0x1f, 0x78, 0xd1, 0xff},
		{0x2e, 0x9d, 0x55, 0xff},
		{0x8e, 0x4f, 0xc9, 0xff},
		{0xe0, 0x82, 0x1e, 0xff},
		{0x13, 0x9c, 0xa8, 0xff},
	}
	carColors = []color.RGBA{
		{0xe8, 0xe8, 0xe8, 0xff}, {0x30, 0x34, 0x3a, 0xff}, {0xb8, 0x3a, 0x34, 0xff},
		{0x3a, 0x6e, 0xb8, 0xff}, {0x9a, 0x9e, 0xa4, 0xff}, {0xd8, 0xb0, 0x40, 0xff},
		{0x4a, 0x8a, 0x5c, 0xff}, {0xf0, 0xf0, 0xf4, 0xff},
	}
)

type Game struct {
	city  *city.Map // generated grid city (nil on a real map)
	scene *scene    // real-world map (nil on the grid city)
	mapW  float64
	mapH  float64
	net   *street.Network
	sim   *traffic.Sim
	// freshNet builds the map's own street network, before any edits.
	freshNet func() *street.Network

	page                page
	tool                tool
	dirty               bool // edited since the simulation was last (re)started
	selLine             street.LineID
	follow              int  // ID of the tram the camera follows (0 = none)
	toMenu              bool // Esc pressed with nothing to cancel
	routes              map[street.LineID][][]street.Vec
	cycles              map[street.LineID]lineCycle
	legend              []legendItem
	cfgBtns             []cfgButton
	cfgPanel            rect
	key                 string // map key, names the save file
	lastSave            []byte // contents last written (or loaded), to skip unchanged saves
	saveTick            int
	autoLast            []byte    // contents of the last autosave
	autoAt              time.Time // when autosaves were last considered
	slots               *slotPanel
	noSave              bool // demo / screenshot runs never write saves
	hudBtns             []hudButton
	drop                *dropMenu       // open drop-down list on the top bar
	treeEdits           int             // road edits the trees were last placed for
	treeNet             *street.Network // and the network
	sound               soundState
	soundPanel          bool // the volume panel is open
	soundBtn, soundRect rect
	lastAnnouncement    string     // the last announcement made (shown and tested)
	lines               *linePanel   // line management window, nil when closed
	modelMenu           *modelPicker // open tram model selector, nil when closed
	rename              *renameBox   // landmark rename window, nil when closed
	lmLabels            []landmarkLabel
	stopLabels          []stopLabel // stop name labels drawn this frame
	// landmarkNames are the player's names for landmarks, by OSM name.
	landmarkNames map[string]string
	shadowImg     *ebiten.Image // building shadow layer (Seattle)
	layout        int           // index into layouts
	platform      int           // sections new stops' platforms hold
	level         int
	densityIdx    int
	passIdx       int               // index into traffic.PassengerRates
	start         street.JunctionID // first junction of a track run, -1 if none
	anchor        *trackAnchor      // active start point & level for point-to-point track building
	roadMode      int               // Road tool: build, demolish or lanes
	roadLanes     int               // index into roadLaneOpts
	roadStart     *street.Vec       // first point of a new street
	pending       []street.StopID   // stops picked for a new line

	paused   bool
	speedIdx int

	camX, camY, zoom float64
	w, h             int
	pressed          bool // a mouse/trackpad button is down
	pressBtn         ebiten.MouseButton
	pressX, pressY   int
	dragX, dragY     int
	panning          bool // the press moved far enough to be a drag
	click            bool // a left click completed this frame (on release)

	cursor  street.Vec
	message string
	msgTTL  int
}

// New starts a game on a map: "seattle" (central Seattle
// from OpenStreetMap) or "grid" (the generated test city).
func New(mapName string) *Game {
	if mapName != "grid" {
		return newOnMap(mapName, mapdata.Seattle())
	}
	g := newGame(mapName)
	g.city = city.Generate(1)
	g.freshNet = func() *street.Network { return street.Generate(g.city) }
	g.net = g.freshNet()
	g.mapW, g.mapH = g.city.SizeMetres()
	return g.ready()
}

// newOnMap starts a game on a real-world map.
func newOnMap(key string, m *mapdata.Map) *Game {
	g := newGame(key)
	g.scene = newScene(m)
	g.freshNet = func() *street.Network { return street.FromMap(m) }
	g.net = g.freshNet()
	g.scene.joinStreets(g.net)
	g.roadsChanged()
	g.mapW, g.mapH = m.W, m.H
	return g.ready()
}

func newGame(key string) *Game {
	return &Game{cfgPanel: rect{-1, -1, -1, -1}, w: 1280, h: 720, start: -1, roadLanes: defaultRoadLanes, level: groundLevel, densityIdx: defaultDensity, passIdx: traffic.DefaultPassengerRate, selLine: -1, platform: street.DefaultPlatform, key: key}
}

func (g *Game) ready() *Game {
	g.attachCorridorCheck()
	g.camX, g.camY = g.mapW/2, g.mapH/2
	g.zoom = 1.0
	g.rebuild()
	return g
}

// SetCamera centres the view on (x, y) metres at zoom pixels per metre.
func (g *Game) SetCamera(x, y, zoom float64) { g.camX, g.camY, g.zoom = x, y, zoom }

// Warp advances the simulation by the given simulated seconds instantly.
func (g *Game) Warp(seconds float64) {
	for i := 0; i < int(seconds/traffic.DT); i++ {
		g.sim.Step()
	}
}

// rebuild restarts traffic on the edited network.
func (g *Game) rebuild() {
	g.sim = traffic.New(g.net, densities[g.densityIdx], 1)
	g.sim.PassengerRate = traffic.PassengerRates[g.passIdx]
	g.dirty, g.routes, g.cycles, g.follow = false, nil, nil, 0
}

// edited marks the network changed; the service restarts on the Run page.
func (g *Game) edited() {
	g.dirty, g.routes, g.cycles = true, nil, nil
	g.roadsChanged()
}

// roadsChanged moves the street trees for edited roads.
func (g *Game) roadsChanged() {
	if g.scene != nil && (g.treeNet != g.net || len(g.net.Edits()) != g.treeEdits) {
		g.treeNet, g.treeEdits = g.net, len(g.net.Edits())
		g.scene.placeTrees(g.net)
	}
}

func (g *Game) say(format string, args ...any) {
	g.message = fmt.Sprintf(format, args...)
	g.msgTTL = 60 * 4
}

func (g *Game) Layout(outW, outH int) (int, int) {
	g.w, g.h = outW, outH
	return outW, outH
}

func (g *Game) toScreen(p street.Vec) (float32, float32) {
	return float32((p.X-g.camX)*g.zoom + float64(g.w)/2), float32((p.Y-g.camY)*g.zoom + float64(g.h)/2)
}

func (g *Game) toWorld(x, y int) street.Vec {
	return street.Vec{X: (float64(x)-float64(g.w)/2)/g.zoom + g.camX, Y: (float64(y)-float64(g.h)/2)/g.zoom + g.camY}
}

func (g *Game) pick(px float64) float64 { return px / g.zoom }

// --- update -----------------------------------------------------------------

func (g *Game) Update() error {
	if g.slots != nil {
		g.updateSlots()
		return nil
	}
	if g.rename != nil {
		g.updateRename()
		g.stepSim()
		return nil
	}
	if g.clickHUD() {
		return nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyF5):
		g.openSlots(true)
		return nil
	case inpututil.IsKeyJustPressed(ebiten.KeyF9):
		g.openSlots(false)
		return nil
	case inpututil.IsKeyJustPressed(ebiten.KeyT):
		g.toggleLinePanel()
		return nil
	case inpututil.IsKeyJustPressed(ebiten.KeyV):
		g.soundPanel = !g.soundPanel
		return nil
	}
	if g.lines != nil {
		g.updateLinePanel()
		g.stepSim()
		g.updateFollow()
		return nil
	}
	g.updateCamera()
	mx, my := ebiten.CursorPosition()
	g.cursor = g.toWorld(mx, my)
	g.updateKeys()
	if my > hudHeight {
		g.updateTool()
	}
	if g.msgTTL > 0 {
		g.msgTTL--
	}
	g.stepSim()
	g.updateFollow()
	g.updateSound()
	return nil
}

// stepSim runs the service on the Run page.
func (g *Game) stepSim() {
	if g.page == pageRun && !g.paused {
		for i := 0; i < speeds[g.speedIdx]; i++ {
			g.sim.Step()
		}
	}
}

func (g *Game) updateCamera() {
	pan := 600 / g.zoom / 60
	if ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl) {
		pan = 0 // Cmd/Ctrl shortcuts (Cmd+S saves) do not pan
	}
	for _, k := range []ebiten.Key{ebiten.KeyA, ebiten.KeyD, ebiten.KeyW, ebiten.KeyS, ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown} {
		if ebiten.IsKeyPressed(k) {
			g.follow = 0 // taking the camera back stops following
		}
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.camX -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.camX += pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.camY -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.camY += pan
	}
	mx, my := ebiten.CursorPosition()
	zoomAt := func(f float64) {
		zx, zy := mx, my
		if g.follow != 0 {
			zx, zy = g.w/2, g.h/2 // zoom around the followed tram at screen centre
		}
		before := g.toWorld(zx, zy)
		g.zoom = math.Max(minZoom, math.Min(g.maxZoom(), g.zoom*f))
		after := g.toWorld(zx, zy)
		g.camX += before.X - after.X
		g.camY += before.Y - after.Y
	}
	// Mac trackpad: pinch zooms; two-finger scrolling is ignored so it never
	// zooms by accident. A physical mouse wheel still zooms.
	if p := platforminput.TakePinchDelta(); p != 0 {
		zoomAt(math.Max(0.5, 1+p))
	}
	if wy := platforminput.TakeMouseWheelY(); wy != 0 {
		if !g.scrollModelMenu(wy) {
			zoomAt(math.Pow(1.15, wy))
		}
	}
	if g.follow == 0 && g.zoom > maxMapZoom {
		g.zoom = math.Max(maxMapZoom, g.zoom*0.88)
		if g.zoom-maxMapZoom < 0.05 {
			g.zoom = maxMapZoom
		}
	}

	// Press and drag (trackpad click or left/middle mouse button) pans the
	// map. A left press that barely moves counts as a click on release.
	g.click = false
	for _, b := range []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonMiddle} {
		if inpututil.IsMouseButtonJustPressed(b) && !g.pressed {
			g.pressed, g.pressBtn = true, b
			g.pressX, g.pressY, g.dragX, g.dragY = mx, my, mx, my
			g.panning = b == ebiten.MouseButtonMiddle
		}
	}
	if g.pressed {
		if !g.panning && math.Hypot(float64(mx-g.pressX), float64(my-g.pressY)) > dragThreshold {
			g.panning = true
		}
		if g.panning {
			g.follow = 0
			g.camX -= float64(mx-g.dragX) / g.zoom
			g.camY -= float64(my-g.dragY) / g.zoom
		}
		g.dragX, g.dragY = mx, my
		if inpututil.IsMouseButtonJustReleased(g.pressBtn) {
			g.click = g.pressBtn == ebiten.MouseButtonLeft && !g.panning && g.pressY > hudHeight
			g.pressed, g.panning = false, false
		}
	}
	g.camX = math.Max(0, math.Min(g.mapW, g.camX))
	g.camY = math.Max(0, math.Min(g.mapH, g.camY))
}

// --- draw -------------------------------------------------------------------

const (
	hudHeight     = 34
	minZoom       = 0.18
	maxMapZoom    = 10.0
	maxFollowZoom = 45.0
)

func (g *Game) maxZoom() float64 {
	if g.follow != 0 {
		return maxFollowZoom
	}
	return maxMapZoom
}

// dragThreshold is how far (px) a press must move to pan instead of click.
const dragThreshold = 5

func (g *Game) Draw(dst *ebiten.Image) {
	g.drawWorld(dst)
	g.drawLabels(dst)
	g.drawHUD(dst)
	g.drawAttribution(dst)
	g.drawLinePanel(dst)
	g.drawModelMenu(dst)
	g.drawRename(dst)
	g.drawSlots(dst, g.w, g.h)
}

// drawWorld draws the map, track, stops and (on the Run page) vehicles.
func (g *Game) drawWorld(dst *ebiten.Image) {
	c := newCanvas(dst, g)
	if g.scene != nil {
		dst.Fill(colUrban)
		g.drawSceneGround(c)
		g.drawHighways(c, 0)
		g.drawRealStreets(c)
		g.drawTunnels(c)
		g.drawUndergroundOnGround(c)
		g.drawHighways(c, 1)
		g.drawHighways(c, 2)
		g.drawGroundTramways(c)
		g.drawTrack(c)
		c.flush()
		g.drawSceneShadows(dst)
		g.drawStreetNames(dst) // under the trams and cars
		g.drawVehicles(c)
		g.drawPortalHeadwalls(c)
		g.drawSceneBuildings(c)
		// Platforms after the buildings: on a one-way street they stand on
		// the pavement, which mapped buildings often overhang.
		g.drawStops(c)
		g.drawSceneTrees(c)
	} else {
		dst.Fill(colGround)
		g.drawGround(c)
		g.drawLots(c)
		g.drawUndergroundOnGround(c)
		g.drawRealStreets(c)
		g.drawGroundTramways(c)
		g.drawTrack(c)
		g.drawStops(c)
		g.drawStreetTrees(c)
		c.flush()
		g.drawStreetNames(dst) // under the trams and cars
		g.drawVehicles(c)
		g.drawPortalHeadwalls(c)
	}
	g.drawSignals(c)
	g.drawTramSignals(c)
	g.drawElevatedLayer(c)
	g.drawUndergroundCutaway(c)
	if levelHeight(g.level) == 0 {
		g.drawToolOverlay(c)
	}
	c.flush()
	g.drawLevelView(dst)
}

func (g *Game) highlightRoads(c *canvas, roads []street.RoadID, col color.RGBA) {
	for _, r := range roads {
		a, b := g.net.Ends(r)
		c.seg(a, b, 2*g.net.Roads[r].HalfWidth(), 4, col)
	}
}

func (g *Game) ring(c *canvas, p street.Vec, r, widthPx float64, col color.RGBA) {
	n := 40
	w := widthPx / c.px()
	for i := 0; i < n; i++ {
		a0, a1 := 2*math.Pi*float64(i)/float64(n), 2*math.Pi*float64(i+1)/float64(n)
		c.seg(street.Vec{X: p.X + r*math.Cos(a0), Y: p.Y + r*math.Sin(a0)},
			street.Vec{X: p.X + r*math.Cos(a1), Y: p.Y + r*math.Sin(a1)}, w, 1, col)
	}
}

func (g *Game) lineIndex(id street.LineID) int {
	for i, l := range g.net.LineIDs() {
		if l == id {
			return i
		}
	}
	return 0
}
