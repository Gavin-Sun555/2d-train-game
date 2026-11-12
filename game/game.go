// Package game is the Ebitengine presentation layer. It only reads from
// rail/sim; all rules live in the simulation core.
package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

var speeds = []int{1, 2, 4, 8, 16, 32}

var (
	colBackground = color.RGBA{0x16, 0x1a, 0x21, 0xff}
	colFree       = color.RGBA{0x5a, 0x61, 0x6d, 0xff}
	colReserved   = color.RGBA{0xd9, 0xa4, 0x2b, 0xff}
	colOccupied   = color.RGBA{0xd9, 0x4a, 0x3d, 0xff}
	colGreen      = color.RGBA{0x3c, 0xd0, 0x6a, 0xff}
	colRed        = color.RGBA{0xe0, 0x3a, 0x3a, 0xff}
	colPlatform   = color.RGBA{0xc8, 0xd0, 0xdc, 0xff}
	trainColors   = []color.RGBA{
		{0x3b, 0x9c, 0xff, 0xff},
		{0xb3, 0x6b, 0xff, 0xff},
		{0x2e, 0xd3, 0xc6, 0xff},
		{0xff, 0x8a, 0x3d, 0xff},
	}
)

type Game struct {
	scenario int
	world    *sim.World
	paused   bool
	speedIdx int

	// world → screen transform, recomputed every frame from the window size
	scale, offX, offY float64
	w, h              int
}

func New(scenario int) *Game {
	g := &Game{}
	g.load(scenario)
	return g
}

// Warp advances the simulation by the given simulated seconds instantly.
func (g *Game) Warp(seconds float64) {
	for i := 0; i < int(seconds/sim.DT); i++ {
		g.world.Step()
	}
}

func (g *Game) load(i int) {
	if i < 0 || i >= len(sim.Scenarios) {
		i = 0
	}
	g.scenario = i
	g.world = sim.Scenarios[i].New()
}

func (g *Game) Update() error {
	for i := range sim.Scenarios {
		if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
			g.load(i)
		}
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		g.paused = !g.paused
	case inpututil.IsKeyJustPressed(ebiten.KeyR):
		g.load(g.scenario)
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual), inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
		g.speedIdx = min(g.speedIdx+1, len(speeds)-1)
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus), inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
		g.speedIdx = max(g.speedIdx-1, 0)
	}
	if !g.paused {
		// One sim tick per 60 Hz update at 1×; fast-forward runs more ticks.
		for i := 0; i < speeds[g.speedIdx]; i++ {
			g.world.Step()
		}
	}
	return nil
}

func (g *Game) Layout(outW, outH int) (int, int) {
	g.w, g.h = outW, outH
	return outW, outH
}

func (g *Game) fit() {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, n := range g.world.Net.Nodes {
		minX, minY = math.Min(minX, n.Pos.X), math.Min(minY, n.Pos.Y)
		maxX, maxY = math.Max(maxX, n.Pos.X), math.Max(maxY, n.Pos.Y)
	}
	const padX, padTop, padBottom = 80.0, 170.0, 80.0
	availW := float64(g.w) - 2*padX
	availH := float64(g.h) - padTop - padBottom
	g.scale = math.Min(availW/math.Max(maxX-minX, 1), availH/math.Max(maxY-minY, 1))
	g.offX = padX + (availW-(maxX-minX)*g.scale)/2 - minX*g.scale
	g.offY = padTop + (availH-(maxY-minY)*g.scale)/2 - minY*g.scale
}

func (g *Game) screen(p sim.Vec2) (float32, float32) {
	return float32(p.X*g.scale + g.offX), float32(p.Y*g.scale + g.offY)
}

func (g *Game) Draw(dst *ebiten.Image) {
	dst.Fill(colBackground)
	g.fit()
	w, net := g.world, g.world.Net

	// Track, coloured by block state.
	for _, e := range net.Edges {
		x0, y0 := g.screen(net.Nodes[e.A].Pos)
		x1, y1 := g.screen(net.Nodes[e.B].Pos)
		c := colFree
		switch w.BlockState(e.ID) {
		case sim.BlockReserved:
			c = colReserved
		case sim.BlockOccupied:
			c = colOccupied
		}
		vector.StrokeLine(dst, x0, y0, x1, y1, 5, c, true)
	}
	// Block joints.
	for _, n := range net.Nodes {
		x, y := g.screen(n.Pos)
		vector.FillCircle(dst, x, y, 4, colFree, true)
	}

	// Signals at each block entry, on the right-hand side of travel.
	for _, e := range net.Edges {
		for _, fwd := range []bool{true, false} {
			s := sim.Step{Edge: e.ID, Forward: fwd}
			if !g.usedByRoute(s) {
				continue
			}
			p := net.PointOnStep(s, math.Min(25, e.Length/4))
			x, y := g.screen(p)
			fx, fy := g.screen(net.Nodes[net.StepTo(s)].Pos)
			dx, dy := fx-x, fy-y
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l > 0 {
				dx, dy = dx/l, dy/l
			}
			sx, sy := x-dy*14, y+dx*14
			c := colRed
			if w.SignalGreen(s) {
				c = colGreen
			}
			vector.StrokeLine(dst, x-dy*4, y+dx*4, sx, sy, 2, colFree, true)
			vector.FillCircle(dst, sx, sy, 5, c, true)
		}
	}

	// Stations.
	for _, n := range net.Nodes {
		if n.Name == "" {
			continue
		}
		x, y := g.screen(n.Pos)
		vector.StrokeRect(dst, x-10, y-10, 20, 20, 2, colPlatform, true)
		ebitenutil.DebugPrintAt(dst, n.Name, int(x)-len(n.Name)*3, int(y)+16)
	}

	// Trains.
	for _, t := range w.Trains {
		pts := w.Body(t)
		c := trainColors[t.ID%len(trainColors)]
		for i := 0; i+1 < len(pts); i++ {
			x0, y0 := g.screen(pts[i])
			x1, y1 := g.screen(pts[i+1])
			vector.StrokeLine(dst, x0, y0, x1, y1, 12, c, true)
		}
		if len(pts) > 0 {
			hx, hy := g.screen(pts[len(pts)-1])
			vector.FillCircle(dst, hx, hy, 4, color.White, true)
			ebitenutil.DebugPrintAt(dst, t.Name, int(hx)+8, int(hy)-24)
		}
	}

	g.drawHUD(dst)
}

func (g *Game) usedByRoute(s sim.Step) bool {
	for _, t := range g.world.Trains {
		for _, leg := range t.Route.Legs {
			for _, rs := range leg.Steps {
				if rs == s {
					return true
				}
			}
		}
	}
	return false
}

func (g *Game) drawHUD(dst *ebiten.Image) {
	w := g.world
	secs := int(w.Seconds())
	state := fmt.Sprintf("x%d", speeds[g.speedIdx])
	if g.paused {
		state = "PAUSED"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s   t=%02d:%02d:%02d   %s   FPS %.0f\n",
		sim.Scenarios[g.scenario].Name, secs/3600, secs/60%60, secs%60, state, ebiten.ActualFPS())
	fmt.Fprintf(&b, "[1-%d] scenario  [Space] pause  [+/-] speed  [R] reset\n\n", len(sim.Scenarios))
	for _, t := range w.Trains {
		extra := "-> " + t.NextStation()
		if t.State == sim.Dwelling {
			extra = fmt.Sprintf("at %s, departs in %.0fs", t.Station, math.Ceil(t.DwellLeft))
		} else if t.Speed == 0 {
			extra += "  (held at red)"
		}
		fmt.Fprintf(&b, "%-3s %-5s %5.1f km/h  %s\n", t.Name, t.State, t.Speed*3.6, extra)
	}
	ebitenutil.DebugPrintAt(dst, b.String(), 12, 10)
}
