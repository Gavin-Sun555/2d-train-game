package street

import (
	"errors"
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"sort"
)

// Saved is the player-built part of a network: track on the roads, stops
// and lines. The roads themselves come from the map.
type Saved struct {
	RoadCount int         `json:"roadCount"` // guards against a changed map
	Track     []SavedRoad `json:"track"`
	Stops     []Stop      `json:"stops"`
	Lines     []SavedLine `json:"lines"`
	NextID    int         `json:"nextID"`
	// Edits are the player's road edits, replayed before the rest.
	Edits []RoadEdit `json:"roadEdits,omitempty"`
}

type SavedRoad struct {
	Road   RoadID   `json:"road"`
	Tram   TramMode `json:"tram"`
	Single bool     `json:"single,omitempty"`
	// Crossovers along the road (0..1 from A).
	Crossovers []float64 `json:"crossovers,omitempty"`
}

type SavedLine struct {
	ID       LineID   `json:"id"`
	Stops    []StopID `json:"stops"`
	Trams    int      `json:"trams,omitempty"`
	Sections int      `json:"sections,omitempty"`
	// PerSection: most passengers per section.
	PerSection int `json:"perSection,omitempty"`
	// Boarding / alighting seconds per passenger (tenths) and passengers per
	// section at once.
	BoardTenths  int `json:"boardTenths,omitempty"`
	AlightTenths int `json:"alightTenths,omitempty"`
	Doors        int `json:"doors,omitempty"`
	// Least stop times in seconds: on the way, and where trams turn back.
	DwellSecs    int    `json:"dwell,omitempty"`
	EndDwellSecs int    `json:"endDwell,omitempty"`
	Model        string `json:"model,omitempty"`
	MaxSpeed     int    `json:"maxSpeed,omitempty"`
}

// Export captures the player's track, stops and lines.
func (n *Network) Export() Saved {
	s := Saved{RoadCount: len(n.Roads), NextID: n.nextID, Edits: n.Edits()}
	for _, r := range n.Roads {
		if r.Tram != NoTram {
			s.Track = append(s.Track, SavedRoad{r.ID, r.Tram, r.Single, append([]float64(nil), r.Crossovers...)})
		}
	}
	for _, id := range n.StopIDs() {
		s.Stops = append(s.Stops, *n.Stops[id])
	}
	for _, id := range n.LineIDs() {
		l := n.Lines[id]
		s.Lines = append(s.Lines, SavedLine{l.ID, append([]StopID(nil), l.Stops...), l.Trams, l.Sections, l.PerSection, l.BoardTenths, l.AlightTenths, l.Doors, l.DwellSecs, l.EndDwellSecs, l.Model, l.MaxSpeed})
	}
	return s
}

// Import replaces the player's track, stops and lines with a saved set.
func (n *Network) Import(s Saved) error {
	if len(n.edits) > 0 {
		return errors.New(locale.T("只能在未修改过的地图路网上读取存档", "Saves can only be read onto the map's unedited roads"))
	}
	if err := n.ApplyEdits(s.Edits); err != nil {
		return err
	}
	if s.RoadCount != len(n.Roads) {
		return fmt.Errorf(locale.T("存档的路网（%d 条道路）与当前地图（%d 条）不一致", "The save's road network (%d roads) doesn't match this map (%d)"), s.RoadCount, len(n.Roads))
	}
	for _, t := range s.Track {
		if t.Road < 0 || int(t.Road) >= len(n.Roads) {
			return fmt.Errorf(locale.T("存档里有不存在的道路 %d", "The save has a road %d that does not exist"), t.Road)
		}
	}
	for i := range n.Roads {
		n.Roads[i].Tram, n.Roads[i].Single, n.Roads[i].Crossovers = NoTram, false, nil
	}
	for _, t := range s.Track {
		rd := &n.Roads[t.Road]
		rd.Tram, rd.Single, rd.Crossovers = t.Tram, t.Single, append([]float64(nil), t.Crossovers...)
	}
	n.Stops, n.Lines = map[StopID]*Stop{}, map[LineID]*Line{}
	for _, st := range s.Stops {
		if st.Road >= 0 && int(st.Road) < len(n.Roads) {
			st := st
			n.Stops[st.ID] = &st
		}
	}
	for _, sl := range s.Lines {
		var stops []StopID
		for _, id := range sl.Stops {
			if n.Stops[id] != nil {
				stops = append(stops, id)
			}
		}
		if len(stops) >= 2 {
			n.Lines[sl.ID] = &Line{ID: sl.ID, Stops: stops, Trams: sl.Trams, Sections: sl.Sections, PerSection: sl.PerSection,
				BoardTenths: sl.BoardTenths, AlightTenths: sl.AlightTenths, Doors: sl.Doors,
				DwellSecs: sl.DwellSecs, EndDwellSecs: sl.EndDwellSecs,
				Model: sl.Model, MaxSpeed: sl.MaxSpeed}
		}
	}
	n.nextID = s.NextID
	n.Rebuild()
	ids := make([]LineID, 0, len(n.Lines))
	for id := range n.Lines {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		n.LineCycle(n.Lines[id]) // works out the calling pattern
	}
	return nil
}
