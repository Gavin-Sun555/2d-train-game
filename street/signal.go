package street

// Signal is a fixed-time, two-phase traffic signal: phase 0 serves the
// east–west approaches, phase 1 the north–south ones.
type Signal struct {
	Offset float64 // seconds, staggers junctions
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
	return &Signal{Offset: float64((int(j) * 11) % int(CycleTime))}
}

// Signal returns the junction's signal, or nil if it is unsignalised.
func (n *Network) Signal(j JunctionID) *Signal { return n.signals[j] }

// AspectAt is the aspect shown to phase group p at time t (seconds).
func (s *Signal) AspectAt(p int, t float64) Aspect {
	half := GreenTime + AmberTime + AllRedTime
	c := t + s.Offset
	c -= CycleTime * float64(int(c/CycleTime))
	if p == 1 {
		c -= half
		if c < 0 {
			c += CycleTime
		}
	}
	switch {
	case c < GreenTime:
		return Green
	case c < GreenTime+AmberTime:
		return Amber
	}
	return Red
}
