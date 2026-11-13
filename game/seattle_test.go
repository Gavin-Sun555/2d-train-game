package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

func TestSeattleStreetcarDemo(t *testing.T) {
	g := New("seattle")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	ids := g.net.LineIDs()
	if len(ids) != 1 || len(g.net.Lines[ids[0]].Stops) < 8 {
		t.Fatalf("want the SLU line with 8 stops, got %d lines", len(ids))
	}
	s := g.sim
	for i := 0; i < 20*60*60; i++ {
		s.Step()
		if i%10 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.0fs: %v", s.Seconds(), err)
			}
		}
	}
	seen := map[string]bool{}
	for _, e := range s.Events {
		if e.Kind == traffic.TramArrive {
			seen[g.net.Stops[e.Stop].Name] = true
		}
	}
	if len(seen) < 6 {
		t.Errorf("streetcar served only %d stops in 20 min: %v", len(seen), seen)
	}
	if s.Stats.GaveUp > 2 {
		t.Errorf("%d cars gave up", s.Stats.GaveUp)
	}
}
