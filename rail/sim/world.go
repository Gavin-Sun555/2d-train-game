package sim

import (
	"fmt"
	"math"
)

// DT is the fixed simulation tick. The renderer may run several ticks per
// frame for fast-forward, but never changes DT.
const DT = 1.0 / 60

const (
	creepSpeed       = 0.3  // m/s, final approach so stops finish in finite time
	lookaheadMargin  = 50.0 // m of reservation beyond the braking distance
	positionEpsilon  = 1e-6
	speedEpsilonTest = 1e-9
)

const noOwner = -1

type EventKind int

const (
	Arrive EventKind = iota
	Depart
)

func (k EventKind) String() string {
	if k == Arrive {
		return "arrive"
	}
	return "depart"
}

type Event struct {
	Tick    int64
	Train   int
	Kind    EventKind
	Station string
}

type World struct {
	Net    *Network
	Trains []*Train
	Tick   int64
	Events []Event

	owner []int // per edge: owning train ID or noOwner
}

func NewWorld(net *Network) *World {
	owner := make([]int, len(net.Edges))
	for i := range owner {
		owner[i] = noOwner
	}
	return &World{Net: net, owner: owner}
}

// Seconds is the elapsed simulated time.
func (w *World) Seconds() float64 { return float64(w.Tick) * DT }

// AddTrain places a train with its head at the exit end of
// route.Legs[leg].Steps[step]. If that step ends the leg, the train starts
// dwelling at the leg's station.
func (w *World) AddTrain(spec TrainSpec, route Route, leg, step int) (*Train, error) {
	if leg < 0 || leg >= len(route.Legs) || step < 0 || step >= len(route.Legs[leg].Steps) {
		return nil, fmt.Errorf("train %s: start position out of range", spec.Name)
	}
	s := route.Legs[leg].Steps[step]
	if l := w.Net.StepLength(s); spec.Length > l {
		return nil, fmt.Errorf("train %s (%.0fm) longer than start block (%.0fm)", spec.Name, spec.Length, l)
	}
	if w.owner[s.Edge] != noOwner {
		return nil, fmt.Errorf("train %s: start block %d already owned", spec.Name, s.Edge)
	}
	t := &Train{ID: len(w.Trains), TrainSpec: spec, Route: route}
	last := step == len(route.Legs[leg].Steps)-1
	t.trail = []held{{Step: s, Stop: last, Leg: leg}}
	t.head = w.Net.StepLength(s)
	t.nextLeg, t.nextStep = leg, step
	t.advanceCursor()
	if last {
		t.State = Dwelling
		t.Station = route.Legs[leg].Station
		t.DwellLeft = route.Legs[leg].Dwell
	}
	w.owner[s.Edge] = t.ID
	w.Trains = append(w.Trains, t)
	return t, nil
}

func (t *Train) advanceCursor() {
	t.nextStep++
	if t.nextStep >= len(t.Route.Legs[t.nextLeg].Steps) {
		t.nextStep = 0
		t.nextLeg = (t.nextLeg + 1) % len(t.Route.Legs)
	}
}

// Step advances the world by one tick. Trains update in ID order so runs are
// fully deterministic.
func (w *World) Step() {
	for _, t := range w.Trains {
		w.stepTrain(t)
	}
	w.Tick++
}

func (w *World) stepTrain(t *Train) {
	if t.State == Dwelling {
		t.DwellLeft -= DT
		if t.DwellLeft > 0 {
			return
		}
		w.depart(t)
	}

	w.reserveAhead(t)

	auth := t.authority(w.Net)
	remaining := auth - t.head
	target := math.Min(t.MaxSpeed, math.Sqrt(2*t.Decel*math.Max(remaining, 0)))
	if remaining > positionEpsilon {
		target = math.Max(target, math.Min(creepSpeed, remaining/DT))
	}
	if target < t.Speed {
		t.Speed = target
	} else {
		t.Speed = math.Min(target, t.Speed+t.Accel*DT)
	}

	t.head += t.Speed * DT
	if auth-t.head < positionEpsilon {
		t.head = auth
		t.Speed = 0
		if t.trail[len(t.trail)-1].Stop {
			leg := t.Route.Legs[t.trail[len(t.trail)-1].Leg]
			t.State = Dwelling
			t.Station = leg.Station
			t.DwellLeft = leg.Dwell
			w.Events = append(w.Events, Event{Tick: w.Tick, Train: t.ID, Kind: Arrive, Station: leg.Station})
		}
	}

	w.releaseBehind(t)
}

// reserveAhead grabs blocks in front of the train until it holds at least a
// full braking distance plus margin, stopping at a red block or at the next
// station stop (the exit signal stays red until departure).
func (w *World) reserveAhead(t *Train) {
	need := t.MaxSpeed*t.MaxSpeed/(2*t.Decel) + lookaheadMargin
	for {
		if t.trail[len(t.trail)-1].Stop {
			return
		}
		if t.authority(w.Net)-t.head >= need {
			return
		}
		leg := t.Route.Legs[t.nextLeg]
		s := leg.Steps[t.nextStep]
		if o := w.owner[s.Edge]; o != noOwner && o != t.ID {
			return
		}
		w.owner[s.Edge] = t.ID
		t.trail = append(t.trail, held{Step: s, Stop: t.nextStep == len(leg.Steps)-1, Leg: t.nextLeg})
		t.advanceCursor()
	}
}

// releaseBehind frees blocks the tail has fully cleared.
func (w *World) releaseBehind(t *Train) {
	for len(t.trail) > 1 {
		l := w.Net.StepLength(t.trail[0].Step)
		if t.tail() < l-positionEpsilon {
			return
		}
		w.owner[t.trail[0].Step.Edge] = noOwner
		t.head -= l
		t.trail = t.trail[1:]
	}
}

func (w *World) depart(t *Train) {
	t.State = Running
	t.DwellLeft = 0
	w.Events = append(w.Events, Event{Tick: w.Tick, Train: t.ID, Kind: Depart, Station: t.Station})
	t.trail[len(t.trail)-1].Stop = false

	leg := t.Route.Legs[t.nextLeg]
	headStep := t.trail[len(t.trail)-1].Step
	if leg.Steps[t.nextStep] != headStep.Reverse() {
		return // through station, carry on in the same direction
	}

	// Terminus reversal: the old tail becomes the new head.
	total := t.authority(w.Net)
	oldTail := t.tail()
	rev := make([]held, len(t.trail))
	for i, h := range t.trail {
		rev[len(t.trail)-1-i] = held{Step: h.Step.Reverse(), Leg: t.nextLeg}
	}
	t.trail = rev
	t.head = total - oldTail

	// The first steps of the new leg are already under the train.
	for _, h := range t.trail {
		if t.Route.Legs[t.nextLeg].Steps[t.nextStep] != h.Step {
			break
		}
		last := t.nextStep == len(t.Route.Legs[t.nextLeg].Steps)-1
		if last {
			break
		}
		t.advanceCursor()
	}
}

// CheckInvariants verifies the safety rules every tick-level test relies on.
func (w *World) CheckInvariants() error {
	physical := make(map[EdgeID]int)
	for _, t := range w.Trains {
		if t.Speed < -speedEpsilonTest || t.Speed > t.MaxSpeed+speedEpsilonTest {
			return fmt.Errorf("train %s speed %.3f outside [0, %.3f]", t.Name, t.Speed, t.MaxSpeed)
		}
		if t.tail() < -positionEpsilon {
			return fmt.Errorf("train %s tail before its first held block", t.Name)
		}
		if t.head > t.authority(w.Net)+positionEpsilon {
			return fmt.Errorf("train %s head beyond movement authority", t.Name)
		}
		for _, h := range t.trail {
			if w.owner[h.Step.Edge] != t.ID {
				return fmt.Errorf("train %s holds block %d owned by %d", t.Name, h.Step.Edge, w.owner[h.Step.Edge])
			}
		}
		for _, e := range w.OccupiedEdges(t) {
			if other, ok := physical[e]; ok {
				return fmt.Errorf("trains %s and %s both on block %d", w.Trains[other].Name, t.Name, e)
			}
			physical[e] = t.ID
		}
	}
	return nil
}

// --- Read-only views for renderers and tests --------------------------------

type BlockState int

const (
	BlockFree BlockState = iota
	BlockReserved
	BlockOccupied
)

// OccupiedEdges lists the blocks the train body physically covers.
func (w *World) OccupiedEdges(t *Train) []EdgeID {
	var out []EdgeID
	tail, off := t.tail(), 0.0
	for _, h := range t.trail {
		l := w.Net.StepLength(h.Step)
		if off < t.head && off+l > tail {
			out = append(out, h.Step.Edge)
		}
		off += l
	}
	return out
}

func (w *World) BlockState(e EdgeID) BlockState {
	o := w.owner[e]
	if o == noOwner {
		return BlockFree
	}
	for _, oe := range w.OccupiedEdges(w.Trains[o]) {
		if oe == e {
			return BlockOccupied
		}
	}
	return BlockReserved
}

// SignalGreen reports the aspect of the signal guarding entry to s: green
// only when the block is reserved for a train that has not yet entered it.
func (w *World) SignalGreen(s Step) bool {
	o := w.owner[s.Edge]
	if o == noOwner {
		return false
	}
	t, off := w.Trains[o], 0.0
	for _, h := range t.trail {
		if h.Step == s {
			return off >= t.head-positionEpsilon && !(t.State == Dwelling)
		}
		off += w.Net.StepLength(h.Step)
	}
	return false
}

// Body returns the train outline as points from tail to head.
func (w *World) Body(t *Train) []Vec2 {
	var pts []Vec2
	tail, off := t.tail(), 0.0
	for _, h := range t.trail {
		l := w.Net.StepLength(h.Step)
		a, b := math.Max(tail, off), math.Min(t.head, off+l)
		if a < b {
			if len(pts) == 0 {
				pts = append(pts, w.Net.PointOnStep(h.Step, a-off))
			}
			pts = append(pts, w.Net.PointOnStep(h.Step, b-off))
		}
		off += l
	}
	return pts
}
