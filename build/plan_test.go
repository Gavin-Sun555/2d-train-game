package build

import (
	"errors"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

// Street centre lines in the seed-1 city (tile 8 and 16 are local streets).
const (
	st8  = 8.5 * city.TileSize
	st16 = 16.5 * city.TileSize
	st24 = 24.5 * city.TileSize // inside the arterial at tiles 24–25
)

func v(x, y float64) sim.Vec2 { return sim.Vec2{X: x, Y: y} }

func newPlan() *Plan { return NewPlan(city.Generate(1)) }

func findTile(t *testing.T, m *city.Map, k city.Kind) (int, int) {
	t.Helper()
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.At(x, y).Kind == k {
				return x, y
			}
		}
	}
	t.Fatalf("no %s tile", k)
	return 0, 0
}

func mustSeg(t *testing.T, p *Plan, a, b sim.Vec2) {
	t.Helper()
	if _, err := p.AddSegment(a, b, sim.Ground); err != nil {
		t.Fatalf("segment %v→%v: %v", a, b, err)
	}
}

func TestGroundTrackFollowsStreetsNotBuildingsOrWater(t *testing.T) {
	p := newPlan()
	if err := p.CheckSegment(v(st8, st8), v(st16+160, st8), sim.Ground); err != nil {
		t.Fatalf("street segment rejected: %v", err)
	}
	for _, k := range []city.Kind{city.Building, city.Water} {
		x, y := findTile(t, p.City, k)
		a := v(float64(x)*city.TileSize+4, float64(y)*city.TileSize+10)
		err := p.CheckSegment(a, v(a.X+12, a.Y), sim.Ground)
		var te TerrainError
		if !errors.As(err, &te) || te.Kind != k {
			t.Errorf("%s: got %v, want terrain error", k, err)
		}
	}
	if err := p.CheckSegment(v(st8, st8), v(st16, st8), sim.Underground); !errors.Is(err, ErrNotBuildable) {
		t.Errorf("underground should be reserved, got %v", err)
	}
	if err := p.CheckSegment(v(st8, st8), v(st8+5, st8), sim.Ground); !errors.Is(err, ErrTooShort) {
		t.Errorf("short segment: got %v", err)
	}
}

func TestCrossingTrackCreatesJunction(t *testing.T) {
	p := newPlan()
	mustSeg(t, p, v(st8, st8), v(st24, st8))
	mustSeg(t, p, v(st16, st8-80), v(st16, st8+80))
	if n := len(p.Segments()); n != 4 {
		t.Fatalf("want 4 segments after crossing, got %d", n)
	}
	j, ok := p.NearestNode(v(st16, st8), sim.Ground, 0.1)
	if !ok || p.degree(j.ID) != 4 {
		t.Fatalf("expected a 4-way junction at the crossing")
	}
	if _, err := p.AddSegment(v(st8+40, st8), v(st16+40, st8), sim.Ground); !errors.Is(err, ErrOverlap) {
		t.Errorf("overlapping track: got %v", err)
	}
	// Ending on the middle of existing track joins it.
	mustSeg(t, p, v(st24, st8), v(st24, st8+100))
	if n := len(p.Nodes()); n != 6 {
		t.Errorf("want 6 nodes, got %d", n)
	}
}

func TestTramRunsLineAcrossStreetCorner(t *testing.T) {
	p := newPlan()
	mustSeg(t, p, v(st8, st8), v(st16, st8))
	mustSeg(t, p, v(st16, st8), v(st16, st16+100))
	a, err := p.AddStop(v(st8+40, st8+3), sim.Ground, 10)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.AddStop(v(st16, st16), sim.Ground, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.AddLine(Tram, []StopID{a, b}); err != nil {
		t.Fatal(err)
	}
	w, status := p.Compile().World()
	if len(status) != 1 || status[0].Train < 0 {
		t.Fatalf("tram not placed: %+v", status)
	}
	var got []string
	for i := 0; i < 15*60*60; i++ {
		w.Step()
		if err := w.CheckInvariants(); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range w.Events {
		if e.Kind == sim.Arrive {
			got = append(got, e.Station)
		}
	}
	names := [2]string{p.Stop(b).Name, p.Stop(a).Name}
	if len(got) < 6 {
		t.Fatalf("only %d arrivals: %v", len(got), got)
	}
	for i, s := range got {
		if s != names[i%2] {
			t.Fatalf("arrival %d at %s, want %s: %v", i, s, names[i%2], got)
		}
	}
}

func TestLineNeedingReversalAtJunctionIsRejected(t *testing.T) {
	p := newPlan()
	// A T-junction: main street plus a spur. Going from one spur stop to a
	// main-street stop on the "wrong" side would need a reversal.
	mustSeg(t, p, v(st8, st8), v(st24, st8))
	mustSeg(t, p, v(st16, st8), v(st16, st8+120))
	main1, _ := p.AddStop(v(st8+40, st8), sim.Ground, 5)
	spur, _ := p.AddStop(v(st16, st8+100), sim.Ground, 5)
	main2, _ := p.AddStop(v(st24-40, st8), sim.Ground, 5)
	if _, err := p.AddLine(Tram, []StopID{main1, spur}); err != nil {
		t.Fatalf("simple turn rejected: %v", err)
	}
	if _, err := p.AddLine(Tram, []StopID{main1, spur, main2}); err == nil {
		t.Fatalf("line requiring a reversal at the spur was accepted")
	}
}

func TestDeletingTrackRemovesOrphanStopsAndLines(t *testing.T) {
	p := newPlan()
	mustSeg(t, p, v(st8, st8), v(st16, st8))
	a, _ := p.AddStop(v(st8+40, st8), sim.Ground, 5)
	b, _ := p.AddStop(v(st16-40, st8), sim.Ground, 5)
	if _, err := p.AddLine(Tram, []StopID{a, b}); err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Segments() {
		p.DeleteSegment(s.ID)
	}
	if len(p.Nodes())+len(p.Stops())+len(p.Lines()) != 0 {
		t.Fatalf("leftovers: %d nodes, %d stops, %d lines", len(p.Nodes()), len(p.Stops()), len(p.Lines()))
	}
}

func TestCrossingLinesNeverShareTheJunction(t *testing.T) {
	p := newPlan()
	mustSeg(t, p, v(st8, st8), v(st24, st8))
	mustSeg(t, p, v(st16, st8-140), v(st16, st8+140))
	a, _ := p.AddStop(v(st8+20, st8), sim.Ground, 5)
	b, _ := p.AddStop(v(st24-20, st8), sim.Ground, 5)
	c, _ := p.AddStop(v(st16, st8-120), sim.Ground, 5)
	d, _ := p.AddStop(v(st16, st8+120), sim.Ground, 5)
	for _, pair := range [][]StopID{{a, b}, {c, d}} {
		if _, err := p.AddLine(Tram, pair); err != nil {
			t.Fatal(err)
		}
	}
	w, status := p.Compile().World()
	for _, s := range status {
		if s.Train < 0 {
			t.Fatalf("line %d has no tram: %v", s.Line, s.Err)
		}
	}
	waited := false
	for i := 0; i < 30*60*60; i++ {
		w.Step()
		if err := w.CheckInvariants(); err != nil {
			t.Fatalf("%.0fs: %v", w.Seconds(), err)
		}
		for _, tr := range w.Trains {
			waited = waited || (tr.State == sim.Running && tr.Speed == 0)
		}
	}
	per := map[int]int{}
	for _, e := range w.Events {
		if e.Kind == sim.Arrive {
			per[e.Train]++
		}
	}
	if per[0] < 10 || per[1] < 10 {
		t.Fatalf("too few arrivals (deadlock?): %v", per)
	}
	if !waited {
		t.Log("note: trams never had to wait at the junction in this run")
	}
}
