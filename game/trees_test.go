package game

import "testing"

// No tree stands in a carriageway, a junction or on a motorway; most of the
// mapped trees are kept (moved to the pavement if they were on a street),
// and they move again when the player builds a road over them.
func TestTreesStayOffTheRoads(t *testing.T) {
	g := New("seattle")
	check := func() {
		t.Helper()
		for _, p := range g.scene.trees {
			for _, rd := range g.net.Roads {
				if onCarriageway(g.net, rd.ID, p, 0.5) {
					t.Fatalf("tree at %.0f,%.0f stands on %s", p.X, p.Y, rd.Name)
				}
			}
			if _, _, ok := g.highwayAt(p); ok {
				t.Fatalf("tree at %.0f,%.0f on a motorway", p.X, p.Y)
			}
		}
	}
	check()
	if kept := float64(len(g.scene.trees)) / float64(len(g.scene.mapTrees)); kept < 0.85 {
		t.Errorf("only %.0f%% of the trees kept", 100*kept)
	}
	// Build a street across the trees of a block and look again.
	built := false
	for _, r1 := range g.net.Roads[:400] {
		p1, q1 := g.net.Ends(r1.ID)
		a := p1.Lerp(q1, 0.5)
		for _, r2 := range g.net.Roads[:400] {
			p2, q2 := g.net.Ends(r2.ID)
			b := p2.Lerp(q2, 0.5)
			if d := dist(a, b); d > 120 && d < 300 && g.net.AddRoad(a, b, 2, 2, 1) == nil {
				built = true
				break
			}
		}
		if built {
			break
		}
	}
	if !built {
		t.Fatal("no road built")
	}
	g.edited()
	check()
}
