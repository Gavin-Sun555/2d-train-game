// Package game is the Ebitengine presentation layer: camera, input, tools and
// drawing. Construction rules live in build, vehicle movement in rail/sim.
package game

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/build"
	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

type tool int

const (
	toolTrack tool = iota
	toolStop
	toolLine
	toolDelete
)

var toolNames = []string{"Track", "Stop", "Line", "Delete"}

var levels = []sim.Level{sim.Ground, sim.Elevated, sim.Underground}

var speeds = []int{1, 2, 4, 8, 16, 32}

var (
	colTrack    = color.RGBA{0x4b, 0x50, 0x59, 0xff}
	colSleeper  = color.RGBA{0x8a, 0x84, 0x7a, 0xff}
	colOK       = color.RGBA{0x2f, 0xa8, 0x5a, 0xff}
	colBad      = color.RGBA{0xd9, 0x3b, 0x3b, 0xff}
	colHover    = color.RGBA{0xf2, 0x8c, 0x28, 0xff}
	colStopFill = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colStopRing = color.RGBA{0x22, 0x26, 0x2e, 0xff}
	colHUD      = color.RGBA{0x1b, 0x1f, 0x27, 0xe0}
	colOutside  = color.RGBA{0x2a, 0x2e, 0x35, 0xff}
	colReserved = color.RGBA{0xe0, 0xa8, 0x2a, 0xff}
	colOccupied = color.RGBA{0xd9, 0x4a, 0x3d, 0xff}
	lineColors  = []color.RGBA{
		{0xd6, 0x2f, 0x45, 0xff},
		{0x1f, 0x78, 0xd1, 0xff},
		{0x2e, 0x9d, 0x55, 0xff},
		{0x8e, 0x4f, 0xc9, 0xff},
		{0xe0, 0x82, 0x1e, 0xff},
		{0x13, 0x9c, 0xa8, 0xff},
	}
)

type Game struct {
	city    *city.Map
	cityImg *ebiten.Image
	plan    *build.Plan

	// Derived from plan; rebuilt after every edit (vehicles restart).
	compiled *build.Compiled
	world    *sim.World
	status   []build.LineStatus

	tool       tool
	level      int
	drawing    bool
	anchor     sim.Vec2
	pending    []build.StopID // stops picked for a new line
	showBlocks bool

	paused   bool
	speedIdx int

	// Camera: world position at screen centre and pixels per metre.
	camX, camY, zoom float64
	w, h             int
	dragging         bool
	dragX, dragY     int

	cursor  sim.Vec2 // world position under the mouse
	message string
	msgTTL  int
}

func New() *Game {
	m := city.Generate(1)
	g := &Game{city: m, plan: build.NewPlan(m), w: 1280, h: 720}
	wm, hm := m.SizeMetres()
	g.camX, g.camY = wm/2, hm/2
	g.zoom = math.Min(1280/wm, 720/hm) * 1.6
	g.rebuild()
	return g
}

// Warp advances the simulation by the given simulated seconds instantly.
func (g *Game) Warp(seconds float64) {
	for i := 0; i < int(seconds/sim.DT); i++ {
		g.world.Step()
	}
}

func (g *Game) rebuild() {
	g.compiled = g.plan.Compile()
	g.world, g.status = g.compiled.World()
}

func (g *Game) say(format string, args ...any) {
	g.message = fmt.Sprintf(format, args...)
	g.msgTTL = 60 * 4
}

func (g *Game) Layout(outW, outH int) (int, int) {
	g.w, g.h = outW, outH
	return outW, outH
}

// --- coordinates ------------------------------------------------------------

func (g *Game) toScreen(p sim.Vec2) (float32, float32) {
	return float32((p.X-g.camX)*g.zoom + float64(g.w)/2), float32((p.Y-g.camY)*g.zoom + float64(g.h)/2)
}

func (g *Game) toWorld(x, y int) sim.Vec2 {
	return sim.Vec2{X: (float64(x)-float64(g.w)/2)/g.zoom + g.camX, Y: (float64(y)-float64(g.h)/2)/g.zoom + g.camY}
}

// pickRadius converts a screen-space pick tolerance into metres.
func (g *Game) pickRadius(px float64) float64 { return px / g.zoom }

func (g *Game) curLevel() sim.Level { return levels[g.level] }

// --- update -----------------------------------------------------------------

func (g *Game) Update() error {
	g.updateCamera()
	mx, my := ebiten.CursorPosition()
	g.cursor = g.toWorld(mx, my)
	g.updateKeys()
	if my > hudHeight { // clicks on the HUD bar do nothing
		g.updateTool()
	}
	if g.msgTTL > 0 {
		g.msgTTL--
	}
	if !g.paused {
		for i := 0; i < speeds[g.speedIdx]; i++ {
			g.world.Step()
		}
	}
	return nil
}

func (g *Game) updateCamera() {
	pan := 600 / g.zoom / 60
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
	if _, wy := ebiten.Wheel(); wy != 0 {
		before := g.toWorld(mx, my)
		g.zoom = math.Max(0.2, math.Min(8, g.zoom*math.Pow(1.15, wy)))
		after := g.toWorld(mx, my)
		g.camX += before.X - after.X
		g.camY += before.Y - after.Y
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		g.dragging, g.dragX, g.dragY = true, mx, my
	}
	if g.dragging {
		if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
			g.dragging = false
		} else {
			g.camX -= float64(mx-g.dragX) / g.zoom
			g.camY -= float64(my-g.dragY) / g.zoom
			g.dragX, g.dragY = mx, my
		}
	}
	wm, hm := g.city.SizeMetres()
	g.camX = math.Max(0, math.Min(wm, g.camX))
	g.camY = math.Max(0, math.Min(hm, g.camY))
}

func (g *Game) updateKeys() {
	for i := range toolNames {
		if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
			g.setTool(tool(i))
		}
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		next := (g.level + 1) % len(levels)
		if !build.Rules[levels[next]].Buildable {
			g.say("%s track is planned for a later version (metro / light rail)", levels[next])
			return
		}
		g.level = next
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		g.paused = !g.paused
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual), inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
		g.speedIdx = min(g.speedIdx+1, len(speeds)-1)
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus), inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
		g.speedIdx = max(g.speedIdx-1, 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyB):
		g.showBlocks = !g.showBlocks
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.drawing, g.pending = false, nil
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter) && g.tool == toolLine:
		g.commitLine()
	case inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && g.tool == toolLine:
		if ls := g.plan.Lines(); len(ls) > 0 && len(g.pending) == 0 {
			g.plan.DeleteLine(ls[len(ls)-1].ID)
			g.rebuild()
			g.say("removed the newest line")
		}
	}
}

func (g *Game) setTool(t tool) {
	g.tool, g.drawing, g.pending = t, false, nil
}

func (g *Game) updateTool() {
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	lvl := g.curLevel()
	switch g.tool {
	case toolTrack:
		if right {
			g.drawing = false
		}
		if !left {
			return
		}
		p := g.plan.Snap(g.cursor, lvl, g.pickRadius(12))
		if !g.drawing {
			g.drawing, g.anchor = true, p
			return
		}
		if _, err := g.plan.AddSegment(g.anchor, p, lvl); err != nil {
			g.say("Can't build: %v", err)
			return
		}
		g.anchor = p
		g.rebuild()
	case toolStop:
		if !left {
			return
		}
		if _, err := g.plan.AddStop(g.cursor, lvl, g.pickRadius(14)); err != nil {
			g.say("Can't place stop: %v", err)
			return
		}
		g.rebuild()
	case toolLine:
		if right && len(g.pending) > 0 {
			g.pending = g.pending[:len(g.pending)-1]
		}
		if !left {
			return
		}
		st, ok := g.plan.StopNear(g.cursor, g.pickRadius(14))
		if !ok {
			g.say("Click stops in order, then press Enter")
			return
		}
		if n := len(g.pending); n > 0 && g.pending[n-1] == st.ID {
			return
		}
		g.pending = append(g.pending, st.ID)
	case toolDelete:
		if !left {
			return
		}
		if s, _, ok := g.plan.NearestSegment(g.cursor, lvl, g.pickRadius(10)); ok {
			g.plan.DeleteSegment(s.ID)
			g.rebuild()
		}
	}
}

func (g *Game) commitLine() {
	if _, err := g.plan.AddLine(build.Tram, g.pending); err != nil {
		g.say("Can't create line: %v", err)
		return
	}
	g.pending = nil
	g.rebuild()
	g.say("Line created")
}

// --- draw -------------------------------------------------------------------

const hudHeight = 28

func (g *Game) Draw(dst *ebiten.Image) {
	if g.cityImg == nil {
		g.cityImg = renderCity(g.city)
	}
	dst.Fill(colOutside)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(1/float64(mapPxPerMetre), 1/float64(mapPxPerMetre))
	op.GeoM.Translate(-g.camX, -g.camY)
	op.GeoM.Scale(g.zoom, g.zoom)
	op.GeoM.Translate(float64(g.w)/2, float64(g.h)/2)
	dst.DrawImage(g.cityImg, op)

	g.drawTrack(dst)
	g.drawStops(dst)
	g.drawVehicles(dst)
	g.drawToolOverlay(dst)
	g.drawHUD(dst)
}

func (g *Game) metres(m, minPx float64) float32 { return float32(math.Max(m*g.zoom, minPx)) }

func (g *Game) drawTrack(dst *ebiten.Image) {
	net := g.compiled.Net
	for _, e := range net.Edges {
		x0, y0 := g.toScreen(net.Nodes[e.A].Pos)
		x1, y1 := g.toScreen(net.Nodes[e.B].Pos)
		vector.StrokeLine(dst, x0, y0, x1, y1, g.metres(3.2, 4), colSleeper, true)
		c := colTrack
		if g.showBlocks {
			switch g.world.BlockState(e.ID) {
			case sim.BlockReserved:
				c = colReserved
			case sim.BlockOccupied:
				c = colOccupied
			}
		}
		vector.StrokeLine(dst, x0, y0, x1, y1, g.metres(1.6, 2), c, true)
	}
	if g.tool == toolDelete {
		if s, _, ok := g.plan.NearestSegment(g.cursor, g.curLevel(), g.pickRadius(10)); ok {
			a, b := g.plan.SegEnds(s)
			x0, y0 := g.toScreen(a)
			x1, y1 := g.toScreen(b)
			vector.StrokeLine(dst, x0, y0, x1, y1, g.metres(4, 6), colBad, true)
		}
	}
}

func (g *Game) lineColor(id build.LineID) color.RGBA {
	for i, l := range g.plan.Lines() {
		if l.ID == id {
			return lineColors[i%len(lineColors)]
		}
	}
	return colTrack
}

func (g *Game) drawStops(dst *ebiten.Image) {
	for _, st := range g.plan.Stops() {
		x, y := g.toScreen(g.plan.Node(st.Node).Pos)
		r := g.metres(4, 5)
		vector.FillCircle(dst, x, y, r+2, colStopRing, true)
		vector.FillCircle(dst, x, y, r, colStopFill, true)
		if g.zoom > 0.5 {
			ebitenutil.DebugPrintAt(dst, st.Name, int(x)+int(r)+4, int(y)-8)
		}
	}
	for i, id := range g.pending {
		x, y := g.toScreen(g.plan.Node(g.plan.Stop(id).Node).Pos)
		vector.StrokeCircle(dst, x, y, 11, 3, colHover, true)
		ebitenutil.DebugPrintAt(dst, fmt.Sprint(i+1), int(x)-3, int(y)-26)
	}
}

func (g *Game) drawVehicles(dst *ebiten.Image) {
	for _, s := range g.status {
		if s.Train < 0 {
			continue
		}
		t := g.world.Trains[s.Train]
		pts := g.world.Body(t)
		c := g.lineColor(s.Line)
		for i := 0; i+1 < len(pts); i++ {
			x0, y0 := g.toScreen(pts[i])
			x1, y1 := g.toScreen(pts[i+1])
			vector.StrokeLine(dst, x0, y0, x1, y1, g.metres(2.6, 6), c, true)
		}
		if len(pts) > 0 {
			hx, hy := g.toScreen(pts[len(pts)-1])
			vector.FillCircle(dst, hx, hy, g.metres(1, 2), color.White, true)
		}
	}
}

func (g *Game) drawToolOverlay(dst *ebiten.Image) {
	if g.tool != toolTrack {
		return
	}
	lvl := g.curLevel()
	p := g.plan.Snap(g.cursor, lvl, g.pickRadius(12))
	cx, cy := g.toScreen(p)
	if !g.drawing {
		vector.StrokeCircle(dst, cx, cy, 6, 2, colHover, true)
		return
	}
	ax, ay := g.toScreen(g.anchor)
	err := g.plan.CheckSegment(g.anchor, p, lvl)
	c := colOK
	if err != nil {
		c = colBad
	}
	vector.StrokeLine(dst, ax, ay, cx, cy, g.metres(2, 3), c, true)
	vector.FillCircle(dst, ax, ay, 4, c, true)
	vector.StrokeCircle(dst, cx, cy, 6, 2, c, true)
	var te build.TerrainError
	if errors.As(err, &te) {
		x, y := g.toScreen(te.At)
		vector.StrokeCircle(dst, x, y, 9, 2, colBad, true)
	}
	l := math.Hypot(p.X-g.anchor.X, p.Y-g.anchor.Y)
	label := fmt.Sprintf("%.0f m", l)
	if err != nil {
		label += "  " + err.Error()
	}
	ebitenutil.DebugPrintAt(dst, label, int(cx)+12, int(cy)+8)
}

func (g *Game) drawHUD(dst *ebiten.Image) {
	vector.FillRect(dst, 0, 0, float32(g.w), hudHeight, colHUD, false)
	var b strings.Builder
	for i, n := range toolNames {
		if tool(i) == g.tool {
			fmt.Fprintf(&b, "[%d %s]  ", i+1, strings.ToUpper(n))
		} else {
			fmt.Fprintf(&b, " %d %s   ", i+1, n)
		}
	}
	secs := int(g.world.Seconds())
	speed := fmt.Sprintf("x%d", speeds[g.speedIdx])
	if g.paused {
		speed = "PAUSED"
	}
	fmt.Fprintf(&b, "| Level: %s (Tab) | %02d:%02d:%02d %s", g.curLevel(), secs/3600, secs/60%60, secs%60, speed)
	ebitenutil.DebugPrintAt(dst, b.String(), 10, 6)

	help := map[tool]string{
		toolTrack:  "Click to start, click again to lay straight track. Right-click/Esc ends. Crossing track makes a junction.",
		toolStop:   "Click on track to place a stop.",
		toolLine:   "Click stops in order, Enter to create the line (one tram shuttles it). Right-click undoes, Backspace deletes newest line.",
		toolDelete: "Click a track segment to remove it.",
	}[g.tool]
	lines := []string{help, "WASD/arrows/middle-drag pan, wheel zoom, Space pause, +/- speed, B blocks"}
	if g.msgTTL > 0 {
		lines = append(lines, "> "+g.message)
	}
	y := g.h - 18*len(lines) - 8
	vector.FillRect(dst, 0, float32(y-4), float32(g.w), float32(g.h-y+4), colHUD, false)
	for i, l := range lines {
		ebitenutil.DebugPrintAt(dst, l, 10, y+18*i)
	}

	// Line list, top-left under the bar.
	for i, l := range g.plan.Lines() {
		st := g.status[i]
		names := make([]string, len(l.Stops))
		for j, id := range l.Stops {
			names[j] = g.plan.Stop(id).Name
		}
		info := ""
		switch {
		case st.Err != nil && st.Train < 0:
			info = "  ! " + st.Err.Error()
		case st.Train >= 0:
			t := g.world.Trains[st.Train]
			if t.State == sim.Dwelling {
				info = fmt.Sprintf("  at %s", t.Station)
			} else {
				info = fmt.Sprintf("  %2.0f km/h -> %s", t.Speed*3.6, t.NextStation())
			}
		}
		y := hudHeight + 8 + 20*i
		vector.FillRect(dst, 8, float32(y), float32(14+len(strings.Join(names, " - "))*6+len(info)*6+40), 18, colHUD, false)
		vector.FillRect(dst, 12, float32(y+4), 10, 10, lineColors[i%len(lineColors)], false)
		ebitenutil.DebugPrintAt(dst, fmt.Sprintf("%s %s%s", l.Mode, strings.Join(names, " - "), info), 28, y+1)
	}
}
