package sim

import (
	"math"
	"reflect"
	"testing"
)

const ticksPerMinute = 60 * 60

// run steps the world, checking safety invariants on every tick.
func run(t *testing.T, w *World, ticks int, each func()) {
	t.Helper()
	for i := 0; i < ticks; i++ {
		w.Step()
		if err := w.CheckInvariants(); err != nil {
			t.Fatalf("tick %d (%.1fs): %v", w.Tick, w.Seconds(), err)
		}
		if each != nil {
			each()
		}
	}
}

func arrivals(w *World, train int) []string {
	var out []string
	for _, e := range w.Events {
		if e.Train == train && e.Kind == Arrive {
			out = append(out, e.Station)
		}
	}
	return out
}

func TestShuttleRoundTrips(t *testing.T) {
	w := Shuttle()
	tr := w.Trains[0]
	peak := 0.0
	run(t, w, 30*ticksPerMinute, func() { peak = math.Max(peak, tr.Speed) })

	got := arrivals(w, 0)
	if len(got) < 8 {
		t.Fatalf("only %d arrivals in 30 min: %v", len(got), got)
	}
	for i, s := range got {
		want := "East"
		if i%2 == 1 {
			want = "West"
		}
		if s != want {
			t.Fatalf("arrival %d at %s, want %s (all: %v)", i, s, want, got)
		}
	}
	if peak < tr.MaxSpeed-0.01 {
		t.Errorf("never reached line speed: peak %.2f m/s", peak)
	}
}

func TestStopsExactlyAtPlatformEnd(t *testing.T) {
	w := Shuttle()
	tr := w.Trains[0]
	seen := 0
	run(t, w, 20*ticksPerMinute, func() {
		n := len(w.Events)
		if n == 0 || w.Events[n-1].Tick != w.Tick-1 || w.Events[n-1].Kind != Arrive {
			return
		}
		body := w.Body(tr)
		head := body[len(body)-1]
		var want Vec2
		for _, node := range w.Net.Nodes {
			if node.Name == w.Events[n-1].Station {
				want = node.Pos
			}
		}
		if d := math.Hypot(head.X-want.X, head.Y-want.Y); d > 0.01 {
			t.Fatalf("stopped %.3fm from %s platform end", d, w.Events[n-1].Station)
		}
		seen++
	})
	if seen < 4 {
		t.Fatalf("checked only %d stops", seen)
	}
}

func TestLoopBlockSignalsKeepTrainsApart(t *testing.T) {
	w := Loop()
	heldAtRed := make([]bool, len(w.Trains))
	run(t, w, 60*ticksPerMinute, func() {
		for i, tr := range w.Trains {
			if tr.State == Running && tr.Speed == 0 {
				heldAtRed[i] = true
			}
		}
	})
	for i, tr := range w.Trains {
		if n := len(arrivals(w, i)); n < 10 {
			t.Errorf("%s made only %d stops in 60 min (deadlock?)", tr.Name, n)
		}
	}
	if !heldAtRed[1] {
		t.Errorf("T2 never waited at a red signal although it starts behind T3")
	}
}

func TestDeterministicReplay(t *testing.T) {
	for _, sc := range Scenarios {
		a, b := sc.New(), sc.New()
		for i := 0; i < 20*ticksPerMinute; i++ {
			a.Step()
			b.Step()
		}
		if !reflect.DeepEqual(a.Events, b.Events) {
			t.Errorf("%s: event logs diverge", sc.Name)
		}
	}
}
