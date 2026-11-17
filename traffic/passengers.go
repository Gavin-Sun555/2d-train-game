package traffic

import (
	"math/rand/v2"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// Passengers, as in the bus game: every stop a line serves gathers
// passengers a few at a time, each heading for another stop of the lines
// calling there. A tram lets off the passengers for its stop (everyone at a
// terminus) and takes on, in arrival order, those whose stop it reaches
// before its terminus, up to its capacity. Doors work in parallel, so
// longer trams load faster; the stop lasts at least TramDwell.

// PassengerRates are the arrival speeds the player picks from:
// few, moderate (default), many.
var PassengerRates = [...]float64{0.5, 1, 2}

const (
	DefaultPassengerRate = 1
	MaxWaiting           = 500 // passengers a stop holds
)

type stopQueue struct {
	dests    []street.StopID // reachable from here
	waiting  []street.StopID // destination of each waiting passenger, in arrival order
	timer    float64
	interval float64 // s between arrivals at the moderate rate
	arrivals int     // counts arrivals, so a stopped tram notices new ones
}

// PassengerStats counts the passengers served.
type PassengerStats struct {
	Boarded, Delivered int
}

func (s *Sim) initPassengers(seed uint64) {
	s.psrc = rand.NewPCG(seed, 0x706178)
	s.prng = rand.New(s.psrc)
	s.queues = map[street.StopID]*stopQueue{}
	reach := map[street.StopID]map[street.StopID]bool{}
	for _, id := range s.Net.LineIDs() {
		l := s.Net.Lines[id]
		for _, a := range l.Stops {
			for _, b := range l.Stops {
				if a != b {
					if reach[a] == nil {
						reach[a] = map[street.StopID]bool{}
					}
					reach[a][b] = true
				}
			}
		}
	}
	for _, a := range s.Net.StopIDs() {
		if len(reach[a]) == 0 {
			continue
		}
		q := &stopQueue{interval: 20 + 15*s.prng.Float64()}
		for _, b := range s.Net.StopIDs() {
			if reach[a][b] {
				q.dests = append(q.dests, b)
			}
		}
		s.queues[a] = q
	}
}

// updatePassengers lets new passengers arrive at the stops.
func (s *Sim) updatePassengers() {
	rate := s.PassengerRate
	if rate <= 0 {
		rate = PassengerRates[DefaultPassengerRate]
	}
	for _, id := range s.Net.StopIDs() {
		q := s.queues[id]
		if q == nil {
			continue
		}
		q.timer += DT * rate
		if q.timer < q.interval {
			continue
		}
		q.timer = 0
		for k := 1 + s.prng.IntN(3); k > 0 && len(q.waiting) < MaxWaiting; k-- {
			q.waiting = append(q.waiting, q.dests[s.prng.IntN(len(q.dests))])
		}
		q.arrivals++
	}
}

// WaitingAt is the number of passengers waiting at the stop.
func (s *Sim) WaitingAt(id street.StopID) int {
	if q := s.queues[id]; q != nil {
		return len(q.waiting)
	}
	return 0
}

// TotalWaiting is the number of passengers waiting at all stops.
func (s *Sim) TotalWaiting() int {
	n := 0
	for _, q := range s.queues {
		n += len(q.waiting)
	}
	return n
}

// Capacity is the most passengers the tram carries.
func (s *Sim) Capacity(v *Vehicle) int {
	l := s.Net.Lines[v.Line]
	if l == nil {
		return 0
	}
	return v.Cars * l.SectionLoad()
}

// Load is the number of passengers on board.
func (v *Vehicle) Load() int { return v.load }

// serve starts the work at the stop the tram has just reached: the
// passengers for this stop (everyone, at a terminus) will get off one by
// one, then those waiting who go its way get on one by one (stepService),
// as in the bus game.
func (s *Sim) serve(v *Vehicle) {
	stop := v.platform.Stop
	l := s.Net.Lines[v.Line]
	if l == nil {
		return
	}
	terminus := l.TurnsAt(v.call) // everyone gets off where the tram turns back
	v.alightLeft = v.riders[stop]
	if terminus {
		v.alightLeft = v.load
	}
	v.alightTimer, v.boardTimer, v.served = 0, 0, true
	v.doors, v.doorT = DoorsOpening, 0   // passengers move once they are open
	s.countStop(v, stop, 0, 0, terminus) // a one-way run ends here
}

// stepService moves passengers through the doors for one tick. Every
// section works at once, each serving DoorsPerSection passengers at a time
// at the line's seconds per passenger; passengers get off before others
// get on.
func (s *Sim) stepService(v *Vehicle) {
	l := s.Net.Lines[v.Line]
	if l == nil {
		return
	}
	doors := float64(max(v.Cars, 1) * l.DoorsPerSection())
	if v.alightLeft > 0 {
		v.alightTimer += DT
		each := l.AlightSeconds() / doors
		for v.alightLeft > 0 && v.alightTimer >= each {
			v.alightTimer -= each
			s.alightOne(v, l)
		}
		return
	}
	if s.Capacity(v) <= v.load || s.nextBoarder(v, l) < 0 {
		v.boardTimer = 0
		return
	}
	v.boardTimer += DT
	each := l.BoardSeconds() / doors
	for v.boardTimer >= each {
		v.boardTimer -= each
		if !s.boardOne(v, l) {
			v.boardTimer = 0
			return
		}
	}
}

// serviceDone reports whether nobody is left to get off or on.
func (s *Sim) serviceDone(v *Vehicle) bool {
	l := s.Net.Lines[v.Line]
	return l == nil || v.alightLeft == 0 && (s.Capacity(v) <= v.load || s.nextBoarder(v, l) < 0)
}

// alightOne lets one passenger off: one for this stop, or at a terminus
// anyone.
func (s *Sim) alightOne(v *Vehicle, l *street.Line) {
	stop := v.platform.Stop
	dest := stop
	if v.riders[stop] == 0 {
		dests := make([]street.StopID, 0, len(v.riders))
		for d, n := range v.riders {
			if n > 0 {
				dests = append(dests, d)
			}
		}
		if len(dests) == 0 {
			v.alightLeft = 0
			return
		}
		sort.Slice(dests, func(i, j int) bool { return dests[i] < dests[j] })
		dest = dests[0]
	}
	v.riders[dest]--
	if v.riders[dest] == 0 {
		delete(v.riders, dest)
	}
	v.load--
	v.alightLeft--
	s.Passengers.Delivered++
	s.countStop(v, stop, 0, 1, false)
}

// ahead is the set of stops the tram reaches before it turns back.
func (s *Sim) ahead(v *Vehicle, l *street.Line) map[street.StopID]bool {
	if v.aheadOf == v.call+1 && v.aheadSet != nil {
		return v.aheadSet
	}
	calls := l.Calls()
	set := map[street.StopID]bool{}
	for k := 1; k < len(calls); k++ {
		i := (v.call + k) % len(calls)
		set[calls[i]] = true
		if l.TurnsAt(i) {
			break // passengers for stops beyond wait for the way back
		}
	}
	v.aheadSet, v.aheadOf = set, v.call+1
	return set
}

// nextBoarder is the first passenger waiting (in arrival order) who goes
// the tram's way, or -1.
func (s *Sim) nextBoarder(v *Vehicle, l *street.Line) int {
	q := s.queues[v.platform.Stop]
	if q == nil {
		return -1
	}
	ahead := s.ahead(v, l)
	for i, d := range q.waiting {
		if ahead[d] && d != v.platform.Stop {
			return i
		}
	}
	return -1
}

// boardOne takes on the next passenger who goes the tram's way.
func (s *Sim) boardOne(v *Vehicle, l *street.Line) bool {
	if s.Capacity(v) <= v.load {
		return false
	}
	i := s.nextBoarder(v, l)
	if i < 0 {
		return false
	}
	q := s.queues[v.platform.Stop]
	d := q.waiting[i]
	q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
	if v.riders == nil {
		v.riders = map[street.StopID]int{}
	}
	v.riders[d]++
	v.load++
	s.Passengers.Boarded++
	s.countStop(v, v.platform.Stop, 1, 0, false)
	return true
}

// Alighting is how many passengers are still to get off at this stop.
func (v *Vehicle) Alighting() int { return v.alightLeft }

// Boarding is how many passengers waiting at the tram's stop go its way
// (and fit), while it stands there.
func (s *Sim) Boarding(v *Vehicle) int {
	l := s.Net.Lines[v.Line]
	q := s.queues[v.platform.Stop]
	if l == nil || q == nil || !v.Dwelling || !v.served {
		return 0
	}
	ahead := s.ahead(v, l)
	n := 0
	for _, d := range q.waiting {
		if ahead[d] && d != v.platform.Stop {
			n++
		}
	}
	return max(0, min(n, s.Capacity(v)-v.load))
}
