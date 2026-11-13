package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

func TestServiceRestartsOnlyOnRunPageAfterEdits(t *testing.T) {
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.setPage(pageRun)
	g.Warp(60)
	before := g.sim
	g.setPage(pageBuild)
	g.net.SetTram([]street.RoadID{0}, street.Shared, false)
	g.edited()
	if g.sim != before {
		t.Fatal("editing must not restart the service before Run")
	}
	g.setPage(pagePlan)
	if routes := g.lineRoutes(); len(routes) != len(g.net.Lines) {
		t.Fatalf("plan page traced %d of %d lines", len(routes), len(g.net.Lines))
	}
	g.setPage(pageRun)
	if g.sim == before || g.sim.Seconds() != 0 || g.dirty {
		t.Fatal("entering Run after edits should restart the service")
	}
}

func TestFollowTracksATram(t *testing.T) {
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.setPage(pageRun)
	g.Warp(120)
	g.followNext()
	if g.follow == 0 {
		t.Fatal("no tram to follow")
	}
	first := g.follow
	for i := 0; i < 600; i++ {
		g.sim.Step()
		g.updateFollow()
	}
	v := g.sim.Find(g.follow)
	mid, _ := along(g.sim.Body(v), v.Length/2)
	if d := dist(mid, street.Vec{X: g.camX, Y: g.camY}); d > 15 {
		t.Errorf("camera %.0f m from the followed tram", d)
	}
	g.followNext()
	if g.follow == first {
		t.Error("F should move on to the next tram")
	}
	g.setPage(pageBuild)
	if g.follow != 0 {
		t.Error("leaving Run should stop following")
	}
}
