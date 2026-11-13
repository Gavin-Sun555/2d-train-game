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
	// Each of the four track ends gets a terminus crossover and turnback stub.
	crossovers, stubs := 0, 0
	for _, l := range n.Links {
		if l.Kind == CrossoverLink {
			crossovers++
		}
		if l.Kind == LaneLink && l.Twin >= 0 {
			stubs++
		}
	}
	if crossovers != 4 || stubs != 8 || len(n.Blocks) != 4 {
		t.Errorf("want 4 crossovers, 4 twinned stubs, 4 blocks; got %d, %d/2, %d", crossovers, stubs, len(n.Blocks))
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
	if _, err := n.AddStop(roads[0], 0.5); err == nil {
		t.Fatal("stop on road without track accepted")
	}
	n.SetTram(roads, Shared, false)
	if _, err := n.AddStop(roads[0], 0.02); err == nil {
		t.Fatal("stop next to junction accepted")
	}
	a, err := n.AddStop(roads[0], 0.5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.AddStop(roads[len(roads)-1], 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.AddLine([]StopID{a, b}); err != nil {
		t.Fatalf("line rejected: %v", err)
	}
	n.SetTram(roads[:1], NoTram, false)
	if len(n.Stops) != 1 || len(n.Lines) != 0 {
		t.Fatalf("removing track should drop its stop and the line: %d stops, %d lines", len(n.Stops), len(n.Lines))
	}
}
