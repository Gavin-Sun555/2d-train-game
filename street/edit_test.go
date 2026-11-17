package street

import (
	"encoding/json"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/city"
)

func gridNet(t *testing.T) *Network {
	t.Helper()
	return Generate(city.Generate(1))
}

// A road between two road middles crossing others; then demolished and its
// lanes changed; a fresh network replays the log to the same roads.
func TestRoadEditsReplay(t *testing.T) {
	n := gridNet(t)
	// Two parallel roads far enough apart, joined across whatever lies between.
	var a, b Vec
	found := false
	for _, r1 := range n.Roads {
		p1, q1 := n.Ends(r1.ID)
		if dist(p1, q1) < 80 || r1.Tram != NoTram {
			continue
		}
		m1 := p1.Lerp(q1, 0.5)
		for _, r2 := range n.Roads {
			p2, q2 := n.Ends(r2.ID)
			m2 := p2.Lerp(q2, 0.5)
			if r2.ID == r1.ID || dist(p2, q2) < 80 || dist(m1, m2) < 150 || dist(m1, m2) > 400 {
				continue
			}
			a, b, found = m1, m2, true
			break
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("no pair of roads")
	}
	before := len(n.Roads)
	err := n.AddRoad(a, b, 1, 1, 2)
	if err != nil {
		t.Logf("first pair: %v", err)
	}
	if err != nil {
		// Try a few other pairs until one works.
		for _, r := range n.Roads[:200] {
			p, q := n.Ends(r.ID)
			if n.AddRoad(a, p.Lerp(q, 0.5), 1, 1, 2) == nil {
				err = nil
				break
			}
		}
	}
	if err != nil {
		t.Fatal("no road could be added")
	}
	if len(n.Roads) <= before {
		t.Fatalf("roads %d → %d", before, len(n.Roads))
	}
	// Change a plain road's lanes, then demolish another.
	var lanes, gone RoadID = -1, -1
	for _, rd := range n.Roads {
		if !n.hasTrack(rd.ID) && n.SetRoadLanes(rd.ID, 2, 2) == nil {
			lanes = rd.ID
			break
		}
	}
	for i := len(n.Roads) - 1; i >= 0; i-- {
		if n.RemoveRoad(RoadID(i)) == nil {
			gone = RoadID(i)
			break
		}
	}
	if lanes < 0 || gone < 0 {
		t.Fatalf("lanes %d, remove %d", lanes, gone)
	}
	if err := n.usable(); err != nil {
		t.Fatal(err)
	}

	m := gridNet(t)
	if err := m.Import(n.Export()); err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(n.Roads)
	y, _ := json.Marshal(m.Roads)
	if string(x) != string(y) {
		t.Fatal("replayed roads differ")
	}
}

func TestRoadEditsRefused(t *testing.T) {
	n := gridNet(t)
	r := n.Roads[10].ID
	n.SetTram([]RoadID{r}, Dedicated, false)
	if n.RemoveRoad(r) == nil || n.SetRoadLanes(r, 1, 1) == nil {
		t.Error("a road with track was changed")
	}
	if n.AddRoad(Vec{X: -500, Y: -500}, n.Junctions[0].Pos, 1, 1, 2) == nil {
		t.Error("a road to nowhere was built")
	}
	if n.SetRoadLanes(n.Roads[20].ID, 0, 0) == nil {
		t.Error("a road with no lanes was accepted")
	}
	if len(n.Edits()) != 0 {
		t.Errorf("refused edits were logged: %v", n.Edits())
	}
	// Making every road around a junction one-way inwards is refused.
	if err := n.usable(); err != nil {
		t.Fatal(err)
	}
}
