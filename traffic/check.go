package traffic

import (
	"fmt"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// CheckInvariants verifies the safety rules: no two vehicles overlap in a
// lane, nobody is inside a junction movement they were not granted, and no
// two conflicting movements are physically occupied at once.
func (s *Sim) CheckInvariants() error {
	s.rebuildOccupancy()
	for id, es := range s.occ {
		for i := 1; i < len(es); i++ {
			behind, ahead := es[i-1], es[i]
			if behind.v == ahead.v {
				continue
			}
			if ahead.front-ahead.v.Length < behind.front-0.05 {
				return fmt.Errorf("vehicles %d and %d overlap on link %d", behind.v.ID, ahead.v.ID, id)
			}
		}
	}
	inside := map[street.LinkID]int{}
	for _, v := range s.Vehicles {
		if v.V < 0 {
			return fmt.Errorf("vehicle %d negative speed", v.ID)
		}
		var err error
		s.covered(v, func(k int, _ float64) {
			c := v.Path[k]
			if s.Net.Links[c].Kind != street.ConnectorLink {
				return
			}
			if k > v.admitted && err == nil {
				err = fmt.Errorf("vehicle %d inside junction movement %d without admission", v.ID, c)
			}
			inside[c] = v.ID
		})
		if err != nil {
			return err
		}
	}
	for c, vid := range inside {
		for _, cc := range s.Net.Links[c].Conflicts {
			if other, ok := inside[cc]; ok && other != vid {
				return fmt.Errorf("vehicles %d and %d on conflicting movements %d/%d", vid, other, c, cc)
			}
		}
	}
	// Track blocks: never vehicles running both ways, and only one tram at
	// a time on single track.
	pos := map[int]map[int]bool{}
	neg := map[int]map[int]bool{}
	for _, v := range s.Vehicles {
		s.covered(v, func(k int, _ float64) {
			l := &s.Net.Links[v.Path[k]]
			if l.Block < 0 {
				return
			}
			m := pos
			if l.BlockSign < 0 {
				m = neg
			}
			if m[l.Block] == nil {
				m[l.Block] = map[int]bool{}
			}
			m[l.Block][v.ID] = true
		})
	}
	for b := range s.Net.Blocks {
		both := map[int]bool{}
		for id := range pos[b] {
			both[id] = true
		}
		for id := range neg[b] {
			both[id] = true
		}
		mixed := len(pos[b]) > 0 && len(neg[b]) > 0 && len(both) > 1
		if mixed || (s.Net.Blocks[b].Kind == street.SingleTrackBlock && len(both) > 1) {
			return fmt.Errorf("track block %d occupied by %d vehicles in conflicting directions", b, len(both))
		}
	}
	return nil
}
