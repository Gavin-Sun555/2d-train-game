package game

import (
	"fmt"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// A save made on an earlier, smaller Seattle map loads on the current one
// with its track, stops and line in the same places.
func TestOldSeattleSavesCarryOver(t *testing.T) {
	for i, o := range oldSeattles {
		t.Run(fmt.Sprint("v", len(oldSeattles)-i), func(t *testing.T) { testCarryOver(t, o) })
	}
}

func testCarryOver(t *testing.T, o oldSeattle) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	old := newOnMap("seattle", o.load())
	old.net = street.FromMapLegacy(o.load()) // as that version built it
	old.rebuild()
	if err := old.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	old.SetCamera(700, 1400, 1.5)
	if err := old.Save(true); err != nil {
		t.Fatal(err)
	}

	g := New("seattle")
	if ok, err := g.Load(); !ok || err != nil {
		t.Fatalf("load: %v, %v", ok, err)
	}
	off := o.off
	if g.camX != 700+off[0] || g.camY != 1400+off[1] {
		t.Errorf("camera at %.0f,%.0f", g.camX, g.camY)
	}
	if len(g.net.Stops) != len(old.net.Stops) || len(g.net.Lines) != 1 {
		t.Fatalf("%d stops, %d lines; want %d, 1", len(g.net.Stops), len(g.net.Lines), len(old.net.Stops))
	}
	for id, st := range old.net.Stops {
		p, q := old.net.StopPos(st), g.net.StopPos(g.net.Stops[id])
		if d := dist(q, add(p, street.Vec{X: off[0], Y: off[1]})); d > 1 {
			t.Errorf("%s moved %.1f m", st.Name, d)
		}
	}
	track := func(gm *Game) (m float64) {
		for _, rd := range gm.net.Roads {
			if rd.Tram != street.NoTram {
				m += gm.net.Length(rd.ID)
			}
		}
		return m
	}
	if a, b := track(old), track(g); b < a*0.98 || b > a*1.02 {
		t.Errorf("track %.0f m, was %.0f m", b, a)
	}
	g.Warp(600)
	if g.sim.Passengers.Boarded == 0 && len(g.sim.Events) == 0 {
		t.Error("nothing ran on the converted line")
	}
}
