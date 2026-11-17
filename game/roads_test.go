package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// The Road tool builds a street between two streets, the edit survives a
// save, and the service still runs on the edited network.
func TestRoadToolBuildsAndSaves(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	g.zoom, g.page, g.tool = 1, pageBuild, toolRoad
	before := len(g.net.Roads)
	// Two road middles 150–400 m apart.
	var a, b street.Vec
	built := false
	for _, r1 := range g.net.Roads {
		p1, q1 := g.net.Ends(r1.ID)
		a = p1.Lerp(q1, 0.5)
		for _, r2 := range g.net.Roads {
			p2, q2 := g.net.Ends(r2.ID)
			b = p2.Lerp(q2, 0.5)
			if d := dist(a, b); d < 150 || d > 400 {
				continue
			}
			g.cursor = a
			g.roadTool(true, false)
			g.cursor = b
			g.roadTool(true, false)
			if len(g.net.Edits()) > 0 {
				built = true
				break
			}
		}
		if built {
			break
		}
	}
	if !built || len(g.net.Roads) <= before {
		t.Fatalf("no street built (%s)", g.message)
	}
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatalf("load: %v %v", ok, err)
	}
	if len(h.net.Roads) != len(g.net.Roads) || len(h.net.Edits()) != 1 {
		t.Fatalf("loaded %d roads, %d edits; want %d, 1", len(h.net.Roads), len(h.net.Edits()), len(g.net.Roads))
	}
	// Loading again starts from the map's own roads, not on top of edits.
	if ok, err := h.Load(); !ok || err != nil || len(h.net.Roads) != len(g.net.Roads) {
		t.Fatalf("second load: %v %v, %d roads", ok, err, len(h.net.Roads))
	}
	if err := h.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	h.Warp(120)
	if h.sim.CarCount() == 0 {
		t.Error("no traffic on the edited network")
	}
}

// Only the ground can be built on; the other four levels are for looking.
func TestOtherLevelsAreReadOnly(t *testing.T) {
	g := New("grid")
	g.zoom, g.page, g.tool = 1, pageBuild, toolTrack
	rd := g.net.Roads[5]
	a, b := g.net.Ends(rd.ID)
	g.cursor = a.Lerp(b, 0.5)
	for _, h := range []int{2, 1, -1, -2} {
		g.ShowLevel(h)
		g.buildTool(true, false)
		if g.net.Roads[5].Tram != street.NoTram {
			t.Fatalf("track built on level %d", h)
		}
	}
	g.ShowLevel(0)
	g.buildTool(true, false)
	if g.net.Roads[5].Tram == street.NoTram {
		t.Fatal("no track on the ground")
	}
}
