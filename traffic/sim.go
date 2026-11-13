// Package traffic simulates cars and street trams on the street lane graph.
// Both drive on sight with the Intelligent Driver Model, obey traffic
// signals and reserve junction movements so conflicting paths are never
// occupied at the same time. Engine-free, fixed tick, deterministic.
package traffic

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

const DT = 1.0 / 60

type Kind uint8

const (
	CarKind Kind = iota
	TramKind
)

type Vehicle struct {
	ID     int
	Kind   Kind
	Length float64
	Width  float64
	V0     float64 // desired speed on an open road
	A, B   float64 // max acceleration, comfortable deceleration
	S0, T  float64 // jam gap, time headway

	Path     []street.LinkID
	idx      int     // current (head) link in Path
	S        float64 // head position along Path[idx]
	V        float64
	admitted int // connectors up to this Path index have been granted

	// Cars
	DestPos float64 // despawn once the head passes this on the last link
	Variant uint8

	// Trams
	Line      street.LineID
	Cars      int // sections in the set
	call      int // index into the line's calling pattern of the next stop
	platform  street.Platform
	platIdx   int // Path index of the platform link, -1 when not heading to one
	Dwelling  bool
	DwellLeft float64

	reversing bool // dwelling to change ends before running onto the twin link

	stuck float64 // seconds spent stopped while not dwelling (diagnostics)
}

// Link returns the link the head is on and the position along it.
func (v *Vehicle) Link() (street.LinkID, float64) { return v.Path[v.idx], v.S }

type EventKind uint8

const (
	TramArrive EventKind = iota
	TramDepart
)

type Event struct {
	Tick    int64
	Vehicle int
	Kind    EventKind
	Stop    street.StopID
}

type Stats struct {
	Trips        int // car trips completed
	Spawned      int
	TramArrivals int
	MaxStuck     float64 // longest any vehicle has waited (excluding dwell)
	GaveUp       int     // cars removed after waiting longer than GiveUpAfter
	Reversals    int     // trams changing ends
}

type entry struct {
	v       *Vehicle
	front   float64 // head position relative to the link start
	opposed bool    // registered from the twin link: travelling the other way
}

type Sim struct {
	Net        *street.Network
	Vehicles   []*Vehicle
	Tick       int64
	TargetCars int
	Events     []Event
	Stats      Stats

	rng      *rand.Rand
	nextID   int
	carLanes []street.LinkID
	depots   []*depot

	// Rebuilt at the start of each tick, updated as admissions happen.
	occ     [][]entry
	holders []int
	inbound []float64
	// Track-block holders by direction sign.
	holdPos, holdNeg []int
}

const (
	GiveUpAfter  = 240.0 // s
	ReverseTime  = 8.0   // s for a tram to change ends
	TramDwell    = 20.0
	tramStartGap = 10.0
	lookahead    = 220.0
	exitMargin   = 2.0
)

// depot releases a line's trams onto its first platform, spaced by the
// headway, whenever the platform track is clear.
type depot struct {
	line    street.LineID
	start   street.Platform
	cars    int
	left    int
	next    float64 // s: earliest release of the next tram
	headway float64
}

// New creates a simulation with each line's fleet entering service one
// headway apart, and cars filling in to targetCars over the first seconds.
func New(n *street.Network, targetCars int, seed uint64) *Sim {
	s := &Sim{Net: n, TargetCars: targetCars, rng: rand.New(rand.NewPCG(seed, 0x636172))}
	for i := range n.Links {
		l := &n.Links[i]
		if l.Kind == street.LaneLink && l.Modes&street.Car != 0 && l.Length > 50 {
			s.carLanes = append(s.carLanes, l.ID)
		}
	}
	for i, id := range n.LineIDs() {
		l := n.Lines[id]
		start, err := n.LineCycle(l)
		if err != nil {
			continue
		}
		fleet := min(l.Fleet(), MaxFleet(n, l))
		s.depots = append(s.depots, &depot{line: id, start: start, cars: l.Cars(), left: fleet,
			next: tramStartGap * float64(i), headway: CycleTime(n, l) / float64(fleet)})
	}
	return s
}

// CycleTime estimates the seconds a tram takes to run the line's whole
// calling pattern and come back to the first stop.
func CycleTime(n *street.Network, l *street.Line) float64 {
	cur, err := n.LineCycle(l)
	if err != nil {
		return 0
	}
	const avgSpeed = 7.0 // m/s with signals, junctions and braking
	calls := l.Calls()
	t := 0.0
	for i := 1; i <= len(calls); i++ {
		path, p, ok := n.TramRoute(cur.Link, cur.Pos, calls[i%len(calls)])
		if !ok {
			break
		}
		d := -cur.Pos
		for k, id := range path {
			d += n.Links[id].Length
			if k > 0 && n.Links[path[k-1]].Twin == id {
				t += ReverseTime + 10
			}
		}
		d -= n.Links[path[len(path)-1]].Length - p.Pos
		t += math.Max(d, 0)/avgSpeed + TramDwell
		cur = p
	}
	return t
}

// MaxFleet is how many trams the line can run: one when its whole route is
// a single stretch of single track with nowhere for trams to pass.
func MaxFleet(n *street.Network, l *street.Line) int {
	cur, err := n.LineCycle(l)
	if err != nil {
		return 0
	}
	block := -2
	calls := l.Calls()
	for i := 1; i <= len(calls); i++ {
		path, p, ok := n.TramRoute(cur.Link, cur.Pos, calls[i%len(calls)])
		if !ok {
			break
		}
		for _, id := range path {
			lk := &n.Links[id]
			if lk.Block < 0 || n.Blocks[lk.Block].Kind != street.SingleTrackBlock || (block >= 0 && lk.Block != block) {
				return street.MaxTrams
			}
			block = lk.Block
		}
		cur = p
	}
	return 1
}

// headPos is where a tram of the given length stops its head for a
// platform: centred on the platform built for the longest set.
func headPos(p street.Platform, length float64) float64 {
	return p.Pos - (street.TramLength-length)/2
}

// release puts trams waiting at the depots into service. It reports whether
// any entered.
func (s *Sim) release() bool {
	added := false
	now := s.Seconds()
	for _, d := range s.depots {
		if d.left == 0 || now < d.next {
			continue
		}
		length := street.SetLength(d.cars)
		pos := headPos(d.start, length)
		v := &Vehicle{ID: s.nextID + 1, Kind: TramKind, Length: length, Width: 2.65, Cars: d.cars,
			V0: 50 / 3.6, A: 1.0, B: 1.3, S0: 3, T: 1.5,
			Path: []street.LinkID{d.start.Link}, S: pos, admitted: -1,
			Line: d.line, call: 0, platIdx: -1, Dwelling: true, DwellLeft: 1}
		if !s.clearFor(v) {
			continue // try again next tick
		}
		s.nextID++
		s.Vehicles = append(s.Vehicles, v)
		d.left--
		d.next = now + d.headway
		added = true
	}
	return added
}

// clearFor reports whether a tram can be placed where it stands.
func (s *Sim) clearFor(v *Vehicle) bool {
	id := v.Path[0]
	if s.inbound[id] > 0 {
		return false
	}
	if l := &s.Net.Links[id]; l.Block >= 0 {
		pos, neg := s.holdPos[l.Block], s.holdNeg[l.Block]
		switch {
		case s.Net.Blocks[l.Block].Kind == street.SingleTrackBlock, l.BlockSign < 0:
			if pos+neg > 0 {
				return false
			}
		case neg > 0:
			return false
		}
	}
	for _, e := range s.occ[id] {
		if e.front > v.S-v.Length-v.S0 && e.front-e.v.Length < v.S+v.S0 {
			return false
		}
	}
	return true
}

func (s *Sim) Seconds() float64 { return float64(s.Tick) * DT }

// Step advances one tick.
func (s *Sim) Step() {
	s.rebuildOccupancy()
	if s.release() {
		s.rebuildOccupancy()
	}
	acc := make([]float64, len(s.Vehicles))
	limit := make([]float64, len(s.Vehicles))
	for i, v := range s.Vehicles {
		acc[i], limit[i] = s.plan(v)
	}
	kept := s.Vehicles[:0]
	for i, v := range s.Vehicles {
		if s.move(v, acc[i], limit[i]) {
			kept = append(kept, v)
		}
	}
	for i := len(kept); i < len(s.Vehicles); i++ {
		s.Vehicles[i] = nil
	}
	s.Vehicles = kept
	s.spawnCars()
	s.Tick++
}

// --- occupancy --------------------------------------------------------------

// covered calls f for every link the vehicle body touches, head first, with
// the head position relative to that link's start.
func (s *Sim) covered(v *Vehicle, f func(k int, front float64)) {
	front := v.S
	for k := v.idx; k >= 0; k-- {
		if k < v.idx {
			front += s.Net.Links[v.Path[k]].Length
		}
		if k < v.idx && front-v.Length >= s.Net.Links[v.Path[k]].Length {
			return
		}
		f(k, front)
	}
}

func (s *Sim) rebuildOccupancy() {
	nl := len(s.Net.Links)
	if len(s.occ) != nl {
		s.occ = make([][]entry, nl)
		s.holders = make([]int, nl)
		s.inbound = make([]float64, nl)
	}
	for i := range s.occ {
		s.occ[i] = s.occ[i][:0]
		s.holders[i] = 0
		s.inbound[i] = 0
	}
	nb := len(s.Net.Blocks)
	if len(s.holdPos) != nb {
		s.holdPos, s.holdNeg = make([]int, nb), make([]int, nb)
	}
	for i := range s.holdPos {
		s.holdPos[i], s.holdNeg[i] = 0, 0
	}
	for _, v := range s.Vehicles {
		first := v.idx
		s.covered(v, func(k int, front float64) {
			id := v.Path[k]
			s.occ[id] = append(s.occ[id], entry{v: v, front: front})
			if tw := s.Net.Links[id].Twin; tw >= 0 {
				// Physically on the twin too, facing the other way.
				s.occ[tw] = append(s.occ[tw], entry{v: v, front: s.Net.Links[id].Length - (front - v.Length), opposed: true})
			}
			first = k
		})
		pos, neg := s.ownHold(v)
		for b := range pos {
			s.holdPos[b]++
		}
		for b := range neg {
			s.holdNeg[b]++
		}
		for j := first; j <= v.admitted && j < len(v.Path); j++ {
			if s.Net.Links[v.Path[j]].Kind == street.ConnectorLink {
				s.holders[v.Path[j]]++
			}
		}
		if v.admitted > v.idx && v.admitted+1 < len(v.Path) {
			s.inbound[v.Path[v.admitted+1]] += v.Length + exitMargin
		}
	}
	for i := range s.occ {
		sort.Slice(s.occ[i], func(a, b int) bool {
			ea, eb := s.occ[i][a], s.occ[i][b]
			if ea.front != eb.front {
				return ea.front < eb.front
			}
			return ea.v.ID < eb.v.ID
		})
	}
}

// ownHold lists the track blocks (by direction) the vehicle occupies or has
// been granted.
func (s *Sim) ownHold(v *Vehicle) (pos, neg map[int]bool) {
	first := v.idx
	s.covered(v, func(k int, _ float64) { first = k })
	last := min(max(v.idx, v.admitted+1), len(v.Path)-1)
	for k := first; k <= last; k++ {
		l := &s.Net.Links[v.Path[k]]
		if l.Block < 0 {
			continue
		}
		if l.BlockSign > 0 {
			if pos == nil {
				pos = map[int]bool{}
			}
			pos[l.Block] = true
		} else {
			if neg == nil {
				neg = map[int]bool{}
			}
			neg[l.Block] = true
		}
	}
	return pos, neg
}

// blockOK checks a block without claiming it.
func (s *Sim) blockOK(v *Vehicle, id street.LinkID) bool {
	b := s.Net.Links[id].Block
	if b < 0 {
		return true
	}
	p, n := s.holdPos[b], s.holdNeg[b]
	ok := s.blockFree(v, id)
	s.holdPos[b], s.holdNeg[b] = p, n
	return ok
}

// blockFree reports whether the vehicle may enter link id as far as track
// blocks are concerned, and claims it if so.
func (s *Sim) blockFree(v *Vehicle, id street.LinkID) bool {
	l := &s.Net.Links[id]
	if l.Block < 0 {
		return true
	}
	b := l.Block
	pos, neg := s.holdPos[b], s.holdNeg[b]
	ownPos, ownNeg := s.ownHold(v)
	if ownPos[b] {
		pos--
	}
	if ownNeg[b] {
		neg--
	}
	ok := false
	switch s.Net.Blocks[b].Kind {
	case street.SingleTrackBlock:
		ok = pos+neg == 0 || ownPos[b] || ownNeg[b]
	case street.TurnbackBlock:
		if l.BlockSign < 0 {
			ok = pos+neg == 0 || ownNeg[b]
		} else {
			ok = neg == 0
		}
	}
	if !ok {
		return false
	}
	if l.BlockSign > 0 && !ownPos[b] {
		s.holdPos[b]++
	}
	if l.BlockSign < 0 && !ownNeg[b] {
		s.holdNeg[b]++
	}
	return true
}

// --- driving ----------------------------------------------------------------

func idm(v *Vehicle, speed, v0, gap, dv, s0 float64) float64 {
	sstar := s0 + math.Max(0, speed*v.T+speed*dv/(2*math.Sqrt(v.A*v.B)))
	free := 1 - math.Pow(speed/math.Max(v0, 0.1), 4)
	return v.A * (free - math.Pow(sstar/math.Max(gap, 0.05), 2))
}

// plan computes the vehicle's acceleration for this tick and the furthest
// it may move (the nearest obstacle), granting junction admissions on the
// way.
func (s *Sim) plan(v *Vehicle) (float64, float64) {
	if v.Dwelling {
		return 0, 0
	}
	net := s.Net
	cur := &net.Links[v.Path[v.idx]]
	v0 := math.Min(v.V0, cur.SpeedLimit)
	acc := math.Inf(1)
	limit := math.Inf(1)
	leaderGap := math.Inf(1) // nearest vehicle ahead
	obstacle := func(gap, speed, s0 float64) {
		acc = math.Min(acc, idm(v, v.V, v0, gap, v.V-speed, s0))
		limit = math.Min(limit, math.Max(0, gap-0.05))
	}
	vehicle := func(e entry, gap float64) {
		speed := e.v.V
		if e.opposed {
			speed = 0 // oncoming: treat as a standing obstacle
		}
		obstacle(gap, speed, v.S0)
		leaderGap = math.Min(leaderGap, gap)
	}

	// Leader on the current link.
	for _, e := range s.occ[v.Path[v.idx]] {
		if e.v != v && e.front > v.S {
			vehicle(e, e.front-e.v.Length-v.S)
			break
		}
	}

	d := cur.Length - v.S // distance from head to the start of Path[k]
	stopped := false
	for k := v.idx + 1; k < len(v.Path) && d < lookahead && !stopped; k++ {
		l := &net.Links[v.Path[k]]
		if net.Links[v.Path[k-1]].Twin == l.ID {
			obstacle(d, 0, 0.2) // stop at the track end to change ends
			break
		}
		// Slow down in time for lower speed limits ahead.
		v0 = math.Min(v0, math.Sqrt(l.SpeedLimit*l.SpeedLimit+2*v.B*d))
		if l.Kind != street.LaneLink && k > v.admitted {
			// Only the first vehicle in the queue may claim the movement:
			// a follower holding it would block other movements while it
			// waits behind its leader.
			check := v.V*v.V/(2*v.B) + 2*v.V + 8
			if d > check || leaderGap < d || !s.admit(v, k, d) {
				obstacle(d, 0, 0.5)
				stopped = true
				break
			}
		}
		for _, e := range s.occ[l.ID] {
			if e.v != v {
				vehicle(e, d+e.front-e.v.Length)
				break
			}
		}
		if v.Kind == TramKind && k == v.platIdx {
			obstacle(d+v.platform.Pos, 0, 0.2)
		}
		d += l.Length
	}
	if v.Kind == TramKind && v.platIdx == v.idx {
		obstacle(v.platform.Pos-v.S, 0, 0.2)
	}
	acc = math.Min(acc, v.A*(1-math.Pow(v.V/math.Max(v0, 0.1), 4)))
	return math.Max(acc, -9), limit
}

// admit tries to grant the movement Path[k] (a junction connector) at
// distance d ahead.
func (s *Sim) admit(v *Vehicle, k int, d float64) bool {
	net := s.Net
	c := &net.Links[v.Path[k]]
	if sig := net.Signal(c.Junction); c.Kind == street.ConnectorLink && sig != nil {
		switch sig.AspectAt(c.Phase, s.Seconds()) {
		case street.Red:
			return false
		case street.Amber:
			if d > v.V*v.V/(2*3.0) { // can still stop comfortably
				return false
			}
		}
	}
	for _, cc := range c.Conflicts {
		if s.holders[cc] > 0 {
			return false
		}
	}
	// Track blocks: check both before claiming either.
	if !s.blockOK(v, v.Path[k]) || (k+1 < len(v.Path) && !s.blockOK(v, v.Path[k+1])) {
		return false
	}
	if k+1 < len(v.Path) {
		out := v.Path[k+1]
		ol := net.Links[out].Length
		free := ol
		for _, e := range s.occ[out] {
			if e.v != v {
				free = math.Min(free, e.front-e.v.Length)
				break
			}
		}
		free -= s.inbound[out]
		need := math.Min(v.Length+exitMargin, ol)
		if free < need {
			return false
		}
		s.inbound[out] += v.Length + exitMargin
	}
	s.blockFree(v, v.Path[k])
	if k+1 < len(v.Path) {
		s.blockFree(v, v.Path[k+1])
	}
	v.admitted = k
	s.holders[c.ID]++
	return true
}

// move integrates one tick. It returns false when the vehicle leaves.
func (s *Sim) move(v *Vehicle, acc, limit float64) bool {
	if v.Dwelling {
		v.DwellLeft -= DT
		if v.DwellLeft <= 0 {
			if v.reversing {
				s.reverse(v)
			} else {
				s.depart(v)
			}
		}
		return true
	}
	v.V = math.Max(0, v.V+acc*DT)
	step := v.V * DT
	if step > limit {
		step = limit
		v.V = math.Min(v.V, step/DT)
	}
	v.S += step
	for v.idx+1 < len(v.Path) && v.S >= s.Net.Links[v.Path[v.idx]].Length {
		if s.Net.Links[v.Path[v.idx+1]].Kind != street.LaneLink && v.idx+1 > v.admitted {
			// Never roll into a junction or crossover without admission.
			v.S, v.V = s.Net.Links[v.Path[v.idx]].Length, 0
			break
		}
		v.S -= s.Net.Links[v.Path[v.idx]].Length
		v.idx++
	}
	if v.V < 0.1 {
		v.stuck += DT
		s.Stats.MaxStuck = math.Max(s.Stats.MaxStuck, v.stuck)
	} else {
		v.stuck = 0
	}

	switch v.Kind {
	case CarKind:
		if v.idx == len(v.Path)-1 && v.S >= v.DestPos {
			s.Stats.Trips++
			return false
		}
		// Clear cars hopelessly stuck in congestion, but never from inside
		// a junction (they hold a movement and must finish it).
		if v.stuck > GiveUpAfter && v.admitted <= v.idx && s.Net.Links[v.Path[v.idx]].Kind == street.LaneLink {
			s.Stats.GaveUp++
			return false
		}
	case TramKind:
		if v.idx == v.platIdx && math.Abs(v.platform.Pos-v.S) < 0.6 && v.V < 0.5 {
			v.V, v.Dwelling, v.DwellLeft, v.platIdx = 0, true, TramDwell, -1
			v.stuck = 0
			s.Stats.TramArrivals++
			s.Events = append(s.Events, Event{Tick: s.Tick, Vehicle: v.ID, Kind: TramArrive, Stop: v.platform.Stop})
		}
		if !v.Dwelling && v.idx+1 < len(v.Path) && s.Net.Links[v.Path[v.idx]].Twin == v.Path[v.idx+1] &&
			s.Net.Links[v.Path[v.idx]].Length-v.S < 0.5 && v.V < 0.3 {
			v.V, v.Dwelling, v.reversing, v.DwellLeft = 0, true, true, ReverseTime
			v.stuck = 0
		}
		s.trimPath(v)
	}
	return true
}

// reverse moves a stopped tram onto the twin of its link: the driver has
// walked to the other cab, so the old tail is now the head.
func (s *Sim) reverse(v *Vehicle) {
	k := v.idx + 1
	tw := &s.Net.Links[v.Path[k]]
	tail := math.Max(0, v.S-v.Length)
	v.S = math.Min(tw.Length, tw.Length-tail)
	v.Path = append([]street.LinkID(nil), v.Path[k:]...)
	v.idx = 0
	v.admitted -= k
	if v.platIdx >= 0 {
		v.platIdx -= k
	}
	v.Dwelling, v.reversing = false, false
	s.Stats.Reversals++
}

// depart sends a tram towards the next stop of its line.
func (s *Sim) depart(v *Vehicle) {
	line := s.Net.Lines[v.Line]
	if line == nil {
		v.DwellLeft = 1e9 // line was deleted; park
		return
	}
	calls := line.Calls()
	next := (v.call + 1) % len(calls)
	path, p, ok := s.Net.TramRoute(v.Path[v.idx], v.S, calls[next])
	if !ok {
		v.DwellLeft = 5 // track missing; try again later
		return
	}
	s.Events = append(s.Events, Event{Tick: s.Tick, Vehicle: v.ID, Kind: TramDepart, Stop: calls[v.call]})
	v.call = next
	v.Path = append(v.Path[:v.idx+1], path[1:]...)
	p.Pos = headPos(p, v.Length)
	v.platform, v.platIdx = p, v.idx+len(path)-1
	v.Dwelling = false
}

// trimPath drops links the tram has fully left so paths stay short.
func (s *Sim) trimPath(v *Vehicle) {
	first := v.idx
	s.covered(v, func(k int, _ float64) { first = k })
	if first < 16 {
		return
	}
	v.Path = append([]street.LinkID(nil), v.Path[first:]...)
	v.idx -= first
	v.admitted -= first
	if v.platIdx >= 0 {
		v.platIdx -= first
	}
}

// --- cars -------------------------------------------------------------------

func (s *Sim) CarCount() int {
	n := 0
	for _, v := range s.Vehicles {
		if v.Kind == CarKind {
			n++
		}
	}
	return n
}

func (s *Sim) spawnCars() {
	if len(s.carLanes) < 2 {
		return
	}
	need := s.TargetCars - s.CarCount()
	for try := 0; try < 4 && need > 0; try++ {
		if s.trySpawn() {
			need--
		}
	}
}

const carLength = 4.5

func (s *Sim) trySpawn() bool {
	from := s.carLanes[s.rng.IntN(len(s.carLanes))]
	to := s.carLanes[s.rng.IntN(len(s.carLanes))]
	if from == to {
		return false
	}
	fl := &s.Net.Links[from]
	pos := 15 + s.rng.Float64()*(fl.Length-30)
	if pos-carLength < s.inbound[from]+5 {
		return false
	}
	used := s.inbound[from]
	for _, e := range s.occ[from] {
		used += e.v.Length + e.v.S0
	}
	if used > fl.Length/2 {
		return false // do not add cars to a congested lane
	}
	for _, e := range s.occ[from] {
		if e.front+12 > pos-carLength && e.front-e.v.Length-12 < pos {
			return false
		}
	}
	path, ok := s.Net.Route(from, map[street.LinkID]bool{to: true}, street.Car, street.CarCost)
	if !ok {
		return false
	}
	s.nextID++
	v := &Vehicle{ID: s.nextID, Kind: CarKind, Length: carLength, Width: 1.9,
		V0: (50 / 3.6) * (0.9 + 0.25*s.rng.Float64()), A: 1.6, B: 2.5, S0: 2, T: 1.3,
		Path: path, S: pos, admitted: -1, DestPos: s.Net.Links[to].Length / 2,
		Variant: uint8(s.rng.IntN(8)), platIdx: -1}
	s.Vehicles = append(s.Vehicles, v)
	s.occ[from] = append(s.occ[from], entry{v: v, front: pos})
	sort.Slice(s.occ[from], func(a, b int) bool { return s.occ[from][a].front < s.occ[from][b].front })
	s.Stats.Spawned++
	return true
}

// Body returns points along the vehicle from tail to head.
func (s *Sim) Body(v *Vehicle) []street.Vec {
	var parts [][]street.Vec
	s.covered(v, func(k int, front float64) {
		l := &s.Net.Links[v.Path[k]]
		if pts := l.Slice(front-v.Length, front); len(pts) > 0 {
			parts = append(parts, pts)
		}
	})
	var out []street.Vec
	for i := len(parts) - 1; i >= 0; i-- {
		out = append(out, parts[i]...)
	}
	return out
}

// TramStop is the stop a tram is standing at or heading for (-1 if its line
// is gone).
func (s *Sim) TramStop(v *Vehicle) street.StopID {
	l := s.Net.Lines[v.Line]
	if l == nil {
		return -1
	}
	calls := l.Calls()
	return calls[v.call%len(calls)]
}

// Waiting is the number of trams of the line not yet in service.
func (s *Sim) Waiting(id street.LineID) int {
	for _, d := range s.depots {
		if d.line == id {
			return d.left
		}
	}
	return 0
}

// Reversing reports whether a tram is stopped to change ends.
func (v *Vehicle) Reversing() bool { return v.reversing }

// Find returns the vehicle with the given ID, or nil once it has left.
func (s *Sim) Find(id int) *Vehicle {
	for _, v := range s.Vehicles {
		if v.ID == id {
			return v
		}
	}
	return nil
}
