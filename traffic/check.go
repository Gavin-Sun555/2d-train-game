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
	// Single track: one tram at a time in a block.
	trams := map[int]map[int]bool{}
	for _, v := range s.Vehicles {
		var err error
		s.covered(v, func(k int, _ float64) {
			id := v.Path[k]
			l := &s.Net.Links[id]
			if l.Block >= 0 {
				if trams[l.Block] == nil {
					trams[l.Block] = map[int]bool{}
				}
				trams[l.Block][v.ID] = true
			}
			// Against the normal direction only on track reserved for it,
			// and nobody else on reserved track.
			o := s.owner[unitOf(s.Net, id)]
			if err == nil && ((l.Contra && o != v.ID) || (o != 0 && o != v.ID)) {
				err = fmt.Errorf("vehicle %d on link %d reserved by %d", v.ID, id, o)
			}
		})
		if err != nil {
			return err
		}
	}
	for b, vs := range trams {
		if len(vs) > 1 {
			return fmt.Errorf("single-track block %d occupied by %d trams", b, len(vs))
		}
	}
	return nil
}
