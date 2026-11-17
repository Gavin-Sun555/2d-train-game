package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

func TestLandmarksRenameAndSave(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("seattle")
	needle := -1
	for i, l := range g.scene.landmarks {
		if l.key == "Space Needle" {
			needle = i
		}
	}
	if needle < 0 || g.LandmarkName(needle) != "太空针塔" {
		t.Fatalf("Space Needle not labelled (%d landmarks)", len(g.scene.landmarks))
	}
	if err := g.RenameLandmark(needle, "  我的太空针  "); err != nil {
		t.Fatal(err)
	}
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}
	h := New("seattle")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatalf("load: %v %v", ok, err)
	}
	if n := h.LandmarkName(needle); n != "我的太空针" {
		t.Fatalf("loaded name %q", n)
	}
	if err := h.RenameLandmark(needle, ""); err != nil || h.LandmarkName(needle) != "太空针塔" {
		t.Fatalf("empty name should restore the default, got %q (%v)", h.LandmarkName(needle), err)
	}
	if err := h.RenameLandmark(needle, "a\nb"); err == nil {
		t.Error("a name with a line break was accepted")
	}
}

func TestNoTrackOnMotorways(t *testing.T) {
	g := New("seattle")
	g.zoom = 1
	found := 0
	for i := range g.scene.highways {
		h := &g.scene.highways[i]
		if h.link || len(h.pts) < 2 {
			continue
		}
		p := h.pts[0].Lerp(h.pts[1], 0.5)
		if _, _, near := g.net.NearestRoad(p, 15); near {
			continue // a street crosses here
		}
		if _, near := g.net.NearestJunction(p, 30); near || h.level < 0 {
			continue // a ramp meeting a street, or a tunnel
		}
		g.cursor = p
		if _, ok := g.onHighway(); !ok {
			t.Fatalf("cursor on motorway at %.0f,%.0f not recognised", p.X, p.Y)
		}
		if roads := g.trackPreview(); roads != nil {
			t.Fatalf("track offered on a motorway: %v", roads)
		}
		g.tool = toolTrack
		g.buildTool(true, false)
		if g.message != highwayRefusal() {
			t.Fatalf("message %q", g.message)
		}
		found++
		if found > 20 {
			break
		}
	}
	if found == 0 {
		t.Fatal("no motorway on the map")
	}
	for _, rd := range g.net.Roads {
		if rd.Tram != street.NoTram {
			t.Fatal("track was laid")
		}
	}
}

// On the Plan page a stop can be renamed (by its label or N); the name is
// saved and shown on the line; an empty or broken name is refused.
func TestRenameStop(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.ShowPage("plan")
	id := g.net.Lines[g.net.LineIDs()[0]].Stops[0]
	g.openStopRename(id)
	if g.rename == nil || g.rename.stop != id {
		t.Fatal("rename window not open")
	}
	if g.RenameStop(id, "   ") == nil || g.RenameStop(id, "a\nb") == nil {
		t.Error("an empty or broken name was accepted")
	}
	if err := g.RenameStop(id, "  中央车站  "); err != nil || g.net.Stops[id].Name != "中央车站" {
		t.Fatalf("rename: %v, %q", err, g.net.Stops[id].Name)
	}
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if n := h.net.Stops[id].Name; n != "中央车站" {
		t.Errorf("loaded name %q", n)
	}
}
