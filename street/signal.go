package street

import "math"

// Signal is a fixed-time traffic signal at a crossing: two phases on normal
// street intersections (phase 0 along the main street, phase 1 across it), or
// up to three phases where a tramway or portal ramp needs its own protected
// movement clear of street traffic.
type Signal struct {
	Offset float64 // seconds, staggers junctions
	Phases int     // number of phases (0 or 2 = two-phase; 3 = with protected tram phase)
}

const (
	GreenTime  = 24.0
	AmberTime  = 3.0
	AllRedTime = 2.0
	CycleTime  = 2 * (GreenTime + AmberTime + AllRedTime)
)

type Aspect uint8

const (
	Red Aspect = iota
	Amber
	Green
)

func newSignal(j JunctionID) *Signal {
	return &Signal{Offset: float64((int(j) * 11) % int(CycleTime)), Phases: 2}
}

// Signal returns the junction's signal, or nil if it is unsignalised.
func (n *Network) Signal(j JunctionID) *Signal { return n.signals[j] }

// AspectAt is the aspect shown to phase group p at time t (seconds).
func (s *Signal) AspectAt(p int, t float64) Aspect {
	n := max(s.Phases, 2)
	if p < 0 {
		return Green
	}
	slot := GreenTime + AmberTime + AllRedTime
	period := float64(n) * slot
	c := t + s.Offset
	c -= period * math.Floor(c/period)
	c -= float64(p%n) * slot
	if c < 0 {
		c += period
	}
	switch {
	case c < GreenTime:
		return Green
	case c < GreenTime+AmberTime:
		return Amber
	}
	return Red
}
