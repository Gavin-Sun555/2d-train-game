package game

import (
	"math"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// The Seattle map grew twice: from Downtown and South Lake Union to most of
// central Seattle, then east to Lake Washington; then its roads gained
// levels and junctions where motorway ramps meet them. Saves made on an older map
// name its roads by number, so they are carried over by position: every
// road, stop and crossover of the old save is found again on the new map at
// the same place.

// oldSeattle is an earlier Seattle map and where its origin lies now.
type oldSeattle struct {
	load func() *mapdata.Map
	off  mapdata.Pt
}

var oldSeattles = []oldSeattle{
	{mapdata.SeattleV3, mapdata.Pt{}},
	{mapdata.SeattleV2, mapdata.Pt{}},
	{mapdata.SeattleV1, mapdata.SeattleV1Offset},
}

// migrateSeattle converts a save made on an earlier Seattle map to the
// current network. It reports the offset its coordinates move by, and false
// if the save is from none of them.
func migrateSeattle(s street.Saved, to *street.Network) (street.Saved, mapdata.Pt, bool) {
	for _, o := range oldSeattles {
		old := street.FromMapLegacy(o.load())
		if s.RoadCount != len(old.Roads) || old.Import(s) != nil {
			continue
		}
		return migrateFrom(old, s, to, o.off), o.off, true
	}
	return s, mapdata.Pt{}, false
}

func migrateFrom(old *street.Network, s street.Saved, to *street.Network, o mapdata.Pt) street.Saved {
	off := street.Vec{X: o[0], Y: o[1]}
	toOld := func(p street.Vec) street.Vec { return street.Vec{X: p.X - off.X, Y: p.Y - off.Y} }
	toNew := func(p street.Vec) street.Vec { return street.Vec{X: p.X + off.X, Y: p.Y + off.Y} }
	const tol = 2.0 // m

	out := street.Saved{RoadCount: len(to.Roads), NextID: s.NextID, Lines: s.Lines}
	tracked := map[street.RoadID]bool{}
	for _, rd := range to.Roads {
		a, b := to.Ends(rd.ID)
		var src street.RoadID = -1
		ok := true
		for _, t := range []float64{0.25, 0.5, 0.75} {
			r, _, found := old.NearestRoad(toOld(a.Lerp(b, t)), tol)
			if !found || old.Roads[r].Tram == street.NoTram || (src >= 0 && r != src) {
				ok = false
				break
			}
			src = r
		}
		if !ok {
			continue
		}
		o := old.Roads[src]
		out.Track = append(out.Track, street.SavedRoad{Road: rd.ID, Tram: o.Tram, Single: o.Single})
		tracked[rd.ID] = true
	}
	// nearestTracked finds the new road with track under a point of the old map.
	nearestTracked := func(p street.Vec) (street.RoadID, float64, bool) {
		q := toNew(p)
		best, bt, bd := street.RoadID(-1), 0.0, tol
		for i := range out.Track {
			r := out.Track[i].Road
			a, b := to.Ends(r)
			t, d := projectOn(q, a, b)
			if d <= bd {
				best, bt, bd = r, t, d
			}
		}
		return best, bt, best >= 0
	}
	for _, rd := range old.Roads {
		a, b := old.Ends(rd.ID)
		for _, x := range rd.Crossovers {
			if r, t, ok := nearestTracked(a.Lerp(b, x)); ok {
				for i := range out.Track {
					if out.Track[i].Road == r {
						out.Track[i].Crossovers = append(out.Track[i].Crossovers, t)
					}
				}
			}
		}
	}
	for _, id := range old.StopIDs() {
		st := *old.Stops[id]
		r, t, ok := nearestTracked(old.StopPos(&st))
		if !ok {
			continue
		}
		st.Road, st.T = r, t
		out.Stops = append(out.Stops, st)
	}
	return out
}

// projectOn is the position (0..1) of p's foot on segment ab and its distance.
func projectOn(p, a, b street.Vec) (float64, float64) {
	ab := sub(b, a)
	l2 := ab.X*ab.X + ab.Y*ab.Y
	if l2 == 0 {
		return 0, dist(p, a)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*ab.X+(p.Y-a.Y)*ab.Y)/l2))
	return t, dist(p, a.Lerp(b, t))
}
