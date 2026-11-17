package traffic

import (
	"encoding/json"
	"math"
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
		id, err := n.AddStop(r, 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	for _, roads := range [][]street.RoadID{shared, dedicated} {
		stops := []street.StopID{stop(roads[0]), stop(roads[len(roads)/2]), stop(roads[len(roads)-1])}
		if n.AddEndCrossovers(stops) != 2 {
			t.Fatal("no room for crossovers at the ends")
		}
		if _, err := n.AddLine(stops); err != nil {
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
		id, err := n.AddStop(r, 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		stops = append(stops, id)
	}
	n.AddEndCrossovers(stops)
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
		id, err := n.AddStop(r, 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	w, mid, e := stop(west[0]), stop(trunk[len(trunk)/2]), stop(east[0])
	for _, calls := range [][]street.StopID{{w, mid, e}, {e, mid, w}} {
		n.AddEndCrossovers(calls)
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

func TestNoCrossoverShuttlesOnOneTrack(t *testing.T) {
	n := street.Generate(city.Generate(1))
	roads := layTram(t, n, street.Dedicated, street.Vec{X: 170, Y: 500}, street.Vec{X: 650, Y: 500})
	var stops []street.StopID
	for _, r := range []street.RoadID{roads[0], roads[len(roads)/2], roads[len(roads)-1]} {
		id, err := n.AddStop(r, 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		stops = append(stops, id)
	}
	id, err := n.AddLine(stops)
	if err != nil {
		t.Fatalf("a dedicated line without crossovers should shuttle: %v", err)
	}
	l := n.Lines[id]
	l.Trams = 3
	if c := MaxFleet(n, l); c != 1 {
		t.Fatalf("a shuttle on one track can run %d trams, want 1", c)
	}
	s := New(n, 150, 7)
	wrongWay := false
	for i := 0; i < 20*minute; i++ {
		s.Step()
		if i%5 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.1fs: %v", s.Seconds(), err)
			}
		}
		for _, v := range s.Vehicles {
			if v.Kind == TramKind && n.Links[v.Path[v.idx]].Contra {
				wrongWay = true
			}
			if v.Kind == TramKind && n.Links[v.Path[v.idx]].Kind == street.CrossoverLink {
				t.Fatal("no crossover was built")
			}
		}
	}
	arrivalsInOrder(t, s, n, 8)
	if !wrongWay || s.Stats.Reversals < 4 {
		t.Errorf("tram should run back along its own track (wrong way %v, reversals %d)", wrongWay, s.Stats.Reversals)
	}
}

func TestShortWorkingTurnsBackAtMidLineCrossover(t *testing.T) {
	n := street.Generate(city.Generate(1))
	roads := layTram(t, n, street.Dedicated, street.Vec{X: 170, Y: 500}, street.Vec{X: 980, Y: 500})
	stop := func(r street.RoadID) street.StopID {
		id, err := n.AddStop(r, 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	w, mid, e := stop(roads[0]), stop(roads[len(roads)/2]), stop(roads[len(roads)-1])
	n.AddEndCrossovers([]street.StopID{w, mid, e})
	// A crossover just past the middle stop lets a short line turn back
	// there while the long line runs through.
	if n.AddEndCrossovers([]street.StopID{w, mid}) == 0 {
		t.Fatal("no room for a mid-line crossover")
	}
	long, err := n.AddLine([]street.StopID{w, mid, e})
	if err != nil {
		t.Fatal(err)
	}
	short, err := n.AddLine([]street.StopID{w, mid})
	if err != nil {
		t.Fatal(err)
	}
	n.Lines[long].Trams, n.Lines[short].Trams = 2, 2
	if MaxFleet(n, n.Lines[short]) < 2 {
		t.Fatal("a line turning back over crossovers should run several trams")
	}
	s := New(n, 150, 8)
	runChecked(t, s, 30*minute)
	t.Logf("arrivals=%d reversals=%d", s.Stats.TramArrivals, s.Stats.Reversals)
	arrivalsInOrder(t, s, n, 8)
}

// A tram changes ends at the last platform while it stands there, instead
// of running on to the end of the track first.
func TestTramsChangeEndsAtTheLastPlatform(t *testing.T) {
	n := tramNetwork(t)
	s := New(n, 100, 3)
	ends := map[street.StopID]bool{}
	for _, l := range n.Lines {
		ends[l.Stops[0]], ends[l.Stops[len(l.Stops)-1]] = true, true
	}
	inPlace := 0
	for i := 0; i < 30*minute; i++ {
		before, events := s.Stats.Reversals, len(s.Events)
		s.Step()
		if i%5 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.1fs: %v", s.Seconds(), err)
			}
		}
		if s.Stats.Reversals == before {
			continue
		}
		departed := false
		for _, e := range s.Events[events:] {
			if e.Kind == TramDepart && ends[e.Stop] {
				departed = true
			}
		}
		if !departed {
			t.Fatalf("%.1fs: a tram changed ends away from a terminus platform", s.Seconds())
		}
		inPlace++
	}
	if inPlace < 6 {
		t.Fatalf("only %d changes of ends at termini", inPlace)
	}
}

func TestPassengersRideToTheirStops(t *testing.T) {
	n := tramNetwork(t)
	for _, l := range n.Lines {
		l.PerSection = 10 // small trams fill up
	}
	run := func(rate float64) *Sim {
		s := New(n, 100, 4)
		s.PassengerRate = rate
		for i := 0; i < 20*minute; i++ {
			s.Step()
			for _, v := range s.Vehicles {
				if v.Kind == TramKind && (v.Load() < 0 || v.Load() > s.Capacity(v)) {
					t.Fatalf("tram %d carries %d of %d", v.ID, v.Load(), s.Capacity(v))
				}
			}
		}
		onBoard := 0
		for _, v := range s.Vehicles {
			onBoard += v.Load()
		}
		if s.Passengers.Boarded != s.Passengers.Delivered+onBoard {
			t.Fatalf("boarded %d, delivered %d, on board %d", s.Passengers.Boarded, s.Passengers.Delivered, onBoard)
		}
		t.Logf("rate %.1f: boarded %d delivered %d waiting %d", rate, s.Passengers.Boarded, s.Passengers.Delivered, s.TotalWaiting())
		return s
	}
	few, many := run(PassengerRates[0]), run(PassengerRates[2])
	if few.Passengers.Delivered == 0 {
		t.Fatal("no passenger reached their stop")
	}
	if many.Passengers.Boarded+many.TotalWaiting() < 3*(few.Passengers.Boarded+few.TotalWaiting()) {
		t.Errorf("4x the arrival rate gave %d passengers vs %d", many.Passengers.Boarded+many.TotalWaiting(), few.Passengers.Boarded+few.TotalWaiting())
	}
}

func TestLineStatsAddUp(t *testing.T) {
	n := tramNetwork(t)
	s := New(n, 100, 5)
	for i := 0; i < 30*minute; i++ {
		s.Step()
	}
	boarded, delivered, trips := 0, 0, 0
	for _, id := range n.LineIDs() {
		st := s.LineStats(id)
		boarded += st.Boarded
		delivered += st.Delivered
		on := 0
		for _, k := range st.On {
			on += k
		}
		if on != st.Boarded {
			t.Errorf("line %d: %d boarded at stops, %d in all", id, on, st.Boarded)
		}
		if st.Trips == 0 || st.AvgTrip() <= 0 || st.Headway() <= 0 {
			t.Errorf("line %d: %d trips, avg %.0f s, headway %.0f s", id, st.Trips, st.AvgTrip(), st.Headway())
		}
		if v := st.Speed() * 3.6; v < 3 || v > 50 {
			t.Errorf("line %d: average speed %.1f km/h", id, v)
		}
		if lf := st.LoadFactor(); lf < 0 || lf > 1 {
			t.Errorf("line %d: load factor %.2f", id, lf)
		}
		// A one-way run is at least the line's length.
		if st.Distance < float64(st.Trips)*LineLength(n, n.Lines[id])*0.9 {
			t.Errorf("line %d: %.0f m for %d trips of %.0f m", id, st.Distance, st.Trips, LineLength(n, n.Lines[id]))
		}
		t.Logf("line %d: %d trips, avg %.0f s, %.1f km/h, headway %.0f s, load %.0f%%", id, st.Trips, st.AvgTrip(), st.Speed()*3.6, st.Headway(), 100*st.LoadFactor())
	}
	for _, v := range s.Vehicles {
		if v.Kind == TramKind {
			trips += v.Stats().Trips
		}
	}
	if boarded != s.Passengers.Boarded || delivered != s.Passengers.Delivered {
		t.Errorf("lines boarded %d delivered %d, sim %d / %d", boarded, delivered, s.Passengers.Boarded, s.Passengers.Delivered)
	}
	if trips == 0 {
		t.Error("no tram counted a trip")
	}
}

// The doors open on arrival; passengers get off and on only while they
// are fully open; they close only once nobody is left to get off and
// nobody waiting goes the tram's way (or it is full); the tram leaves only
// with its doors shut.
func TestDoorsFramePassengers(t *testing.T) {
	n := tramNetwork(t)
	s := New(n, 50, 7)
	s.PassengerRate = PassengerRates[2]
	type before struct {
		doors DoorState
		load  int
		dwell bool
	}
	prev := map[int]before{}
	closings, departs := 0, 0
	for i := 0; i < 30*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind != TramKind {
				continue
			}
			p, seen := prev[v.ID]
			prev[v.ID] = before{v.doors, v.Load(), v.Dwelling}
			if !seen {
				continue
			}
			if v.Load() != p.load && p.doors != DoorsOpen {
				t.Fatalf("%.0fs: tram %d's load changed with its doors %d", s.Seconds(), v.ID, p.doors)
			}
			if p.dwell && !v.Dwelling && !v.reversing {
				departs++
				if v.doors != DoorsClosed || (p.doors != DoorsClosing && p.doors != DoorsClosed) {
					t.Fatalf("%.0fs: tram %d left with its doors %d (were %d)", s.Seconds(), v.ID, v.doors, p.doors)
				}
			}
			if p.doors == DoorsOpen && v.doors == DoorsClosing {
				closings++
				if v.alightLeft != 0 {
					t.Fatalf("%.0fs: tram %d closed with %d still to get off", s.Seconds(), v.ID, v.alightLeft)
				}
				if l := n.Lines[v.Line]; v.Load() < s.Capacity(v) && s.nextBoarder(v, l) >= 0 {
					t.Fatalf("%.0fs: tram %d closed its doors on a passenger going its way", s.Seconds(), v.ID)
				}
			}
		}
	}
	if closings == 0 || departs == 0 {
		t.Fatalf("%d door closings, %d departures", closings, departs)
	}
}

// Passengers get on one by one, sections in parallel, at the line's
// seconds per passenger: a busy stop takes longer, and the load rises
// gradually rather than all at once.
func TestBoardingTakesTime(t *testing.T) {
	n := tramNetwork(t)
	for _, l := range n.Lines {
		l.BoardTenths, l.AlightTenths, l.Doors = 40, 20, 1 // 4 s / 2 s a passenger, one at a time per section
		l.DwellSecs, l.EndDwellSecs = 5, 10                // short least stops: the doors set the time
	}
	s := New(n, 0, 3)
	s.PassengerRate = PassengerRates[2]
	jumps, rises := 0, 0
	load := map[int]int{}
	longest := 0.0
	start := map[int]float64{}
	for i := 0; i < 40*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind != TramKind {
				continue
			}
			if d := v.Load() - load[v.ID]; d > 0 {
				rises++
				// At 4 s a passenger and one door a section, no more than
				// one per section can board in a tick.
				if d > v.Cars {
					jumps++
				}
			}
			load[v.ID] = v.Load()
			switch {
			case v.Dwelling && v.served && start[v.ID] == 0:
				start[v.ID] = s.Seconds()
			case !v.Dwelling && start[v.ID] > 0:
				longest = math.Max(longest, s.Seconds()-start[v.ID])
				start[v.ID] = 0
			}
		}
	}
	if rises == 0 {
		t.Fatal("nobody boarded")
	}
	if jumps > 0 {
		t.Errorf("%d times more than one passenger per section boarded in a tick", jumps)
	}
	if longest <= 15 {
		t.Errorf("no stop lasted longer than the least stop time (longest %.0f s)", longest)
	}
	t.Logf("%d boardings, longest stop %.0f s", rises, longest)
}

// A snapshot taken mid-run, written out and read back on a fresh
// simulation of the same network, carries on exactly as the original.
func TestSnapshotCarriesOn(t *testing.T) {
	n := tramNetwork(t)
	a := New(n, 80, 9)
	a.PassengerRate = PassengerRates[2]
	for i := 0; i < 7*minute; i++ {
		a.Step()
	}
	data, err := json.Marshal(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	b := New(n, 80, 1) // another seed: everything must come from the snapshot
	b.PassengerRate = PassengerRates[2]
	if err := b.Restore(&snap); err != nil {
		t.Fatal(err)
	}
	trams := 0
	for _, v := range b.Vehicles {
		if v.Kind == TramKind {
			trams++
		}
	}
	if trams == 0 || b.Seconds() != a.Seconds() || b.TotalWaiting() != a.TotalWaiting() {
		t.Fatalf("restored %d trams at %.0fs, %d waiting; want %.0fs, %d", trams, b.Seconds(), b.TotalWaiting(), a.Seconds(), a.TotalWaiting())
	}
	for i := 0; i < 3*minute; i++ {
		a.Step()
		b.Step()
	}
	if len(a.Vehicles) != len(b.Vehicles) {
		t.Fatalf("%d vehicles vs %d", len(a.Vehicles), len(b.Vehicles))
	}
	for i, v := range a.Vehicles {
		w := b.Vehicles[i]
		if v.ID != w.ID || v.Path[v.idx] != w.Path[w.idx] || math.Abs(v.S-w.S) > 1e-9 || v.Load() != w.Load() {
			t.Fatalf("vehicle %d differs after carrying on: link %d at %.2f (load %d) vs link %d at %.2f (load %d)",
				v.ID, v.Path[v.idx], v.S, v.Load(), w.Path[w.idx], w.S, w.Load())
		}
	}
	if a.Passengers != b.Passengers {
		t.Errorf("passengers %+v vs %+v", a.Passengers, b.Passengers)
	}
	// A different network is refused.
	other := New(n, 0, 1)
	snap.Links++
	if other.Restore(&snap) == nil {
		t.Error("a snapshot of another network was accepted")
	}
}

// A line drawn there and back again (A B C B A, as a player may click it)
// turns back at C as well as at A: passengers board only trams going their
// way, and the others wait for the tram to come back.
func TestThereAndBackLineBoardsByDirection(t *testing.T) {
	n := tramNetwork(t)
	ids := n.LineIDs()
	old := n.Lines[ids[1]]
	a, b, c := old.Stops[0], old.Stops[1], old.Stops[2]
	n.DeleteLine(old.ID)
	id, err := n.AddLine([]street.StopID{a, b, c, b, a})
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[id]
	if ends := l.Ends(); len(ends) < 2 || ends[0] != a || ends[1] != c {
		t.Fatalf("ends %v, want %d then %d", ends, a, c)
	}
	s := New(n, 30, 11)
	s.PassengerRate = PassengerRates[2]
	boarded := 0
	for i := 0; i < 40*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind != TramKind || v.Line != id || v.alightLeft > 0 {
				continue
			}
			calls := l.Calls()
			start := v.call
			if v.Dwelling && v.served {
				start++ // riders now aboard are for the stops after this one
			}
			ok := map[street.StopID]bool{}
			for k := 0; k < len(calls); k++ {
				j := (start + k) % len(calls)
				ok[calls[j]] = true
				if l.TurnsAt(j) {
					break
				}
			}
			for d, m := range v.riders {
				if m > 0 && !ok[d] {
					t.Fatalf("%.0fs: tram %d (next call %d) carries %d for stop %d, which is the other way", s.Seconds(), v.ID, v.call, m, d)
				}
			}
			boarded = max(boarded, v.Load())
		}
	}
	if boarded == 0 {
		t.Fatal("nobody rode the line")
	}
	if st := s.LineStats(id); st.Trips < 4 {
		t.Errorf("only %d one-way trips counted", st.Trips)
	}
}

// Trams stand at least the line's stop time at stops on the way, and the
// longer end time where they turn back (the driver changes ends); both are
// the player's to set.
func TestLeastStopTimes(t *testing.T) {
	for _, set := range []struct{ mid, end int }{{0, 0}, {30, 90}} {
		n := tramNetwork(t)
		for _, l := range n.Lines {
			l.DwellSecs, l.EndDwellSecs = set.mid, set.end
		}
		s := New(n, 0, 5)
		s.PassengerRate = PassengerRates[0]
		arrived := map[int]float64{}
		atEnd := map[int]bool{}
		checked := map[bool]int{}
		for i := 0; i < 40*minute; i++ {
			s.Step()
			for _, e := range s.Events {
				if e.Tick != s.Tick-1 {
					continue
				}
				v := s.Find(e.Vehicle)
				if v == nil {
					continue
				}
				l := n.Lines[v.Line]
				switch e.Kind {
				case TramArrive:
					arrived[v.ID], atEnd[v.ID] = s.Seconds(), l.TurnsAt(v.call)
				case TramDepart:
					if at, ok := arrived[v.ID]; ok {
						want := float64(l.Dwell())
						if atEnd[v.ID] {
							want = float64(l.EndDwell())
						}
						if got := s.Seconds() - at; got < want-0.1 {
							t.Fatalf("tram %d stood %.1f s (end %v), least %.0f s", v.ID, got, atEnd[v.ID], want)
						}
						checked[atEnd[v.ID]]++
						delete(arrived, v.ID)
					}
				}
			}
		}
		if checked[true] == 0 || checked[false] == 0 {
			t.Fatalf("settings %+v: stops checked %v", set, checked)
		}
		if l := n.Lines[n.LineIDs()[0]]; set.mid == 0 && (l.Dwell() != 20 || l.EndDwell() != 60) {
			t.Errorf("defaults %d / %d s, want 20 / 60", l.Dwell(), l.EndDwell())
		}
	}
}

// A tram entering service stops at the first stop for the line's normal
// least stop time (not the longer end time: it has no ends to change)
// before setting off.
func TestFirstStopBeforeSettingOff(t *testing.T) {
	n := tramNetwork(t)
	for _, l := range n.Lines {
		l.DwellSecs, l.EndDwellSecs = 25, 90
	}
	s := New(n, 0, 2)
	checked := 0
	for i := 0; i < 15*minute; i++ {
		s.Step()
		for _, e := range s.Events {
			if e.Kind != TramDepart || e.Tick != s.Tick-1 {
				continue
			}
			v := s.Find(e.Vehicle)
			if v == nil || v.Stats().Trips > 0 || v.Stats().Distance > 1 {
				continue
			}
			wait := s.Seconds() - v.Stats().Since
			if wait < 25-0.1 || wait > 90 {
				t.Fatalf("tram %d left the first stop after %.1f s, want the normal 25 s", v.ID, wait)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no tram set off")
	}
}

func TestTramModelsAndOperatingSpeedLimits(t *testing.T) {
	n := &street.Network{
		Junctions: []street.Junction{
			{ID: 0, Pos: street.Vec{X: 0, Y: 0}},
			{ID: 1, Pos: street.Vec{X: 1000, Y: 0}},
		},
		Roads: []street.Road{
			{ID: 0, A: 0, B: 1, Class: street.Boulevard},
		},
		Stops: map[street.StopID]*street.Stop{},
		Lines: map[street.LineID]*street.Line{},
	}
	n.Rebuild()
	n.SetTram([]street.RoadID{0}, street.Dedicated, false)
	a, err := n.AddStop(0, 0.08, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.AddStop(0, 0.92, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	n.AddEndCrossovers([]street.StopID{a, b})
	id, err := n.AddLine([]street.StopID{a, b})
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[id]
	l.Model = "kinkisharyo-p3010" // 95 km/h, 1.35 m/s^2
	l.MaxSpeed = 85

	s := New(n, 0, 12)
	peakHigh := 0.0
	for i := 0; i < 8*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind == TramKind {
				peakHigh = math.Max(peakHigh, v.V*3.6)
				if v.V*3.6 > 85.5 {
					t.Fatalf("tram exceeded 85 km/h operating cap: %.2f km/h", v.V*3.6)
				}
			}
		}
	}
	if peakHigh < 62 {
		t.Fatalf("high-speed LRV on dedicated track only reached %.1f km/h", peakHigh)
	}

	// Lower operating speed live without restarting Sim; once slowed down it
	// must never exceed the new 30 km/h limit.
	l.MaxSpeed = 30
	for i := 0; i < 30*60; i++ {
		s.Step() // allow deceleration from high speed
	}
	peakCapped := 0.0
	for i := 0; i < 6*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind == TramKind {
				peakCapped = math.Max(peakCapped, v.V*3.6)
				if v.V*3.6 > 30.5 {
					t.Fatalf("tram exceeded live 30 km/h operating cap: %.2f km/h", v.V*3.6)
				}
			}
		}
	}
	if peakCapped < 25 {
		t.Fatalf("capped tram only reached %.1f km/h, expected ~30 km/h", peakCapped)
	}
}

// Lowering per-section capacity during live service does not drop riders on
// board: while a tram is above or at the new limit, riders for intermediate
// stops still get off and nobody boards ("alight only") until the load drops
// strictly below the new capacity, after which boarding resumes up to the new cap.
func TestLiveCapacityReductionAlightsOnlyUntilBelowLimit(t *testing.T) {
	n := street.Generate(city.Generate(1))
	roads := layTram(t, n, street.Dedicated, street.Vec{X: 170, Y: 500}, street.Vec{X: 980, Y: 500})
	var stops []street.StopID
	for _, k := range []int{0, 1, 2, 3, len(roads) - 1} {
		st, err := n.AddStop(roads[k], 0.5, street.DefaultPlatform)
		if err != nil {
			t.Fatal(err)
		}
		stops = append(stops, st)
	}
	n.AddEndCrossovers(stops)
	id, err := n.AddLine(stops)
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[id]
	l.Sections = 3
	l.PerSection = 30 // initial capacity = 90
	l.BoardTenths, l.AlightTenths = 5, 5

	s := New(n, 0, 19)
	s.PassengerRate = PassengerRates[2]
	// Pre-fill stops so trams quickly load well above 30 riders, with mixed destinations.
	for _, q := range s.queues {
		for i := 0; i < 60; i++ {
			q.waiting = append(q.waiting, q.dests[i%len(q.dests)])
		}
	}

	var tram *Vehicle
	for i := 0; i < 15*minute; i++ {
		s.Step()
		for _, v := range s.Vehicles {
			if v.Kind == TramKind && !v.Dwelling && v.Load() >= 45 {
				tram = v
				break
			}
		}
		if tram != nil {
			break
		}
	}
	if tram == nil {
		t.Fatal("tram never reached >= 45 passengers")
	}

	// Lower capacity live from 90 down to 30 (10 per section × 3 sections) while carrying >= 45!
	l.PerSection = 10
	newCap := s.Capacity(tram)
	if newCap != 30 || tram.Load() <= newCap {
		t.Fatalf("expected load > %d, got %d", newCap, tram.Load())
	}

	prevLoad := tram.Load()
	sawAlightWhileOver := false
	sawBoardAfterRecovery := false
	for i := 0; i < 25*minute; i++ {
		wasOverOrFull := prevLoad >= newCap
		s.Step()
		curLoad := tram.Load()
		if wasOverOrFull {
			if curLoad > prevLoad {
				t.Fatalf("over-capacity tram boarded riders: %d -> %d (cap %d)", prevLoad, curLoad, newCap)
			}
			if curLoad < prevLoad && curLoad > newCap {
				sawAlightWhileOver = true
			}
		} else {
			if curLoad > newCap {
				t.Fatalf("tram exceeded new capacity %d: got %d", newCap, curLoad)
			}
			if curLoad > prevLoad {
				sawBoardAfterRecovery = true
			}
		}
		if tram.Dwelling && curLoad >= newCap && s.Boarding(tram) != 0 {
			t.Fatalf("over-capacity tram (load %d, cap %d) reports %d boarding", curLoad, newCap, s.Boarding(tram))
		}
		prevLoad = curLoad
	}
	if !sawAlightWhileOver {
		t.Error("expected passengers to alight at stops while still above the lowered capacity")
	}
	if !sawBoardAfterRecovery {
		t.Error("expected boarding to resume once load dropped below the lowered capacity")
	}
}

// A tram line running across ground streets, an elevated viaduct (+1), an
// off-road corridor crossing a car street at grade (0), and an underground
// tunnel (-1) coexists safely with AI road traffic: signalised portals and
// level crossings prevent car-tram collisions, trams serve stops at every
// elevation, and snapshots restore cleanly.
func TestMultiLevelAndOffRoadTramServiceWithRoadTraffic(t *testing.T) {
	n := street.Generate(city.Generate(1))
	startRoads := layTram(t, n, street.Shared, street.Vec{X: 170, Y: 170}, street.Vec{X: 330, Y: 170})
	endRoads := layTram(t, n, street.Dedicated, street.Vec{X: 1290, Y: 330}, street.Vec{X: 1460, Y: 330})

	mustTw := func(a, b street.Vec, la, lb int) {
		t.Helper()
		if err := n.AddTramway(a, b, la, lb, false, 12); err != nil {
			t.Fatalf("AddTramway(%v lv%d -> %v lv%d): %v", a, la, b, lb, err)
		}
	}
	mustTw(street.Vec{X: 330, Y: 170}, street.Vec{X: 390, Y: 230}, 0, 1)
	mustTw(street.Vec{X: 390, Y: 230}, street.Vec{X: 730, Y: 230}, 1, 1)
	mustTw(street.Vec{X: 730, Y: 230}, street.Vec{X: 785, Y: 250}, 1, 0)
	mustTw(street.Vec{X: 785, Y: 250}, street.Vec{X: 920, Y: 250}, 0, 0) // crosses X=810 car street
	mustTw(street.Vec{X: 920, Y: 250}, street.Vec{X: 965, Y: 285}, 0, -1)
	mustTw(street.Vec{X: 965, Y: 285}, street.Vec{X: 1230, Y: 285}, -1, -1)
	mustTw(street.Vec{X: 1230, Y: 285}, street.Vec{X: 1290, Y: 330}, -1, 0)

	s0, err := n.AddStop(startRoads[0], 0.5, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	elevRd, elevT, _ := n.NearestRoadAt(street.Vec{X: 560, Y: 230}, 1, 6)
	s1, err := n.AddStop(elevRd, elevT, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	offRd, offT, _ := n.NearestRoadAt(street.Vec{X: 865, Y: 250}, 0, 6)
	s2, err := n.AddStop(offRd, offT, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	tunRd, tunT, _ := n.NearestRoadAt(street.Vec{X: 1100, Y: 285}, -1, 6)
	s3, err := n.AddStop(tunRd, tunT, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	s4, err := n.AddStop(endRoads[len(endRoads)-1], 0.5, street.DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	n.AddEndCrossovers([]street.StopID{s0, s4})
	lineID, err := n.AddLine([]street.StopID{s0, s1, s2, s3, s4})
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[lineID]
	l.Trams = 4
	l.Model = "crrc-suzhou"

	s := New(n, 45, 29)
	s.PassengerRate = PassengerRates[2]

	servedByLevel := map[int]int{}
	var sawElev, sawUnder, sawRampUp, sawRampDown bool
	for i := 0; i < 28*minute; i++ {
		s.Step()
		if i%5 == 0 {
			if err := s.CheckInvariants(); err != nil {
				t.Fatalf("%.1fs: %v", s.Seconds(), err)
			}
		}
		for _, v := range s.Vehicles {
			if v.Kind != TramKind {
				continue
			}
			z := n.LinkElevation(v.Path[v.idx], v.S)
			switch {
			case z > 0.99:
				sawElev = true
			case z < -0.99:
				sawUnder = true
			case z > 0.15 && z < 0.85:
				sawRampUp = true
			case z < -0.15 && z > -0.85:
				sawRampDown = true
			}
			if v.Dwelling {
				st := n.Stops[l.Calls()[v.call]]
				servedByLevel[n.StopLevel(st)]++
			}
		}
	}

	if !sawElev || !sawUnder || !sawRampUp || !sawRampDown {
		t.Fatalf("elevations visited: elev=%v under=%v rampUp=%v rampDown=%v", sawElev, sawUnder, sawRampUp, sawRampDown)
	}
	for _, lv := range []int{-1, 0, 1} {
		if servedByLevel[lv] == 0 {
			t.Errorf("no tram dwelled at level %d stop (counts: %v)", lv, servedByLevel)
		}
	}
	if s.Passengers.Delivered == 0 {
		t.Error("no passengers completed journeys on the multi-level line")
	}

	// Verify Snapshot / Restore works cleanly across multi-level links.
	snap := s.Snapshot()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Snapshot
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	clone := New(n, 45, 99)
	if err := clone.Restore(&loaded); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	runChecked(t, clone, 2*minute)
}
