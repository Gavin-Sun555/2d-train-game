package traffic

import (
	"reflect"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

const minute = 60 * 60

func runChecked(t *testing.T, s *Sim, ticks int) {
	t.Helper()
	for i := 0; i < ticks; i++ {
		s.Step()
		if i%5 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.1fs: %v", s.Seconds(), err)
			}
		}
	}
}

func junctionAt(t *testing.T, n *street.Network, x, y float64) street.JunctionID {
	t.Helper()
	j, ok := n.NearestJunction(street.Vec{X: x, Y: y}, 2)
	if !ok {
		t.Fatalf("no junction at %.0f,%.0f", x, y)
	}
	return j
}

func layTram(t *testing.T, n *street.Network, mode street.TramMode, a, b street.Vec) []street.RoadID {
	t.Helper()
	path, ok := n.RoadPath(junctionAt(t, n, a.X, a.Y), junctionAt(t, n, b.X, b.Y))
	if !ok {
		t.Fatal("no road path")
	}
	n.SetTram(path, mode, false)
	return path
}

// tramNetwork: a shared-track line along a street and a dedicated-track
// line along a perpendicular street, crossing each other.
func tramNetwork(t *testing.T) *street.Network {
	n := street.Generate(city.Generate(1))
	shared := layTram(t, n, street.Shared, street.Vec{X: 170, Y: 170}, street.Vec{X: 650, Y: 170})
	dedicated := layTram(t, n, street.Dedicated, street.Vec{X: 330, Y: 20}, street.Vec{X: 330, Y: 500})
	stop := func(r street.RoadID) street.StopID {
		id, err := n.AddStop(r, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	for _, roads := range [][]street.RoadID{shared, dedicated} {
		if _, err := n.AddLine([]street.StopID{stop(roads[0]), stop(roads[len(roads)/2]), stop(roads[len(roads)-1])}); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

func TestCarsFlowWithoutCollisionsOrGridlock(t *testing.T) {
	s := New(street.Generate(city.Generate(1)), 250, 1)
	runChecked(t, s, 20*minute)
	t.Logf("trips=%d spawned=%d cars=%d maxStuck=%.0fs", s.Stats.Trips, s.Stats.Spawned, s.CarCount(), s.Stats.MaxStuck)
	if s.Stats.Trips < 400 {
		t.Errorf("only %d car trips completed in 20 min", s.Stats.Trips)
	}
	if s.Stats.GaveUp > 0 {
		t.Errorf("%d cars gave up at normal density (max wait %.0fs)", s.Stats.GaveUp, s.Stats.MaxStuck)
	}
}

func TestTramsServeTheirLinesInTraffic(t *testing.T) {
	n := tramNetwork(t)
	s := New(n, 200, 2)
	runChecked(t, s, 30*minute)
	byTram := map[int][]street.StopID{}
	for _, e := range s.Events {
		if e.Kind == TramArrive {
			byTram[e.Vehicle] = append(byTram[e.Vehicle], e.Stop)
		}
	}
	t.Logf("arrivals=%v trips=%d maxStuck=%.0fs", len(s.Events), s.Stats.Trips, s.Stats.MaxStuck)
	if len(byTram) != 2 {
		t.Fatalf("want 2 trams arriving, got %d", len(byTram))
	}
	lines := n.LineIDs()
	for i, v := range s.Vehicles[:2] {
		calls := n.Lines[lines[i]].Calls()
		got := byTram[v.ID]
		if len(got) < 12 {
			t.Errorf("tram %d made only %d stops", v.ID, len(got))
		}
		for k, st := range got {
			if want := calls[(k+1)%len(calls)]; st != want {
				t.Fatalf("tram %d arrival %d at stop %d, want %d (%v)", v.ID, k, st, want, got)
			}
		}
	}
}

func TestDeterministic(t *testing.T) {
	run := func() []Event {
		s := New(tramNetwork(t), 120, 9)
		for i := 0; i < 10*minute; i++ {
			s.Step()
		}
		return s.Events
	}
	if !reflect.DeepEqual(run(), run()) {
		t.Fatal("event logs differ between identical runs")
	}
}

// arrivalsInOrder checks every tram called at its line's stops in order.
func arrivalsInOrder(t *testing.T, s *Sim, n *street.Network, minStops int) {
	t.Helper()
	byTram := map[int][]street.StopID{}
	for _, e := range s.Events {
		if e.Kind == TramArrive {
			byTram[e.Vehicle] = append(byTram[e.Vehicle], e.Stop)
		}
	}
	for _, v := range s.Vehicles {
		if v.Kind != TramKind {
			continue
		}
		calls := n.Lines[v.Line].Calls()
		got := byTram[v.ID]
		if len(got) < minStops {
			t.Errorf("tram %d made only %d stops", v.ID, len(got))
		}
		for k, st := range got {
			if want := calls[(k+1)%len(calls)]; st != want {
				t.Fatalf("tram %d arrival %d at stop %d, want %d (%v)", v.ID, k, st, want, got)
			}
		}
	}
}

func addLine(t *testing.T, n *street.Network, roads ...street.RoadID) {
	t.Helper()
	var stops []street.StopID
	for _, r := range roads {
		id, err := n.AddStop(r, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		stops = append(stops, id)
	}
	if _, err := n.AddLine(stops); err != nil {
		t.Fatal(err)
	}
}

func TestSingleTrackShuttle(t *testing.T) {
	n := street.Generate(city.Generate(1))
	roads := layTramSingle(t, n, street.Vec{X: 170, Y: 170}, street.Vec{X: 810, Y: 170})
	addLine(t, n, roads[0], roads[len(roads)/2], roads[len(roads)-1])
	s := New(n, 200, 4)
	runChecked(t, s, 30*minute)
	t.Logf("arrivals=%d reversals=%d", s.Stats.TramArrivals, s.Stats.Reversals)
	arrivalsInOrder(t, s, n, 12)
	if s.Stats.Reversals < 6 {
		t.Errorf("tram changed ends only %d times", s.Stats.Reversals)
	}
}

func TestTwoLinesShareSingleTrackOneAtATime(t *testing.T) {
	n := street.Generate(city.Generate(1))
	// A single-track trunk with double track beyond each end, so trams can
	// wait clear of the single section.
	trunk := layTramSingle(t, n, street.Vec{X: 330, Y: 170}, street.Vec{X: 650, Y: 170})
	west := layTram(t, n, street.Dedicated, street.Vec{X: 170, Y: 170}, street.Vec{X: 330, Y: 170})
	east := layTram(t, n, street.Dedicated, street.Vec{X: 650, Y: 170}, street.Vec{X: 810, Y: 170})
	stop := func(r street.RoadID) street.StopID {
		id, err := n.AddStop(r, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	w, mid, e := stop(west[0]), stop(trunk[len(trunk)/2]), stop(east[0])
	for _, calls := range [][]street.StopID{{w, mid, e}, {e, mid, w}} {
		if _, err := n.AddLine(calls); err != nil {
			t.Fatal(err)
		}
	}
	s := New(n, 150, 5)
	runChecked(t, s, 30*minute)
	t.Logf("arrivals=%d reversals=%d", s.Stats.TramArrivals, s.Stats.Reversals)
	arrivalsInOrder(t, s, n, 8)
}

func TestDoubleTrackTerminusUsesCrossover(t *testing.T) {
	n := street.Generate(city.Generate(1))
	roads := layTram(t, n, street.Shared, street.Vec{X: 170, Y: 170}, street.Vec{X: 650, Y: 170})
	addLine(t, n, roads[0], roads[len(roads)-1])
	s := New(n, 250, 6)
	crossed := false
	for i := 0; i < 20*minute; i++ {
		s.Step()
		if i%5 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.1fs: %v", s.Seconds(), err)
			}
		}
		for _, v := range s.Vehicles {
			if v.Kind == TramKind && n.Links[v.Path[v.idx]].Kind == street.CrossoverLink {
				crossed = true
			}
		}
	}
	arrivalsInOrder(t, s, n, 8)
	if !crossed || s.Stats.Reversals < 4 {
		t.Fatalf("tram should turn back over the crossover (crossed=%v, reversals=%d)", crossed, s.Stats.Reversals)
	}
}

func layTramSingle(t *testing.T, n *street.Network, a, b street.Vec) []street.RoadID {
	t.Helper()
	path, ok := n.RoadPath(junctionAt(t, n, a.X, a.Y), junctionAt(t, n, b.X, b.Y))
	if !ok {
		t.Fatal("no road path")
	}
	n.SetTram(path, street.Dedicated, true)
	return path
}
