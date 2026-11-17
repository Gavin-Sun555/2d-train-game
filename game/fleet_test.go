package game

import (
	"strings"
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
					if c := g.net.TramSections(l); v.Cars != c || v.Length != street.SetLength(c) {
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
				t.Logf("line %d: %d trams × %d sections, cycle ≈ %.0f s", id, n, g.net.TramSections(l), traffic.CycleTime(g.net, l))
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
		maxCars, _ := g.net.LineCapacity(l)
		if l.Fleet() != want || l.Cars() != maxCars {
			t.Errorf("line %d: %d trams × %d sections after raising, want %d × %d", id, l.Fleet(), l.Cars(), want, maxCars)
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

// While a tram stands at a stop its status says who is getting off or on,
// and the boarding settings are kept in the save.
func TestDoorStatusAndServiceSettings(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.SelectLine(0)
	g.configureService(2, 1, 1) // 3.0 s on, 1.5 s off, 3 at once
	l := g.net.Lines[g.net.LineIDs()[0]]
	if l.BoardSeconds() != 3 || l.AlightSeconds() != 1.5 || l.DoorsPerSection() != 3 {
		t.Fatalf("settings %.1f %.1f %d", l.BoardSeconds(), l.AlightSeconds(), l.DoorsPerSection())
	}
	g.sim.PassengerRate = traffic.PassengerRates[2]
	g.Run()
	seen := map[string]bool{}
	for i := 0; i < 20*60*60 && len(seen) < 2; i++ {
		g.sim.Step()
		for _, v := range g.sim.Vehicles {
			if v.Kind == traffic.TramKind && v.Dwelling {
				s := g.doorState(v)
				switch {
				case strings.Contains(s, "下车中"):
					seen["off"] = true
				case strings.Contains(s, "上车中"):
					seen["on"] = true
				}
			}
		}
	}
	if !seen["off"] || !seen["on"] {
		t.Errorf("door states seen: %v", seen)
	}
	if err := g.Save(true); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if hl := h.net.Lines[l.ID]; hl.BoardSeconds() != 3 || hl.AlightSeconds() != 1.5 || hl.DoorsPerSection() != 3 {
		t.Error("boarding settings not saved")
	}
}

func TestLoadAndSpeedApplyLiveWhileModelRestartsOnRun(t *testing.T) {
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.Run()
	g.Warp(120)
	beforeSim := g.sim
	beforeSec := g.sim.Seconds()

	g.OpenLinePanel()
	g.SelectLine(0)
	l := g.net.Lines[g.selLine]

	// Adjusting load and operating speed must NOT mark dirty or reset service.
	g.configureLoad(-2)
	g.configureSpeed(-2)
	if g.dirty {
		t.Fatal("changing load or operating speed marked the network dirty")
	}
	// Trying to raise speed above the vehicle's own top speed clamps at MaxSpeedKmh.
	for i := 0; i < 20; i++ {
		g.configureSpeed(1)
	}
	if l.OperatingSpeedKmh() != l.TramModel().MaxSpeedKmh {
		t.Fatalf("operating speed %d != model max %d", l.OperatingSpeedKmh(), l.TramModel().MaxSpeedKmh)
	}
	g.closeLinePanel()
	if g.sim != beforeSim || g.sim.Seconds() < beforeSec {
		t.Fatal("closing line panel after load/speed adjustments restarted the service")
	}

	// Switching rolling stock clamps operating speed to the new model's limit and restarts on close.
	g.OpenLinePanel()
	g.setLineModel(street.TramModelIndex("siemens-s700")) // 90 km/h
	g.configureSpeed(-1)                                  // 85 km/h
	if l.OperatingSpeedKmh() != 85 {
		t.Fatalf("operating speed %d, want 85", l.OperatingSpeedKmh())
	}
	g.setLineModel(street.TramModelIndex("toyama-centram")) // 50 km/h
	if l.OperatingSpeedKmh() != 50 {
		t.Fatalf("switching to 50 km/h tram gave operating speed %d (raw %d)", l.OperatingSpeedKmh(), l.MaxSpeed)
	}
	if !g.dirty {
		t.Fatal("changing tram model should mark service for restart")
	}
	g.closeLinePanel()
	if g.sim == beforeSim || g.dirty {
		t.Fatal("closing line panel after changing tram model should restart service")
	}
}
