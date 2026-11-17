package street

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/city"
)

func gen() *Network { return Generate(city.Generate(1)) }

func TestGeneratedNetworkHasAllClassesAndNoDeadLanes(t *testing.T) {
	n := gen()
	classes := map[Class]int{}
	for _, r := range n.Roads {
		classes[r.Class]++
		for _, fwd := range []bool{true, false} {
			for i := 0; i < r.Class.LanesPerDir(); i++ {
				if n.Lane(r.ID, fwd, i) < 0 {
					t.Fatalf("road %d missing lane %v/%d", r.ID, fwd, i)
				}
			}
		}
	}
	for _, c := range []Class{Street, Avenue, Boulevard} {
		if classes[c] == 0 {
			t.Errorf("no %s roads generated", c)
		}
	}
	for _, l := range n.Links {
		if l.Kind == LaneLink && l.Modes&Car != 0 && len(l.Next) == 0 {
			t.Errorf("lane %d (road %d) has no way out", l.ID, l.Road)
		}
	}
	if len(n.signals) == 0 {
		t.Error("no signalised junctions")
	}
}

func TestConflictsAreSymmetricAndOpposingPhasesNeverBothGreen(t *testing.T) {
	n := gen()
	for _, l := range n.Links {
		for _, c := range l.Conflicts {
			found := false
			for _, back := range n.Links[c].Conflicts {
				found = found || back == l.ID
			}
			if !found {
				t.Fatalf("conflict %d→%d not symmetric", l.ID, c)
			}
		}
	}
	for _, sig := range n.signals {
		for tt := 0.0; tt < 2*CycleTime; tt += 0.5 {
			if sig.AspectAt(0, tt) != Red && sig.AspectAt(1, tt) != Red {
				t.Fatalf("both phases open at t=%.1f", tt)
			}
		}
	}
}

func roadBetween(t *testing.T, n *Network, ax, ay, bx, by float64) []RoadID {
	t.Helper()
	a, _ := n.NearestJunction(Vec{X: ax, Y: ay}, 2)
	b, _ := n.NearestJunction(Vec{X: bx, Y: by}, 2)
	p, ok := n.RoadPath(a, b)
	if !ok {
		t.Fatal("no path")
	}
	return p
}

func TestTramTrackTakesInnerLane(t *testing.T) {
	n := gen()
	shared := roadBetween(t, n, 170, 170, 330, 170)
	dedicated := roadBetween(t, n, 330, 500, 650, 500) // along the boulevard
	n.SetTram(shared, Shared, false)
	n.SetTram(dedicated, Dedicated, false)
	s := n.Links[n.Lane(shared[0], true, 0)]
	if s.Modes != Car|Tram || !s.Track {
		t.Errorf("shared lane modes %b", s.Modes)
	}
	d := n.Links[n.Lane(dedicated[0], true, 0)]
	if d.Modes != Tram {
		t.Errorf("dedicated lane modes %b", d.Modes)
	}
	if o := n.Links[n.Lane(dedicated[0], true, 1)]; o.Modes != Car {
		t.Errorf("outer boulevard lane should stay car-only, got %b", o.Modes)
	}
	// No crossovers until the player builds one; every track piece can be
	// run both ways.
	count := func() (crossovers, contra int) {
		for _, l := range n.Links {
			if l.Kind == CrossoverLink {
				crossovers++
			}
			if l.Contra && l.Kind == LaneLink {
				contra++
			}
		}
		return
	}
	if c, k := count(); c != 0 || k == 0 || len(n.Blocks) != 0 {
		t.Errorf("want no crossovers or blocks and wrong-way track; got %d, %d, %d", c, k, len(n.Blocks))
	}
	if err := n.AddCrossover(shared[0], 0.5); err != nil {
		t.Fatal(err)
	}
	if err := n.AddCrossover(shared[0], 0.52); err == nil {
		t.Error("crossovers on top of each other accepted")
	}
	if c, _ := count(); c != 4 {
		t.Errorf("a scissors crossover is 4 movements, got %d", c)
	}
	if !n.RemoveCrossover(shared[0], 0.49, 5) {
		t.Error("crossover not removed")
	}
	if c, _ := count(); c != 0 {
		t.Errorf("%d crossover links left", c)
	}
}

func TestSingleTrackRunsDownTheCentre(t *testing.T) {
	n := gen()
	roads := roadBetween(t, n, 170, 170, 650, 170)
	before := n.Roads[roads[0]].HalfWidth()
	n.SetTram(roads, Shared, true)
	rd := n.Roads[roads[0]]
	if rd.Tram != Dedicated || !rd.Single {
		t.Fatalf("single track should be dedicated, got %v single=%v", rd.Tram, rd.Single)
	}
	if rd.HalfWidth() <= before {
		t.Errorf("centre track should widen the road")
	}
	f := n.CentreTrack(roads[0], true)
	if n.Links[f].Twin != n.CentreTrack(roads[0], false) || n.Links[f].Block < 0 {
		t.Fatalf("centre tracks must be twins in a block")
	}
	if len(n.Blocks) != 1 {
		t.Errorf("one connected single-track section should be one block, got %d", len(n.Blocks))
	}
	for _, l := range n.Links {
		if l.Kind == LaneLink && l.Modes&Car != 0 && l.Track {
			t.Fatalf("car lane %d marked as track on a single-track road", l.ID)
		}
	}
}

func TestStopsAndLines(t *testing.T) {
	n := gen()
	roads := roadBetween(t, n, 170, 170, 650, 170)
	if _, err := n.AddStop(roads[0], 0.5, DefaultPlatform); err == nil {
		t.Fatal("stop on road without track accepted")
	}
	n.SetTram(roads, Shared, false)
	if _, err := n.AddStop(roads[0], 0.02, DefaultPlatform); err == nil {
		t.Fatal("stop next to junction accepted")
	}
	a, err := n.AddStop(roads[0], 0.5, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.AddStop(roads[len(roads)-1], 0.5, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.AddLine([]StopID{a, b}); err == nil {
		t.Fatal("a line on shared track with no crossover cannot turn back")
	}
	if n.AddEndCrossovers([]StopID{a, b}) != 2 {
		t.Fatal("no room for end crossovers")
	}
	if _, err := n.AddLine([]StopID{a, b}); err != nil {
		t.Fatalf("line rejected: %v", err)
	}
	n.SetTram(roads[:1], NoTram, false)
	if len(n.Stops) != 1 || len(n.Lines) != 0 {
		t.Fatalf("removing track should drop its stop and the line: %d stops, %d lines", len(n.Stops), len(n.Lines))
	}
}

func TestPlatformLengthsLimitTheLine(t *testing.T) {
	n := gen()
	roads := roadBetween(t, n, 170, 170, 650, 170)
	n.SetTram(roads, Dedicated, false)
	a, err := n.AddStop(roads[0], 0.5, 8)
	if err != nil {
		t.Fatal(err)
	}
	mid, err := n.AddStop(roads[len(roads)/2], 0.5, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.AddStop(roads[len(roads)-1], 0.27, 6)
	if err != nil {
		t.Fatal(err)
	}
	id, err := n.AddLine([]StopID{a, mid, b})
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[id]
	l.Sections = 7
	if c, why := n.LineCapacity(l); c != 2 || n.TramSections(l) != 2 {
		t.Fatalf("capacity %d (%s), want 2 from the shortest platform", c, why)
	}
	if err := n.SetStopSections(mid, 7); err != nil {
		t.Fatal(err)
	}
	if c, why := n.LineCapacity(l); c != 6 {
		t.Fatalf("capacity %d (%s), want 6", c, why)
	}
	for _, p := range n.Platforms(n.Stops[a]) {
		if p.Len != PlatformLength(8) {
			t.Errorf("platform %.0f m, want %.0f", p.Len, PlatformLength(8))
		}
	}
	if err := n.SetStopSections(a, 99); err != nil {
		t.Logf("8 sections clamped: %v", err)
	}
	if n.Stops[a].Capacity() > MaxSections {
		t.Error("platform longer than the longest tram")
	}

	saved := n.Export()
	m := gen()
	if err := m.Import(saved); err != nil {
		t.Fatal(err)
	}
	if len(m.Stops) != 3 || len(m.Lines) != 1 || len(m.Links) != len(n.Links) {
		t.Fatalf("import: %d stops, %d lines, %d links (want 3, 1, %d)", len(m.Stops), len(m.Lines), len(m.Links), len(n.Links))
	}
	if ml := m.Lines[id]; ml.Sections != 7 || m.TramSections(ml) != 6 {
		t.Errorf("imported line %d sections, runs %d", ml.Sections, m.TramSections(ml))
	}
}

// Every tram track arriving at a signalled crossing has its own tram
// signal, on the phase the tram's movements through the crossing obey.
func TestTramSignalsAtSignalledCrossings(t *testing.T) {
	n := gen()
	for _, mode := range []TramMode{Shared, Dedicated} {
		n.SetTram(roadBetween(t, n, 170, 170, 650, 170), mode, false)
	}
	n.SetTram(roadBetween(t, n, 330, 20, 330, 500), Dedicated, true)
	if len(n.TramSignalHeads()) == 0 {
		t.Fatal("no tram signals")
	}
	want := 0
	for i := range n.Links {
		l := &n.Links[i]
		if l.Kind != LaneLink || l.Modes&Tram == 0 || !l.Track && !l.Centre || l.Contra {
			continue
		}
		for _, c := range l.Next {
			cl := &n.Links[c]
			if cl.Kind != ConnectorLink || n.Signal(cl.Junction) == nil {
				continue
			}
			h, ok := n.TramSignalFor(l.ID)
			if !ok {
				t.Fatalf("track %d into junction %d has no tram signal", l.ID, cl.Junction)
			}
			if h.Junction != cl.Junction || h.Phase != cl.Phase {
				t.Fatalf("tram signal for track %d: junction %d phase %d, movement junction %d phase %d", l.ID, h.Junction, h.Phase, cl.Junction, cl.Phase)
			}
			want++
			break
		}
	}
	if want == 0 {
		t.Fatal("no track reaches a signalled crossing")
	}
	if got := len(n.TramSignalHeads()); got < want {
		t.Errorf("%d tram signals for %d tracks", got, want)
	}
}

func TestTramModelsAndOperatingSpeed(t *testing.T) {
	if len(TramModels) < 31 {
		t.Fatalf("only %d tram models, want at least 31", len(TramModels))
	}
	keys := map[string]bool{}
	regions := map[string]int{}
	speeds := map[int]bool{}
	accels := map[float64]bool{}
	vintageCount := 0
	modernChina := 0
	for _, m := range TramModels {
		if m.Key == "" || keys[m.Key] {
			t.Fatalf("duplicate or empty model key %q", m.Key)
		}
		keys[m.Key] = true
		regions[m.regionEn]++
		speeds[m.MaxSpeedKmh] = true
		accels[m.Accel] = true
		if m.Vintage {
			vintageCount++
			if m.MaxSpeedKmh > 75 {
				t.Errorf("vintage model %s has implausibly high top speed %d km/h", m.Key, m.MaxSpeedKmh)
			}
		} else if m.regionEn == "China" {
			modernChina++
		}
		if m.MaxSpeedKmh < 40 || m.Accel <= 0.5 || m.Decel <= 0.5 || m.Width < 2.1 || m.Width > 2.8 {
			t.Errorf("model %s has implausible specs: %+v", m.Key, m)
		}
	}
	for _, r := range []string{"Europe", "N. America", "Japan", "China"} {
		if regions[r] < 5 {
			t.Errorf("region %s has only %d models, want at least 5", r, regions[r])
		}
	}
	if modernChina < 5 {
		t.Errorf("only %d modern Chinese tram models, want at least 5", modernChina)
	}
	if vintageCount < 10 {
		t.Errorf("only %d pre-1950s vintage tram models, want at least 10", vintageCount)
	}
	if len(speeds) < 6 || len(accels) < 6 {
		t.Errorf("models lack variety: %d speeds, %d accelerations", len(speeds), len(accels))
	}

	n := gen()
	roads := roadBetween(t, n, 170, 170, 650, 170)
	n.SetTram(roads, Dedicated, false)
	a, _ := n.AddStop(roads[0], 0.5, DefaultPlatform)
	b, _ := n.AddStop(roads[len(roads)-1], 0.5, DefaultPlatform)
	n.AddEndCrossovers([]StopID{a, b})
	id, err := n.AddLine([]StopID{a, b})
	if err != nil {
		t.Fatal(err)
	}
	l := n.Lines[id]
	l.Model = "crrc-suzhou" // 70 km/h Chinese modern LRV
	if l.OperatingSpeedKmh() != 70 {
		t.Fatalf("default operating speed %d, want 70", l.OperatingSpeedKmh())
	}
	l.MaxSpeed = 60
	if l.OperatingSpeedKmh() != 60 {
		t.Fatalf("configured operating speed %d, want 60", l.OperatingSpeedKmh())
	}
	l.Model = "birney-1916" // 40 km/h vintage streetcar: clamps operating speed down to 40
	if l.OperatingSpeedKmh() != 40 {
		t.Fatalf("clamped operating speed %d, want 40", l.OperatingSpeedKmh())
	}
	saved := n.Export()
	m := gen()
	if err := m.Import(saved); err != nil {
		t.Fatal(err)
	}
	if ml := m.Lines[id]; ml.Model != "birney-1916" || ml.MaxSpeed != 60 || ml.OperatingSpeedKmh() != 40 {
		t.Fatalf("imported line model=%q maxSpeed=%d op=%d", ml.Model, ml.MaxSpeed, ml.OperatingSpeedKmh())
	}
}

func TestOffRoadAndMultiLevelTramways(t *testing.T) {
	n := gen()

	// 1. Road conflict guards: portal ramps cannot cut across ground streets,
	//    cannot lie inside a street carriageway, and cannot jump 2 levels.
	if err := n.AddTramway(Vec{X: 250, Y: 250}, Vec{X: 410, Y: 250}, 0, 1, false, 12); err == nil {
		t.Fatal("expected ramp crossing X=330 street to be rejected")
	}
	if err := n.AddTramway(Vec{X: 170, Y: 170}, Vec{X: 250, Y: 170}, 0, -1, false, 12); err == nil {
		t.Fatal("expected portal ramp along street carriageway Y=170 to be rejected")
	}
	if err := n.AddTramway(Vec{X: 220, Y: 220}, Vec{X: 280, Y: 280}, -1, 1, false, 12); err == nil {
		t.Fatal("expected -1 -> +1 two-level jump to be rejected")
	}

	// 2. Build a multi-level + off-road corridor connecting street termini:
	//    Street (0) -> Ramp (0->+1) -> Elevated (+1) -> Ramp (+1->0) ->
	//    Off-road crossing car street (0) -> Portal (0->-1) -> Tunnel (-1) ->
	//    Portal (-1->0) -> Street (0).
	startRoads := roadBetween(t, n, 170, 170, 330, 170)
	n.SetTram(startRoads, Dedicated, false)
	endRoads := roadBetween(t, n, 1290, 330, 1460, 330)
	n.SetTram(endRoads, Dedicated, false)

	groundJBefore := 0
	for _, j := range n.Junctions {
		if j.Level == 0 {
			groundJBefore++
		}
	}

	mustBuild := func(a, b Vec, la, lb int) {
		t.Helper()
		if err := n.AddTramway(a, b, la, lb, false, 12); err != nil {
			t.Fatalf("AddTramway(%v lv%d -> %v lv%d): %v", a, la, b, lb, err)
		}
	}
	mustBuild(Vec{X: 330, Y: 170}, Vec{X: 390, Y: 230}, 0, 1)    // ramp up from street junction
	mustBuild(Vec{X: 390, Y: 230}, Vec{X: 730, Y: 230}, 1, 1)    // viaduct over X=500 and X=650 streets
	mustBuild(Vec{X: 730, Y: 230}, Vec{X: 785, Y: 250}, 1, 0)    // ramp down to open ground

	// Grade separation check: flying over X=500 and X=650 at +1 must only add
	// the single open-ground landing junction at (785, 250) on level 0.
	groundJAfterViaduct := 0
	for _, j := range n.Junctions {
		if j.Level == 0 {
			groundJAfterViaduct++
		}
	}
	if groundJAfterViaduct != groundJBefore+1 {
		t.Fatalf("elevated viaduct split ground streets: ground junctions %d -> %d", groundJBefore, groundJAfterViaduct)
	}

	mustBuild(Vec{X: 785, Y: 250}, Vec{X: 920, Y: 250}, 0, 0)    // off-road ground track crossing X=810 street!
	mustBuild(Vec{X: 920, Y: 250}, Vec{X: 965, Y: 285}, 0, -1)   // tunnel portal down
	mustBuild(Vec{X: 965, Y: 285}, Vec{X: 1230, Y: 285}, -1, -1) // underground tunnel under X=980 & X=1130
	mustBuild(Vec{X: 1230, Y: 285}, Vec{X: 1290, Y: 330}, -1, 0) // tunnel portal up into street junction

	// Verify the off-road level crossing at (810, 250) created a signalised
	// level-0 junction with both car signals and tram signals, and that the
	// portal junction at (330, 170) has protected signal phases.
	crossJ, ok := n.NearestJunctionAt(Vec{X: 810, Y: 250}, 0, 4)
	if !ok {
		t.Fatal("off-road crossing over X=810 street did not create a level-0 junction")
	}
	if n.Signal(crossJ) == nil {
		t.Fatal("off-road street level crossing has no traffic signal")
	}
	portalJ, _ := n.NearestJunctionAt(Vec{X: 330, Y: 170}, 0, 4)
	if sig := n.Signal(portalJ); sig == nil || sig.Phases < 3 {
		t.Fatalf("portal street junction %d expected 3-phase signal protection, got %+v", portalJ, sig)
	}

	// Verify stops and crossovers cannot be placed on sloped ramps, but work on
	// flat +1 viaducts, 0 off-road corridors, and -1 tunnels.
	rampRd, _, ok := n.NearestRoadAt(Vec{X: 360, Y: 200}, 1, 10)
	if !ok || !n.IsRamp(rampRd) {
		t.Fatal("did not find 0->+1 ramp")
	}
	if _, err := n.AddStop(rampRd, 0.5, DefaultPlatform); err == nil {
		t.Fatal("expected AddStop on sloped ramp to fail")
	}
	if err := n.AddCrossover(rampRd, 0.5); err == nil {
		t.Fatal("expected AddCrossover on sloped ramp to fail")
	}

	s0, err := n.AddStop(startRoads[0], 0.5, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	elevRd, elevT, ok := n.NearestRoadAt(Vec{X: 560, Y: 230}, 1, 6)
	if !ok {
		t.Fatal("no elevated road at (560, 230)")
	}
	s1, err := n.AddStop(elevRd, elevT, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	offRd, offT, ok := n.NearestRoadAt(Vec{X: 865, Y: 250}, 0, 6)
	if !ok || !n.Roads[offRd].Tramway {
		t.Fatal("no off-road tramway at (865, 250)")
	}
	s2, err := n.AddStop(offRd, offT, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	tunRd, tunT, ok := n.NearestRoadAt(Vec{X: 1100, Y: 285}, -1, 6)
	if !ok {
		t.Fatal("no underground tunnel at (1100, 285)")
	}
	s3, err := n.AddStop(tunRd, tunT, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}
	s4, err := n.AddStop(endRoads[len(endRoads)-1], 0.5, DefaultPlatform)
	if err != nil {
		t.Fatal(err)
	}

	if n.StopLevel(n.Stops[s1]) != 1 || n.StopLevel(n.Stops[s2]) != 0 || n.StopLevel(n.Stops[s3]) != -1 {
		t.Fatalf("stop levels = (%d, %d, %d), want (1, 0, -1)",
			n.StopLevel(n.Stops[s1]), n.StopLevel(n.Stops[s2]), n.StopLevel(n.Stops[s3]))
	}

	n.AddEndCrossovers([]StopID{s0, s4})
	lineID, err := n.AddLine([]StopID{s0, s1, s2, s3, s4})
	if err != nil {
		t.Fatalf("AddLine across ground + elevated + off-road + underground failed: %v", err)
	}

	// 3. Export / Import round-trip preserves edits, levels, stops, signals and line.
	saved := n.Export()
	m := gen()
	if err := m.Import(saved); err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if len(m.Roads) != len(n.Roads) || len(m.Links) != len(n.Links) || len(m.Stops) != 5 {
		t.Fatalf("round-trip mismatch: roads %d/%d, links %d/%d, stops %d/5",
			len(m.Roads), len(n.Roads), len(m.Links), len(n.Links), len(m.Stops))
	}
	if m.Lines[lineID] == nil || m.StopLevel(m.Stops[s1]) != 1 || m.StopLevel(m.Stops[s3]) != -1 {
		t.Fatal("imported line or stop levels lost")
	}
}
