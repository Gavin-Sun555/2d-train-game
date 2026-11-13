package build

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

type NodeID int
type SegID int
type StopID int
type LineID int

type Node struct {
	ID    NodeID
	Pos   sim.Vec2
	Level sim.Level
}

type Segment struct {
	ID    SegID
	A, B  NodeID
	Level sim.Level
}

type Stop struct {
	ID   StopID
	Node NodeID
	Name string
}

type Line struct {
	ID    LineID
	Mode  Mode
	Stops []StopID
}

// Plan is everything the player has built. IDs are stable across edits and
// iteration is always in ID order so compiles are deterministic.
type Plan struct {
	City *city.Map

	nodes  map[NodeID]*Node
	segs   map[SegID]*Segment
	stops  map[StopID]*Stop
	lines  map[LineID]*Line
	nextID int
}

func NewPlan(m *city.Map) *Plan {
	return &Plan{
		City:  m,
		nodes: map[NodeID]*Node{},
		segs:  map[SegID]*Segment{},
		stops: map[StopID]*Stop{},
		lines: map[LineID]*Line{},
	}
}

func (p *Plan) id() int { p.nextID++; return p.nextID }

func sortedKeys[K ~int, V any](m map[K]V) []K {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

func (p *Plan) Nodes() []*Node {
	out := []*Node{}
	for _, k := range sortedKeys(p.nodes) {
		out = append(out, p.nodes[k])
	}
	return out
}

func (p *Plan) Segments() []*Segment {
	out := []*Segment{}
	for _, k := range sortedKeys(p.segs) {
		out = append(out, p.segs[k])
	}
	return out
}

func (p *Plan) Stops() []*Stop {
	out := []*Stop{}
	for _, k := range sortedKeys(p.stops) {
		out = append(out, p.stops[k])
	}
	return out
}

func (p *Plan) Lines() []*Line {
	out := []*Line{}
	for _, k := range sortedKeys(p.lines) {
		out = append(out, p.lines[k])
	}
	return out
}

func (p *Plan) Node(id NodeID) *Node      { return p.nodes[id] }
func (p *Plan) Segment(id SegID) *Segment { return p.segs[id] }
func (p *Plan) Stop(id StopID) *Stop      { return p.stops[id] }
func (p *Plan) SegEnds(s *Segment) (sim.Vec2, sim.Vec2) {
	return p.nodes[s.A].Pos, p.nodes[s.B].Pos
}

func (p *Plan) degree(n NodeID) int {
	d := 0
	for _, s := range p.segs {
		if s.A == n || s.B == n {
			d++
		}
	}
	return d
}

func (p *Plan) stopAt(n NodeID) *Stop {
	for _, k := range sortedKeys(p.stops) {
		if p.stops[k].Node == n {
			return p.stops[k]
		}
	}
	return nil
}

// --- geometry ---------------------------------------------------------------

func sub(a, b sim.Vec2) sim.Vec2             { return sim.Vec2{X: a.X - b.X, Y: a.Y - b.Y} }
func dot(a, b sim.Vec2) float64              { return a.X*b.X + a.Y*b.Y }
func cross(a, b sim.Vec2) float64            { return a.X*b.Y - a.Y*b.X }
func dist(a, b sim.Vec2) float64             { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func lerp(a, b sim.Vec2, t float64) sim.Vec2 { return a.Lerp(b, t) }

// project returns the parameter t∈[0,1] of the closest point on ab to q and
// the distance from q to it.
func project(q, a, b sim.Vec2) (float64, float64) {
	ab := sub(b, a)
	l2 := dot(ab, ab)
	if l2 == 0 {
		return 0, dist(q, a)
	}
	t := math.Max(0, math.Min(1, dot(sub(q, a), ab)/l2))
	return t, dist(q, lerp(a, b, t))
}

// intersect returns the parameters (t on ab, u on cd) of a proper crossing.
func intersect(a, b, c, d sim.Vec2) (float64, float64, bool) {
	r, s := sub(b, a), sub(d, c)
	den := cross(r, s)
	if math.Abs(den) < 1e-9 {
		return 0, 0, false
	}
	ca := sub(c, a)
	t, u := cross(ca, s)/den, cross(ca, r)/den
	if t < -1e-9 || t > 1+1e-9 || u < -1e-9 || u > 1+1e-9 {
		return 0, 0, false
	}
	return t, u, true
}

// --- queries ----------------------------------------------------------------

// NearestNode returns the closest node on a level within maxDist.
func (p *Plan) NearestNode(q sim.Vec2, level sim.Level, maxDist float64) (*Node, bool) {
	var best *Node
	for _, n := range p.Nodes() {
		if n.Level != level {
			continue
		}
		if d := dist(n.Pos, q); d <= maxDist && (best == nil || d < dist(best.Pos, q)) {
			best = n
		}
	}
	return best, best != nil
}

// NearestSegment returns the closest segment on a level within maxDist and
// the parameter of the closest point on it.
func (p *Plan) NearestSegment(q sim.Vec2, level sim.Level, maxDist float64) (*Segment, float64, bool) {
	var best *Segment
	bestT, bestD := 0.0, maxDist
	for _, s := range p.Segments() {
		if s.Level != level {
			continue
		}
		a, b := p.SegEnds(s)
		if t, d := project(q, a, b); d <= bestD {
			best, bestT, bestD = s, t, d
		}
	}
	return best, bestT, best != nil
}

// Snap moves a cursor position onto an existing node within radius, else
// onto the half-tile grid (street centre lines lie on it).
func (p *Plan) Snap(q sim.Vec2, level sim.Level, radius float64) sim.Vec2 {
	if n, ok := p.NearestNode(q, level, radius); ok {
		return n.Pos
	}
	g := city.TileSize / 2
	return sim.Vec2{X: math.Round(q.X/g) * g, Y: math.Round(q.Y/g) * g}
}

// --- validation -------------------------------------------------------------

var (
	ErrTooShort     = errors.New("segment too short")
	ErrNotBuildable = errors.New("this level is not available yet")
	ErrOverlap      = errors.New("overlaps existing track")
)

// TerrainError names the first tile a segment may not cross.
type TerrainError struct {
	At   sim.Vec2
	Kind city.Kind
}

func (e TerrainError) Error() string { return fmt.Sprintf("cannot build through %s", e.Kind) }

// CheckSegment validates a proposed straight segment without changing anything.
func (p *Plan) CheckSegment(a, b sim.Vec2, level sim.Level) error {
	rules := Rules[level]
	if !rules.Buildable {
		return ErrNotBuildable
	}
	l := dist(a, b)
	if l < MinSegment {
		return ErrTooShort
	}
	n := int(math.Ceil(l / sampleStep))
	for i := 0; i <= n; i++ {
		q := lerp(a, b, float64(i)/float64(n))
		if k := p.City.TileAt(q).Kind; !rules.Allows(k) {
			return TerrainError{At: q, Kind: k}
		}
	}
	dir := sub(b, a)
	for _, s := range p.Segments() {
		if s.Level != level {
			continue
		}
		c, d := p.SegEnds(s)
		if math.Abs(cross(dir, sub(d, c)))/(l*dist(c, d)) > 1e-6 {
			continue // not parallel
		}
		if math.Abs(cross(dir, sub(c, a)))/l > snapEps {
			continue // parallel but on a different line
		}
		// Collinear: overlap if the shared length exceeds snapEps.
		t0 := dot(sub(c, a), dir) / (l * l)
		t1 := dot(sub(d, a), dir) / (l * l)
		lo, hi := math.Max(0, math.Min(t0, t1)), math.Min(1, math.Max(t0, t1))
		if (hi-lo)*l > snapEps {
			return ErrOverlap
		}
	}
	return nil
}

// --- edits ------------------------------------------------------------------

// nodeAt returns the node at q, splitting a segment or creating a node when
// needed.
func (p *Plan) nodeAt(q sim.Vec2, level sim.Level) NodeID {
	if n, ok := p.NearestNode(q, level, snapEps); ok {
		return n.ID
	}
	n := &Node{ID: NodeID(p.id()), Pos: q, Level: level}
	p.nodes[n.ID] = n
	if s, _, ok := p.NearestSegment(q, level, snapEps); ok {
		p.split(s, n.ID)
	}
	return n.ID
}

// split replaces segment s with two segments meeting at node m.
func (p *Plan) split(s *Segment, m NodeID) {
	delete(p.segs, s.ID)
	for _, ends := range [][2]NodeID{{s.A, m}, {m, s.B}} {
		ns := &Segment{ID: SegID(p.id()), A: ends[0], B: ends[1], Level: s.Level}
		p.segs[ns.ID] = ns
	}
}

// AddSegment builds straight track from a to b. Crossing existing track on
// the same level creates a junction there, and ending on existing track
// joins it.
func (p *Plan) AddSegment(a, b sim.Vec2, level sim.Level) ([]SegID, error) {
	if err := p.CheckSegment(a, b, level); err != nil {
		return nil, err
	}
	ts := []float64{0, 1}
	for _, s := range p.Segments() {
		if s.Level != level {
			continue
		}
		c, d := p.SegEnds(s)
		if t, _, ok := intersect(a, b, c, d); ok {
			ts = append(ts, t)
		}
	}
	for _, n := range p.Nodes() {
		if n.Level != level {
			continue
		}
		if t, d := project(n.Pos, a, b); d <= snapEps {
			ts = append(ts, t)
		}
	}
	sort.Float64s(ts)

	var ids []NodeID
	var last sim.Vec2
	for i, t := range ts {
		q := lerp(a, b, t)
		switch i {
		case 0:
			q = a
		case len(ts) - 1:
			q = b
		}
		if len(ids) > 0 && dist(q, last) < snapEps {
			continue
		}
		ids = append(ids, p.nodeAt(q, level))
		last = q
	}
	var out []SegID
	for i := 0; i+1 < len(ids); i++ {
		s := &Segment{ID: SegID(p.id()), A: ids[i], B: ids[i+1], Level: level}
		p.segs[s.ID] = s
		out = append(out, s.ID)
	}
	return out, nil
}

// DeleteSegment removes a segment, then any nodes and stops left unconnected,
// and drops removed stops from lines.
func (p *Plan) DeleteSegment(id SegID) {
	s, ok := p.segs[id]
	if !ok {
		return
	}
	delete(p.segs, id)
	for _, n := range []NodeID{s.A, s.B} {
		if p.degree(n) > 0 {
			continue
		}
		if st := p.stopAt(n); st != nil {
			p.deleteStop(st.ID)
		}
		delete(p.nodes, n)
	}
}

func (p *Plan) deleteStop(id StopID) {
	delete(p.stops, id)
	for _, lid := range sortedKeys(p.lines) {
		l := p.lines[lid]
		kept := l.Stops[:0]
		for _, s := range l.Stops {
			if s != id {
				kept = append(kept, s)
			}
		}
		l.Stops = kept
		if len(l.Stops) < 2 {
			delete(p.lines, lid)
		}
	}
}

var stopNames = []string{
	"Central", "Market St", "City Hall", "Riverside", "Park Gate", "Museum",
	"Harbour Rd", "University", "Old Town", "Library", "Hospital", "Stadium",
	"Union Sq", "Mill Lane", "Bridge St", "Garden Ave", "Cathedral", "Exchange",
	"North End", "South Quay", "Theatre", "Arcade", "Elm Row", "Foundry",
}

// AddStop places a stop on the track nearest q (within maxDist). Stops must
// sit on plain track, not on a junction.
func (p *Plan) AddStop(q sim.Vec2, level sim.Level, maxDist float64) (StopID, error) {
	s, t, ok := p.NearestSegment(q, level, maxDist)
	if !ok {
		return 0, errors.New("place stops on existing track")
	}
	a, b := p.SegEnds(s)
	at := lerp(a, b, t)
	var nid NodeID
	switch {
	case dist(at, a) < 3:
		nid = s.A
	case dist(at, b) < 3:
		nid = s.B
	default:
		nid = p.nodeAt(at, level)
	}
	if p.degree(nid) > 2 {
		return 0, errors.New("stops cannot be on a junction")
	}
	if st := p.stopAt(nid); st != nil {
		return st.ID, nil
	}
	st := &Stop{ID: StopID(p.id()), Node: nid, Name: stopNames[len(p.stops)%len(stopNames)]}
	if n := len(p.stops); n >= len(stopNames) {
		st.Name = fmt.Sprintf("%s %d", st.Name, n/len(stopNames)+1)
	}
	p.stops[st.ID] = st
	return st.ID, nil
}

// StopNear returns the stop closest to q within maxDist.
func (p *Plan) StopNear(q sim.Vec2, maxDist float64) (*Stop, bool) {
	var best *Stop
	bestD := maxDist
	for _, st := range p.Stops() {
		if d := dist(p.nodes[st.Node].Pos, q); d <= bestD {
			best, bestD = st, d
		}
	}
	return best, best != nil
}

// AddLine creates a line serving the stops in order (and back), after
// checking a vehicle can actually be routed through them.
func (p *Plan) AddLine(mode Mode, stops []StopID) (LineID, error) {
	if len(stops) < 2 {
		return 0, errors.New("a line needs at least two stops")
	}
	l := &Line{ID: LineID(p.id()), Mode: mode, Stops: append([]StopID(nil), stops...)}
	c := p.Compile()
	if _, err := c.route(l); err != nil {
		return 0, err
	}
	p.lines[l.ID] = l
	return l.ID, nil
}

func (p *Plan) DeleteLine(id LineID) { delete(p.lines, id) }
