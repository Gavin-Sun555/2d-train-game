package game

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// The South Lake Union Streetcar, stop by stop from Westlake Hub
// (McGraw Square) to Fairview & Campus Drive.
var sluStops = []string{
	"Westlake Hub", "Westlake & 7th", "Westlake & 9th-Denny", "Westlake & Thomas",
	"Westlake & Mercer", "Lake Union Park", "Terry & Mercer", "Terry & Thomas",
	"Fairview & Campus Drive",
}

// loadSeattleDemo lays tram track wherever the real SLU Streetcar runs and
// recreates its line with the real stop names.
func (g *Game) loadSeattleDemo() error {
	n, m := g.net, g.scene.src
	// Roads whose centre line follows the mapped tram track.
	nearTram := func(p street.Vec) float64 {
		best := math.Inf(1)
		for _, way := range m.Tram {
			for i := 0; i+1 < len(way); i++ {
				a, b := street.Vec{X: way[i][0], Y: way[i][1]}, street.Vec{X: way[i+1][0], Y: way[i+1][1]}
				best = math.Min(best, segDist(p, a, b))
			}
		}
		return best
	}
	// Real stops: average the two platforms of each name, then place the
	// stop on the nearest road with track.
	sum := map[string]street.Vec{}
	cnt := map[string]float64{}
	for _, s := range m.TramStops {
		sum[s.Name] = add(sum[s.Name], street.Vec{X: s.P[0], Y: s.P[1]})
		cnt[s.Name]++
	}
	var pts []street.Vec
	var names []string
	for _, name := range sluStops {
		if cnt[name] > 0 {
			pts = append(pts, scale(sum[name], 1/cnt[name]))
			names = append(names, name)
		}
	}
	// Track on every road the real streetcar runs along (one-way streets
	// included: there it runs with the traffic)...
	matched := map[street.RoadID]bool{}
	var route []street.RoadID
	for _, rd := range n.Roads {
		a, b := n.Ends(rd.ID)
		near := 0
		for _, t := range []float64{0.2, 0.35, 0.5, 0.65, 0.8} {
			if nearTram(a.Lerp(b, t)) < 12 {
				near++
			}
		}
		if near >= 4 {
			matched[rd.ID] = true
			route = append(route, rd.ID)
		}
	}
	// ...plus the shortest links needed to join consecutive stops where the
	// merged street network and the mapped track part ways.
	cost := func(r street.RoadID) float64 {
		if matched[r] {
			return n.Length(r)
		}
		if n.Roads[r].OneWay() {
			return -1
		}
		return 8 * n.Length(r)
	}
	nearestJ := func(p street.Vec) street.JunctionID {
		best, bd := street.JunctionID(0), math.Inf(1)
		for _, j := range n.Junctions {
			if d := dist(j.Pos, p); d < bd {
				best, bd = j.ID, d
			}
		}
		return best
	}
	for i := 0; i+1 < len(pts); i++ {
		for _, pair := range [][2]street.Vec{{pts[i], pts[i+1]}, {pts[i+1], pts[i]}} {
			if p, ok := n.RoadPathCost(nearestJ(pair[0]), nearestJ(pair[1]), cost); ok {
				route = append(route, p...)
			}
		}
	}
	if len(route) == 0 {
		return fmt.Errorf("no route along the streetcar")
	}
	n.SetTram(route, street.Shared, false)

	var stops []street.StopID
	for i, p := range pts {
		if id, ok := g.placeStop(p, names[i]); ok {
			stops = append(stops, id)
		}
	}
	// Crossovers at both ends, then build the line, dropping any stop the
	// track cannot serve.
	n.AddEndCrossovers(stops)
	for len(stops) >= 2 {
		if _, err := n.AddLine(stops); err == nil {
			break
		}
		dropped := false
		for i := range stops {
			try := append(append([]street.StopID(nil), stops[:i]...), stops[i+1:]...)
			if _, err := n.AddLine(try); err == nil {
				n.DeleteLine(n.LineIDs()[len(n.LineIDs())-1])
				stops, dropped = try, true
				break
			}
		}
		if !dropped {
			stops = stops[:len(stops)-1]
		}
	}
	g.rebuild()
	g.camX, g.camY, g.zoom = 2486, 3214, 1.2
	return nil
}

// placeStop puts a named stop on the tram road nearest p, nudged clear of
// junctions if needed.
func (g *Game) placeStop(p street.Vec, name string) (street.StopID, bool) {
	n := g.net
	type cand struct {
		r street.RoadID
		d float64
		t float64
	}
	var cs []cand
	for _, rd := range n.Roads {
		if rd.Tram == street.NoTram {
			continue
		}
		a, b := n.Ends(rd.ID)
		if d := segDist(p, a, b); d < 120 {
			l := dist(a, b)
			t := math.Max(0, math.Min(1, ((p.X-a.X)*(b.X-a.X)+(p.Y-a.Y)*(b.Y-a.Y))/(l*l)))
			// "Westlake & Mercer" stands on Westlake: prefer the road named
			// first in the stop's name.
			street := strings.TrimSpace(strings.SplitN(name, "&", 2)[0])
			if !strings.HasPrefix(rd.Name, street) {
				d += 80
			}
			// A divided boulevard's one-way halves cannot turn trams back.
			if rd.OneWay() {
				d += 60
			}
			cs = append(cs, cand{rd.ID, d, t})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	for _, c := range cs {
		for _, t := range []float64{c.t, 0.5*c.t + 0.25, 0.5} {
			if id, err := n.AddStop(c.r, t, street.DefaultPlatform); err == nil {
				n.Stops[id].Name = name
				return id, true
			}
		}
	}
	return 0, false
}

func segDist(p, a, b street.Vec) float64 {
	ab := sub(b, a)
	l2 := ab.X*ab.X + ab.Y*ab.Y
	if l2 == 0 {
		return dist(p, a)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*ab.X+(p.Y-a.Y)*ab.Y)/l2))
	return dist(p, a.Lerp(b, t))
}
