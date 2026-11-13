package game

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

func TestLineFleetsRunTogether(t *testing.T) {
	for _, m := range []string{"grid", "seattle"} {
		t.Run(m, func(t *testing.T) {
			g := New(m)
			if err := g.LoadDemo(); err != nil {
				t.Fatal(err)
			}
			for i, id := range g.net.LineIDs() {
				l := g.net.Lines[id]
				l.Trams, l.Sections = 3+i%2, street.MinSections+i%4
			}
			g.rebuild()
			s := g.sim
			const minutes = 30
			for i := 0; i < minutes*3600; i++ {
				s.Step()
				if i%7 == 0 {
					if err := s.CheckInvariants(); err != nil {
						t.Fatalf("%.0fs: %v", s.Seconds(), err)
					}
				}
			}
			arrivals := map[int]int{}
			for _, e := range s.Events {
				if e.Kind == traffic.TramArrive {
					arrivals[e.Vehicle]++
				}
			}
			for _, id := range g.net.LineIDs() {
				l := g.net.Lines[id]
				if w := s.Waiting(id); w > 0 {
					t.Errorf("line %d: %d trams never entered service", id, w)
				}
				n := 0
				for _, v := range s.Vehicles {
					if v.Kind != traffic.TramKind || v.Line != id {
						continue
					}
					n++
					if v.Cars != l.Cars() || v.Length != street.SetLength(l.Cars()) {
						t.Errorf("tram %d: %d sections, %.0f m", v.ID, v.Cars, v.Length)
					}
					if arrivals[v.ID] < 3 {
						t.Errorf("line %d tram %d made only %d stops in %d min", id, v.ID, arrivals[v.ID], minutes)
					}
				}
				want := min(l.Fleet(), traffic.MaxFleet(g.net, l))
				if n != want {
					t.Errorf("line %d: %d trams running, want %d", id, n, want)
				}
				t.Logf("line %d: %d trams × %d sections, cycle ≈ %.0f s", id, n, l.Cars(), traffic.CycleTime(g.net, l))
			}
		})
	}
}

func TestPlanPageConfigClamps(t *testing.T) {
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.setPage(pagePlan)
	for _, id := range g.net.LineIDs() {
		g.selLine = id
		l := g.net.Lines[id]
		for i := 0; i < 20; i++ {
			g.configure(1, 1)
		}
		want := min(street.MaxTrams, traffic.MaxFleet(g.net, l))
		if l.Fleet() != want || l.Cars() != street.MaxSections {
			t.Errorf("line %d: %d trams × %d sections after raising, want %d × %d", id, l.Fleet(), l.Cars(), want, street.MaxSections)
		}
		for i := 0; i < 20; i++ {
			g.configure(-1, -1)
		}
		if l.Fleet() != 1 || l.Cars() != street.MinSections {
			t.Errorf("line %d: %d trams × %d sections after lowering", id, l.Fleet(), l.Cars())
		}
	}
	if !g.dirty {
		t.Error("config changes should restart the service on Run")
	}
}
