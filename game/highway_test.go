package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
)

// Every motorway or ramp end runs on into another motorway piece, meets a
// street junction, or leaves the map.
func TestMotorwaysConnect(t *testing.T) {
	m := mapdata.Seattle()
	g := New("seattle")
	use := map[int]int{}
	for _, h := range m.Highways {
		use[h.Nodes[0]]++
		use[h.Nodes[len(h.Nodes)-1]]++
		for _, id := range h.Nodes[1 : len(h.Nodes)-1] {
			use[id] += 2
		}
	}
	gates, loose := 0, 0
	for _, h := range m.Highways {
		for _, id := range []int{h.Nodes[0], h.Nodes[len(h.Nodes)-1]} {
			p := m.Nodes[id]
			switch {
			case use[id] > 1:
			case g.net.Gates[id] >= 0 && hasGate(g, id):
				gates++
				j := g.net.Gates[id]
				if len(g.net.RoadsAt(j)) == 0 {
					t.Errorf("ramp end at %.0f,%.0f meets a junction with no streets", p[0], p[1])
				}
			case p[0] < 5 || p[1] < 5 || p[0] > m.W-5 || p[1] > m.H-5:
			default:
				loose++
				t.Logf("motorway end at %.0f,%.0f leads nowhere", p[0], p[1])
			}
		}
	}
	t.Logf("%d ramp ends meet streets, %d loose", gates, loose)
	// Two known exceptions: the entrance to the Colman Dock ferry holding
	// lanes (which are not public roads), and a one-way scrap of Dexter
	// Way N leading only to a stretch of street missing from OpenStreetMap.
	if loose > 2 {
		t.Errorf("%d motorway ends lead nowhere", loose)
	}
	if gates < 30 {
		t.Errorf("only %d ramps meet the streets", gates)
	}
	// Ramp ends are drawn at the junction they meet.
	for i := range g.scene.highways {
		h := &g.scene.highways[i]
		for k, id := range h.nodes {
			if j, ok := g.net.Gates[id]; ok && id >= 0 && h.pts[k] != g.net.Junctions[j].Pos {
				t.Fatalf("ramp point %d not on its junction", id)
			}
		}
	}
}

func hasGate(g *Game, id int) bool { _, ok := g.net.Gates[id]; return ok }
