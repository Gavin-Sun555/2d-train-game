package sim

// Leg is the run from one stop to the next; the train stops with its head at
// the exit end of the final step and dwells there.
type Leg struct {
	Steps   []Step
	Station string
	Dwell   float64 // seconds
}

// Route is a cyclic list of legs. A shuttle is A→B then B→A; the train
// reverses automatically when a leg starts on the edge it is standing on.
type Route struct {
	Legs []Leg
}

type TrainState int

const (
	Running TrainState = iota
	Dwelling
)

func (s TrainState) String() string {
	if s == Dwelling {
		return "dwell"
	}
	return "run"
}

// TrainSpec holds the static performance figures of a train.
type TrainSpec struct {
	Name     string
	Length   float64 // m
	MaxSpeed float64 // m/s
	Accel    float64 // m/s²
	Decel    float64 // m/s² service brake
}

// held is one block the train occupies or has reserved ahead of itself.
type held struct {
	Step Step
	Stop bool // the train must stop at the exit end of this step
	Leg  int
}

type Train struct {
	ID int
	TrainSpec
	Route Route

	trail []held  // tail-most first; covers the body plus reserved blocks ahead
	head  float64 // distance from the entry of trail[0] to the train's head

	Speed     float64
	State     TrainState
	DwellLeft float64
	Station   string // station while dwelling, last station otherwise

	// Route cursor: next step to reserve.
	nextLeg, nextStep int
}

// authority is the distance from trail[0] entry to the end of the last
// reserved block: the train may never move beyond it.
func (t *Train) authority(n *Network) float64 {
	total := 0.0
	for _, h := range t.trail {
		total += n.StepLength(h.Step)
	}
	return total
}

func (t *Train) tail() float64 { return t.head - t.Length }

// NextStation is the station the train is heading for (or standing at).
func (t *Train) NextStation() string {
	if t.State == Dwelling {
		return t.Station
	}
	for _, h := range t.trail {
		if h.Stop {
			return t.Route.Legs[h.Leg].Station
		}
	}
	if len(t.trail) > 0 {
		return t.Route.Legs[t.trail[len(t.trail)-1].Leg].Station
	}
	return ""
}
