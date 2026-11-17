package traffic

import "github.com/Gavin-Sun555/2d-train-game/street"

// Running against the normal direction of a double track (after a
// crossover, or changing ends where there is none) is only safe on track
// nobody else is on or about to enter. A tram reserves the whole stretch it
// will run that way, plus the piece it changes ends on, before it starts:
// the "run". Until it has left the run, other vehicles stop short of it.
// While a tram waits for a run, the track is also "requested": vehicles
// that can still stop in time do not enter it, so it drains.

// unitOf is the physical piece of track (or lane) a link lies on: the link
// or its twin, whichever has the lower ID.
func unitOf(n *street.Network, id street.LinkID) street.LinkID {
	if tw := n.Links[id].Twin; tw >= 0 && tw < id {
		return tw
	}
	return id
}

// runStart reports whether path[k] begins a run: it is wrong-way track, or
// a double-track piece the tram changes ends on.
func runStart(n *street.Network, path []street.LinkID, k int) bool {
	l := &n.Links[path[k]]
	if l.Contra {
		return true
	}
	return k+1 < len(path) && path[k+1] == l.Twin && l.Block < 0 && l.Kind == street.LaneLink
}

// runEnd is the last index of the run starting at t: through every
// wrong-way link and change of ends that follows.
func runEnd(n *street.Network, path []street.LinkID, t int) int {
	j := t
	for j+1 < len(path) {
		nx := path[j+1]
		if nx == n.Links[path[j]].Twin || n.Links[nx].Contra {
			j++
			continue
		}
		break
	}
	return j
}

// owns reports whether v holds the run covering v.Path[k].
func (s *Sim) owns(v *Vehicle, k int) bool {
	return s.owner[unitOf(s.Net, v.Path[k])] == v.ID
}

// taken reports whether link id is reserved by another vehicle, or (when
// canStop) requested by one.
func (s *Sim) taken(v *Vehicle, id street.LinkID, canStop bool) bool {
	u := unitOf(s.Net, id)
	if o := s.owner[u]; o != 0 && (v == nil || o != v.ID) {
		return true
	}
	if r := s.req[u]; canStop && r != 0 && (v == nil || r != v.ID) && s.reqTick[u] >= s.Tick-1 {
		return true
	}
	return false
}

// approaching reports whether a vehicle other than v is on, or could not
// stop before, any of the units.
func (s *Sim) approaching(v *Vehicle, units map[street.LinkID]bool) bool {
	for _, w := range s.Vehicles {
		if w == v {
			continue
		}
		hit := false
		s.covered(w, func(k int, _ float64) {
			if units[unitOf(s.Net, w.Path[k])] {
				hit = true
			}
		})
		if hit {
			return true
		}
		if w.Dwelling {
			continue // it sees the reservation before it moves off
		}
		reach := w.V*w.V/(2*w.B) + w.V + 0.5
		d := s.Net.Links[w.Path[w.idx]].Length - w.S
		for j := w.idx + 1; j < len(w.Path) && d <= reach; j++ {
			if units[unitOf(s.Net, w.Path[j])] {
				return true
			}
			d += s.Net.Links[w.Path[j]].Length
		}
	}
	return false
}

// tryRun reserves the run starting at v.Path[t]. On failure it requests
// the track so that it empties.
func (s *Sim) tryRun(v *Vehicle, t int) bool {
	e := runEnd(s.Net, v.Path, t)
	units := map[street.LinkID]bool{}
	for j := t; j <= e; j++ {
		units[unitOf(s.Net, v.Path[j])] = true
	}
	ok := true
	for u := range units {
		if s.taken(v, u, true) {
			ok = false
		}
		tw := s.Net.Links[u].Twin
		for _, id := range []street.LinkID{u, tw} {
			if id < 0 {
				continue
			}
			if s.holders[id] > 0 && !s.holdsOwn(v, id) {
				ok = false
			}
			if in := s.inbound[id]; in > 0 && !(v.admitted >= 0 && v.admitted+1 < len(v.Path) && v.Path[v.admitted+1] == id) {
				ok = false
			}
		}
	}
	// Track already held is respected by everyone; only new track needs
	// nobody about to run onto it.
	fresh := map[street.LinkID]bool{}
	for u := range units {
		if s.owner[u] != v.ID {
			fresh[u] = true
		}
	}
	if ok && len(fresh) > 0 && s.approaching(v, fresh) {
		ok = false
	}
	if !ok {
		for u := range units {
			if r := s.req[u]; r == 0 || r == v.ID || s.reqTick[u] < s.Tick-1 {
				s.req[u], s.reqTick[u] = v.ID, s.Tick
			}
		}
		return false
	}
	for u := range units {
		s.owner[u] = v.ID
		if s.req[u] == v.ID {
			s.req[u] = 0
		}
	}
	if v.runTo < 0 {
		v.runFrom = t
	} else {
		v.runFrom = min(v.runFrom, t)
	}
	v.runTo = max(v.runTo, e)
	return true
}

// holdsOwn reports whether v itself holds the admission for connector id.
func (s *Sim) holdsOwn(v *Vehicle, id street.LinkID) bool {
	for j := max(v.idx-8, 0); j <= v.admitted && j < len(v.Path); j++ {
		if v.Path[j] == id {
			return true
		}
	}
	return false
}

// markRuns records who owns which track, from each tram's run.
func (s *Sim) markRuns() {
	if len(s.owner) != len(s.Net.Links) {
		s.owner = make([]int, len(s.Net.Links))
		s.req = make([]int, len(s.Net.Links))
		s.reqTick = make([]int64, len(s.Net.Links))
	}
	for i := range s.owner {
		s.owner[i] = 0
	}
	for _, v := range s.Vehicles {
		if v.runTo < 0 {
			continue
		}
		first := v.idx
		s.covered(v, func(k int, _ float64) { first = k })
		if first > v.runTo {
			v.runTo = -1 // the tram has left its run
			continue
		}
		for j := max(first, v.runFrom); j <= v.runTo && j < len(v.Path); j++ {
			s.owner[unitOf(s.Net, v.Path[j])] = v.ID
		}
	}
}
