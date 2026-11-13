package game

import (
	"github.com/Gavin-Sun555/2d-train-game/build"
	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

// LoadDemo lays out three sample tram lines along the streets, including a
// flat crossing and two T-junctions. Used by -demo and for screenshots.
func (g *Game) LoadDemo() error {
	st := func(i int) float64 { return city.RoadCentre(i) }
	v := func(x, y float64) sim.Vec2 { return sim.Vec2{X: x, Y: y} }
	p := build.NewPlan(g.city)
	segs := [][2]sim.Vec2{
		{v(st(8), st(8)), v(st(40), st(8))},
		{v(st(40), st(8)), v(st(40), st(24))},
		{v(st(16), st(0)+20), v(st(16), st(24))},
		{v(st(8), st(24)), v(st(48), st(24))},
	}
	for _, s := range segs {
		if _, err := p.AddSegment(s[0], s[1], sim.Ground); err != nil {
			return err
		}
	}
	stop := func(x, y float64) build.StopID {
		id, err := p.AddStop(v(x, y), sim.Ground, 5)
		if err != nil {
			panic(err)
		}
		return id
	}
	lines := [][]build.StopID{
		{stop(st(8)+30, st(8)), stop(st(32), st(8)), stop(st(40), st(16))},
		{stop(st(16), st(0)+40), stop(st(16), st(16))},
		{stop(st(8)+30, st(24)), stop(st(32), st(24)), stop(st(48)-30, st(24))},
	}
	for _, l := range lines {
		if _, err := p.AddLine(build.Tram, l); err != nil {
			return err
		}
	}
	g.plan = p
	g.rebuild()
	g.frame(p)
	return nil
}

// frame centres the camera on everything built and zooms to fit it.
func (g *Game) frame(p *build.Plan) {
	nodes := p.Nodes()
	if len(nodes) == 0 {
		return
	}
	minX, minY, maxX, maxY := nodes[0].Pos.X, nodes[0].Pos.Y, nodes[0].Pos.X, nodes[0].Pos.Y
	for _, n := range nodes {
		minX, maxX = min(minX, n.Pos.X), max(maxX, n.Pos.X)
		minY, maxY = min(minY, n.Pos.Y), max(maxY, n.Pos.Y)
	}
	g.camX, g.camY = (minX+maxX)/2, (minY+maxY)/2
	const margin = 160.0
	g.zoom = min(float64(g.w)/(maxX-minX+2*margin), float64(g.h-2*hudHeight)/(maxY-minY+2*margin))
}
