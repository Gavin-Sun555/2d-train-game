package game

import (
	"math"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// From one frame to the next a car moves and turns only a little, even
// through sharp turns at junctions: no jumps.
func TestCarsTurnSmoothly(t *testing.T) {
	g := New("seattle")
	g.densityIdx = 2
	g.rebuild()
	type pose struct{ c, d [2]float64 }
	last := map[int]pose{}
	worstMove, worstTurn := 0.0, 0.0
	turns, sharp, frames := 0, 0, 0
	for i := 0; i < 90*60; i++ {
		g.sim.Step()
		if i < 30*60 {
			continue // let traffic fill in
		}
		for _, v := range g.sim.Vehicles {
			if v.Kind != traffic.CarKind {
				continue
			}
			c, d := carPose(g.sim.Body(v), v.Length)
			p := pose{[2]float64{c.X, c.Y}, [2]float64{d.X, d.Y}}
			if q, ok := last[v.ID]; ok {
				move := math.Hypot(p.c[0]-q.c[0], p.c[1]-q.c[1])
				turn := math.Acos(math.Max(-1, math.Min(1, p.d[0]*q.d[0]+p.d[1]*q.d[1]))) * 180 / math.Pi
				worstMove = math.Max(worstMove, move)
				worstTurn = math.Max(worstTurn, turn)
				if turn > 0.5 {
					turns++
				}
				if turn > 5 {
					sharp++
				}
				frames++
			}
			last[v.ID] = p
		}
	}
	if turns == 0 {
		t.Fatal("no car turned")
	}
	// At 50 km/h a car covers 0.23 m a frame; turning, it swings a few
	// degrees a frame. (Placed by its nose, cars snapped by up to ~60° as
	// the nose passed a corner, thousands of times a minute.)
	if worstMove > 1.0 || worstTurn > 20 {
		t.Errorf("a car jumped: %.2f m and %.1f° in one frame", worstMove, worstTurn)
	}
	if float64(sharp) > 1e-4*float64(frames) {
		t.Errorf("%d of %d car frames turned more than 5°", sharp, frames)
	}
	t.Logf("worst per frame: %.2f m, %.1f°; %d of %d frames over 5°", worstMove, worstTurn, sharp, frames)
}
