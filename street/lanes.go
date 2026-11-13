package street

import (
	"container/heap"
	"fmt"
	"math"
)

type LinkID int

// Mode is a bit set of vehicle types allowed on a link.
type Mode uint8

const (
	Car Mode = 1 << iota
	Tram
)

type LinkKind uint8

const (
	LaneLink      LinkKind = iota // a lane (or a piece of one) along a road
	ConnectorLink                 // a path across a junction box
	CrossoverLink                 // tram crossover (a pair of switches) between tracks
)

type Turn uint8

const (
	Straight Turn = iota
	Left
	Right
)

// Link is one piece of the lane graph: a lane along a road, or a connector
// through a junction from one lane to another.
type Link struct {
	ID    LinkID
	Kind  LinkKind
	Modes Mode

	// Lanes
	Road    RoadID
	Forward bool // travels A→B
	Index   int  // 0 = innermost (next to the centre line)
	Track   bool // has tram track embedded
	Centre  bool // single track running along the road centre

	// Twin is the same piece of track travelled the other way (-1 if none).
	// A vehicle on one is physically on the other too.
	Twin LinkID
	// Block is the track block this link belongs to (-1 if none): single
	// track sections and terminus turnback zones, where trams may run
	// against the normal direction. BlockSign is the travel direction
	// within the block.
	Block     int
	BlockSign int8

	// Connectors
	Junction JunctionID
	From, To LinkID
	Turn     Turn
	Phase    int // signal phase group, -1 if the junction is unsignalised
	// Conflicts are connectors in the same junction whose paths cross or
	// merge with this one; two of them may never be occupied at once.
	Conflicts []LinkID

	Pts        []Vec
	cum        []float64
	Length     float64
	SpeedLimit float64
	Next       []LinkID
}

func (l *Link) setPts(pts []Vec) {
	l.Pts = pts
	l.cum = make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		l.cum[i] = l.cum[i-1] + dist(pts[i-1], pts[i])
	}
	l.Length = l.cum[len(pts)-1]
}

// PointAt returns the position and unit heading s metres along the link.
func (l *Link) PointAt(s float64) (Vec, Vec) {
	s = math.Max(0, math.Min(l.Length, s))
	i := 1
	for i < len(l.cum)-1 && l.cum[i] < s {
		i++
	}
	seg := l.cum[i] - l.cum[i-1]
	t := 0.0
	if seg > 0 {
		t = (s - l.cum[i-1]) / seg
	}
	return l.Pts[i-1].Lerp(l.Pts[i], t), unit(sub(l.Pts[i], l.Pts[i-1]))
}

// Slice returns the polyline between positions a and b along the link.
func (l *Link) Slice(a, b float64) []Vec {
	a, b = math.Max(0, a), math.Min(l.Length, b)
	if a >= b {
		return nil
	}
	p, _ := l.PointAt(a)
	out := []Vec{p}
	for i := 1; i < len(l.Pts)-1; i++ {
		if l.cum[i] > a && l.cum[i] < b {
			out = append(out, l.Pts[i])
		}
	}
	q, _ := l.PointAt(b)
	return append(out, q)
}

// SignalHead is where a signal for one approach stands (kerb side, at the
// stop line).
type SignalHead struct {
	Junction JunctionID
	Phase    int
	Pos      Vec
	Dir      Vec // direction of the traffic it controls
}

func (n *Network) SignalHeads() []SignalHead { return n.heads }

func hasCarConnector(n *Network, lane LinkID) bool {
	for _, c := range n.Links[lane].Next {
		if n.Links[c].Modes&Car != 0 {
			return true
		}
	}
	return false
}

func polylinesCross(a, b []Vec) bool {
	for i := 0; i+1 < len(a); i++ {
		for k := 0; k+1 < len(b); k++ {
			if segmentsCross(a[i], a[i+1], b[k], b[k+1]) {
				return true
			}
		}
	}
	return false
}

func segmentsCross(a, b, c, d Vec) bool {
	r, s := sub(b, a), sub(d, c)
	den := cross(r, s)
	if math.Abs(den) < 1e-12 {
		return false
	}
	ca := sub(c, a)
	t, u := cross(ca, s)/den, cross(ca, r)/den
	return t >= 0 && t <= 1 && u >= 0 && u <= 1
}

// --- routing ----------------------------------------------------------------

// Route finds the cheapest chain of links usable by mode m from link `from`
// to any link in targets (the result starts with `from`). cost scales each
// link's base cost (travel time at the speed limit).
func (n *Network) Route(from LinkID, targets map[LinkID]bool, m Mode, cost func(*Link) float64) ([]LinkID, bool) {
	dist := map[LinkID]float64{from: 0}
	prev := map[LinkID]LinkID{}
	pq := &linkQueue{{from, 0}}
	for pq.Len() > 0 {
		it := heap.Pop(pq).(linkItem)
		if it.d > dist[it.id] {
			continue
		}
		if it.id != from && targets[it.id] {
			path := []LinkID{it.id}
			for c := it.id; c != from; {
				c = prev[c]
				path = append(path, c)
			}
			for i, k := 0, len(path)-1; i < k; i, k = i+1, k-1 {
				path[i], path[k] = path[k], path[i]
			}
			return path, true
		}
		for _, nx := range n.Links[it.id].Next {
			l := &n.Links[nx]
			if l.Modes&m == 0 {
				continue
			}
			c := l.Length / l.SpeedLimit
			if cost != nil {
				c *= cost(l)
			}
			if n.Links[it.id].Twin == nx {
				c += ReversePenalty // stop and change ends
			}
			if d := it.d + c; !hasKey(dist, nx) || d < dist[nx] {
				dist[nx], prev[nx] = d, it.id
				heap.Push(pq, linkItem{nx, d})
			}
		}
	}
	return nil, false
}

func hasKey(m map[LinkID]float64, k LinkID) bool { _, ok := m[k]; return ok }

// CarCost makes cars avoid lanes shared with trams when they can.
func CarCost(l *Link) float64 {
	if l.Kind == LaneLink && l.Track {
		return 1.3
	}
	return 1
}

type linkItem struct {
	id LinkID
	d  float64
}

type linkQueue []linkItem

func (q linkQueue) Len() int { return len(q) }
func (q linkQueue) Less(i, j int) bool {
	if q[i].d != q[j].d {
		return q[i].d < q[j].d
	}
	return q[i].id < q[j].id
}
func (q linkQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *linkQueue) Push(x any)   { *q = append(*q, x.(linkItem)) }
func (q *linkQueue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// --- tram platforms and line validation --------------------------------------

// Tram sets are built from SectionLength modules. TramLength, the longest
// set, sizes platforms and turnback stubs.
const (
	SectionLength   = 6.0
	MinSections     = 2
	MaxSections     = 5
	DefaultSections = 3
	MaxTrams        = 12
	TramLength      = SectionLength * MaxSections
)

// SetLength is the length of a tram of the given number of sections.
func SetLength(sections int) float64 { return SectionLength * float64(sections) }

// Fleet is the number of trams on the line (at least one).
func (l *Line) Fleet() int { return min(max(l.Trams, 1), MaxTrams) }

// Cars is the number of sections in each of the line's trams.
func (l *Line) Cars() int {
	if l.Sections == 0 {
		return DefaultSections
	}
	return min(max(l.Sections, MinSections), MaxSections)
}

// ReversePenalty is the routing cost (s) of a tram changing ends.
const ReversePenalty = 60.0

// Platform is where a tram's head stops for a stop in one direction.
type Platform struct {
	Stop StopID
	Link LinkID
	Pos  float64
}

// Platforms returns the stop's platform in each direction of travel.
func (n *Network) Platforms(s *Stop) []Platform {
	centre := n.StopPos(s)
	var out []Platform
	for i := range n.Links {
		l := &n.Links[i]
		if l.Road != s.Road || l.Kind != LaneLink || l.Modes&Tram == 0 || !l.Track || !n.tramOnward(l) {
			continue
		}
		t, _ := project(centre, l.Pts[0], l.Pts[len(l.Pts)-1])
		at := t * l.Length
		if t <= 0 || t >= 1 {
			continue
		}
		pos := math.Min(l.Length-0.5, at+TramLength/2)
		out = append(out, Platform{Stop: s.ID, Link: l.ID, Pos: pos})
	}
	return out
}

// tramOnward reports whether a tram can leave the link (it is not the dead
// piece of track beyond a terminus crossover).
func (n *Network) tramOnward(l *Link) bool {
	for _, nx := range l.Next {
		if n.Links[nx].Modes&Tram != 0 {
			return true
		}
	}
	return false
}

// TramRoute finds track from (from, pos) to the nearest platform of stop.
func (n *Network) TramRoute(from LinkID, pos float64, stop StopID) ([]LinkID, Platform, bool) {
	s := n.Stops[stop]
	if s == nil {
		return nil, Platform{}, false
	}
	ps := n.Platforms(s)
	for _, p := range ps {
		if p.Link == from && p.Pos > pos+0.5 {
			return []LinkID{from}, p, true
		}
	}
	targets := map[LinkID]bool{}
	for _, p := range ps {
		targets[p.Link] = true
	}
	path, ok := n.Route(from, targets, Tram, nil)
	if !ok {
		return nil, Platform{}, false
	}
	for _, p := range ps {
		if p.Link == path[len(path)-1] {
			return path, p, true
		}
	}
	return nil, Platform{}, false
}

// Calls returns the cyclic calling pattern: out through every stop and back,
// skipping stops a direction cannot reach (one-way streets). LineCycle
// works it out for the current track.
func (l *Line) Calls() []StopID {
	if len(l.pattern) > 0 {
		return l.pattern
	}
	return l.outAndBack()
}

func (l *Line) outAndBack() []StopID {
	calls := append([]StopID(nil), l.Stops...)
	for i := len(l.Stops) - 2; i >= 1; i-- {
		calls = append(calls, l.Stops[i])
	}
	return calls
}

func (n *Network) LineCycle(l *Line) (Platform, error) {
	calls := l.outAndBack()
	first := n.Stops[calls[0]]
	if first == nil {
		return Platform{}, fmt.Errorf("line uses a removed stop")
	}
	ends := map[StopID]bool{l.Stops[0]: true, l.Stops[len(l.Stops)-1]: true}
	var lastErr error
	for _, start := range n.Platforms(first) {
		cur, ok := start, true
		pattern := []StopID{calls[0]}
		for i := 1; i <= len(calls) && ok; i++ {
			next := calls[i%len(calls)]
			if n.Stops[next] == nil {
				return Platform{}, fmt.Errorf("line uses a removed stop")
			}
			_, p, found := n.TramRoute(cur.Link, cur.Pos, next)
			if !found {
				// A stop on a one-way street is only served in its direction.
				if !ends[next] && len(n.Platforms(n.Stops[next])) == 1 {
					continue
				}
				ok = false
				lastErr = fmt.Errorf("no track from %s to %s", n.Stops[pattern[len(pattern)-1]].Name, n.Stops[next].Name)
				break
			}
			cur = p
			if i < len(calls) {
				pattern = append(pattern, next)
			}
		}
		if ok {
			l.pattern = pattern
			return start, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s has no platform", first.Name)
	}
	return Platform{}, lastErr
}
