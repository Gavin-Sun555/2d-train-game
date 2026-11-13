package game

import (
	"fmt"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// LoadDemo lays three sample tram lines: one in mixed traffic on a street,
// two with their own right of way (one along the boulevard) crossing it.
func (g *Game) LoadDemo() error {
	if g.scene != nil {
		return g.loadSeattleDemo()
	}
	n := g.net
	at := func(x, y float64) street.JunctionID {
		j, ok := n.NearestJunction(street.Vec{X: x, Y: y}, 2)
		if !ok {
			panic(fmt.Sprintf("demo: no junction at %.0f,%.0f", x, y))
		}
		return j
	}
	type run struct {
		mode   street.TramMode
		single bool
		ax, ay float64
		bx, by float64
	}
	runs := []run{
		{street.Shared, false, 170, 170, 810, 170},
		{street.Dedicated, false, 330, 20, 330, 500},
		{street.Dedicated, false, 170, 500, 980, 500},
		{street.Dedicated, true, 500, 330, 980, 330},
	}
	var lines [][]street.RoadID
	for _, r := range runs {
		roads, ok := n.RoadPath(at(r.ax, r.ay), at(r.bx, r.by))
		if !ok {
			return fmt.Errorf("demo: no road path")
		}
		n.SetTram(roads, r.mode, r.single)
		lines = append(lines, roads)
	}
	for _, roads := range lines {
		var stops []street.StopID
		for _, r := range []street.RoadID{roads[0], roads[len(roads)/2], roads[len(roads)-1]} {
			id, err := n.AddStop(r, 0.5)
			if err != nil {
				return err
			}
			stops = append(stops, id)
		}
		if _, err := n.AddLine(stops); err != nil {
			return err
		}
	}
	g.rebuild()
	g.camX, g.camY, g.zoom = 560, 300, 1.25
	return nil
}
