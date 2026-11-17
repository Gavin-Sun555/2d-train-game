package street

import (
	"container/heap"
	"errors"
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"math"
	"sort"
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
	Contra  bool // double track travelled against its normal direction

	// Twin is the same piece of track travelled the other way (-1 if none).
	// A vehicle on one is physically on the other too.
	Twin LinkID
	// Block is the single-track block this link belongs to (-1 if none);
	// BlockSign is the travel direction within the block.
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
	Level    int
}

func (n *Network) SignalHeads() []SignalHead { return n.heads }

// TramSignalHead is a signal for trams only, beside the track at the stop
// line of a signalled crossing. It shows the same phase as the road
// signal for its approach (or the protected tram phase at a portal/tramway
// crossing); cars sharing the lane are not held by it. Track is the tram
// track it guards, so a tram (or, later, a player driving one) can look up
// its signal.
type TramSignalHead struct {
	Junction JunctionID
	Phase    int
	Pos      Vec
	Dir      Vec // direction of the trams it controls
	Track    LinkID
	Level    int
}

// TramSignalHeads lists the tram signals.
func (n *Network) TramSignalHeads() []TramSignalHead { return n.tramHeads }

// TramSignalFor is the tram signal guarding the end of a track link, if
// there is one.
func (n *Network) TramSignalFor(track LinkID) (TramSignalHead, bool) {
	for _, h := range n.tramHeads {
		if h.Track == track {
			return h, true
		}
	}
	return TramSignalHead{}, false
}

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
	path, _, ok := n.route(from, targets, m, cost, 0)
	return path, ok
}

// route finds the cheapest path; a vehicle changes ends only on links at
// least minTurn long (so its whole body is on the link).
func (n *Network) route(from LinkID, targets map[LinkID]bool, m Mode, cost func(*Link) float64, minTurn float64) ([]LinkID, float64, bool) {
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
			return path, it.d, true
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
			if l.Contra {
				c *= ContraCost
			}
			if n.Links[it.id].Twin == nx {
				if n.Links[it.id].Length < minTurn || n.Links[it.id].Modes&Car != 0 {
					// The tram would not fit, or cars queue behind it: on
					// a shared lane trams change ends only after crossing
					// over to the other track.
					continue
				}
				c += ReversePenalty // stop and change ends
			}
			if d := it.d + c; !hasKey(dist, nx) || d < dist[nx] {
				dist[nx], prev[nx] = d, it.id
				heap.Push(pq, linkItem{nx, d})
			}
		}
	}
	return nil, 0, false
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

// Tram sets are built from SectionLength modules; platforms are built to
// hold a number of them.
const (
	SectionLength   = 6.0
	MinSections     = 1
	MaxSections     = 8
	DefaultSections = 3 // a new line's trams
	DefaultPlatform = 4 // a new stop's platforms
	MaxTrams        = 12
)

// SetLength is the length of a tram of the given number of sections.
func SetLength(sections int) float64 { return SectionLength * float64(sections) }

// PlatformLength is the length of a platform holding the given sections.
func PlatformLength(sections int) float64 { return SetLength(sections) + 2 }

func clampSections(k int) int { return min(max(k, MinSections), MaxSections) }

// Fleet is the number of trams on the line (at least one).
func (l *Line) Fleet() int { return min(max(l.Trams, 1), MaxTrams) }

// Cars is the number of sections the line's trams are planned with; the
// platforms and turnback tracks may hold fewer (see LineCapacity).
func (l *Line) Cars() int {
	if l.Sections == 0 {
		return DefaultSections
	}
	return clampSections(l.Sections)
}

// Passengers one tram section carries at most: the player sets it per
// line in steps of SectionLoadStep.
const (
	MinSectionLoad     = 10
	MaxSectionLoad     = 200
	DefaultSectionLoad = 40
	SectionLoadStep    = 5
)

// SectionLoad is the most passengers one of the line's sections carries.
func (l *Line) SectionLoad() int {
	if l.PerSection == 0 {
		return DefaultSectionLoad
	}
	return min(max(l.PerSection, MinSectionLoad), MaxSectionLoad)
}

// Boarding and alighting: seconds each passenger takes at one door, set
// per line in tenths of a second, and passengers one section serves at
// once.
const (
	MinServiceTenths    = 5   // 0.5 s
	MaxServiceTenths    = 100 // 10 s
	ServiceStepTenths   = 5
	DefaultBoardTenths  = 20 // 2 s
	DefaultAlightTenths = 10 // 1 s
	MinDoors, MaxDoors  = 1, 4
	DefaultDoors        = 2
)

func tenths(v, def int) float64 {
	if v == 0 {
		v = def
	}
	return float64(min(max(v, MinServiceTenths), MaxServiceTenths)) / 10
}

// BoardSeconds is how long one passenger takes to board through a door.
func (l *Line) BoardSeconds() float64 { return tenths(l.BoardTenths, DefaultBoardTenths) }

// AlightSeconds is how long one passenger takes to get off through a door.
func (l *Line) AlightSeconds() float64 { return tenths(l.AlightTenths, DefaultAlightTenths) }

// Least stop times, set per line in seconds.
const (
	DefaultDwell, MinDwell, MaxDwell, DwellStep             = 20, 5, 300, 5
	DefaultEndDwell, MinEndDwell, MaxEndDwell, EndDwellStep = 60, 10, 600, 10
)

// Dwell is the least time a tram stands at a stop on the way, in seconds.
func (l *Line) Dwell() int {
	if l.DwellSecs == 0 {
		return DefaultDwell
	}
	return min(max(l.DwellSecs, MinDwell), MaxDwell)
}

// EndDwell is the least time a tram stands where it turns back: the
// driver walks to the other cab.
func (l *Line) EndDwell() int {
	if l.EndDwellSecs == 0 {
		return DefaultEndDwell
	}
	return min(max(l.EndDwellSecs, MinEndDwell), MaxEndDwell)
}

// StopDwell is the least stop time at call i of the calling pattern.
func (l *Line) StopDwell(i int) float64 {
	if l.TurnsAt(i) {
		return float64(l.EndDwell())
	}
	return float64(l.Dwell())
}

// DoorsPerSection is how many passengers one section serves at once.
func (l *Line) DoorsPerSection() int {
	if l.Doors == 0 {
		return DefaultDoors
	}
	return min(max(l.Doors, MinDoors), MaxDoors)
}

// Capacity is the number of tram sections the stop's platforms hold.
func (s *Stop) Capacity() int {
	if s.Sections == 0 {
		return DefaultPlatform
	}
	return clampSections(s.Sections)
}

// LineCapacity is the longest tram (in sections) the line can run: no
// longer than its shortest platform, and short enough to change ends
// where the line turns back. limit names what sets it. Zero means not
// even a one-section tram can run the line.
func (n *Network) LineCapacity(l *Line) (sections int, limit string) {
	if l.capVer == n.version && l.capStops == len(l.Stops) && l.capVer != 0 {
		return l.capVal, l.capWhy
	}
	sections = MaxSections
	for _, id := range l.Stops {
		if s := n.Stops[id]; s != nil && s.Capacity() < sections {
			sections, limit = s.Capacity(), s.Name+locale.T(" 站台", " platform")
		}
	}
	for ; sections >= MinSections; sections-- {
		if _, _, err := n.lineCycle(l, SetLength(sections)); err == nil {
			break
		}
		limit = locale.T("折返处轨道长度", "the track where it turns back")
	}
	l.capVer, l.capStops, l.capVal, l.capWhy = n.version, len(l.Stops), max(sections, 0), limit
	return l.capVal, limit
}

// TramSections is the number of sections the line's trams actually run with.
func (n *Network) TramSections(l *Line) int {
	c, _ := n.LineCapacity(l)
	return max(min(l.Cars(), c), MinSections)
}

// TramLength is the length of the line's trams.
func (n *Network) TramLength(l *Line) float64 { return SetLength(n.TramSections(l)) }

// ReversePenalty is the routing cost (s) of a tram changing ends.
const ReversePenalty = 60.0

// Platform is where a tram's head stops for a stop in one direction: Pos
// is the far end of a platform Len long. Shorter trams stop centred on it.
type Platform struct {
	Stop StopID
	Link LinkID
	Pos  float64
	Len  float64
}

// Head is where a tram of the given length stops its head: centred on
// the platform.
func (p Platform) Head(length float64) float64 {
	return p.Pos - math.Max(p.Len-length, 0)/2
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
		plen := PlatformLength(s.Capacity())
		pos := math.Min(l.Length-0.5, at+plen/2)
		out = append(out, Platform{Stop: s.ID, Link: l.ID, Pos: pos, Len: plen})
	}
	// Platforms on the normal running direction first.
	sort.SliceStable(out, func(i, j int) bool { return !n.Links[out[i].Link].Contra && n.Links[out[j].Link].Contra })
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

// TramRoute finds track from (from, pos) to the nearest platform of stop
// for a tram of the given length whose head is at pos. With then, it only
// picks a platform from which the tram can go on to that stop (it does not
// stop facing a dead end it cannot turn back from).
//
// A tram standing at a platform may also change ends right there, during
// its stop, and leave the other way: the route then starts [from, twin].
// TurnsInPlace tells such a route from one that runs to the end of the
// link before reversing.
func (n *Network) TramRoute(from LinkID, pos float64, stop StopID, length float64, then ...StopID) ([]LinkID, Platform, bool) {
	return n.tramRoute(from, pos, stop, length, true, then...)
}

// TramRouteOnward is TramRoute without changing ends where the tram
// stands: it runs on (to a crossover, if it must turn back). A tram whose
// change of ends at the platform cannot be reserved (trams queued on the
// track it would run back over) leaves this way instead.
func (n *Network) TramRouteOnward(from LinkID, pos float64, stop StopID, length float64, then ...StopID) ([]LinkID, Platform, bool) {
	return n.tramRoute(from, pos, stop, length, false, then...)
}

func (n *Network) tramRoute(from LinkID, pos float64, stop StopID, length float64, turn bool, then ...StopID) ([]LinkID, Platform, bool) {
	s := n.Stops[stop]
	if s == nil {
		return nil, Platform{}, false
	}
	onward := func(p Platform) bool {
		if len(then) == 0 || then[0] == stop {
			return true
		}
		_, _, ok := n.TramRoute(p.Link, p.Head(length), then[0], length)
		return ok
	}
	ps := n.Platforms(s)
	path, p, cost, ok := n.tramRouteFrom(from, pos, ps, length, onward)
	if turn && n.TurnsInPlace(from, pos, length) {
		tw := n.Links[from].Twin
		tpos := n.Links[tw].Length - (pos - length)
		if bp, bpl, bc, bok := n.tramRouteFrom(tw, tpos, ps, length, onward); bok && (!ok || bc+TurnInPlacePenalty < cost) {
			return append([]LinkID{from}, bp...), bpl, true
		}
	}
	return path, p, ok
}

// TurnInPlacePenalty is the routing cost (s) of changing ends at a
// platform: it happens while passengers board, so it costs little.
const TurnInPlacePenalty = 5.0

// TurnsInPlace reports whether a tram with its head at pos on the link can
// change ends where it stands: its whole body is on the link and the link
// is tram-only track (on a lane shared with cars, trams change ends only
// after crossing over).
func (n *Network) TurnsInPlace(from LinkID, pos, length float64) bool {
	l := &n.Links[from]
	return l.Kind == LaneLink && l.Track && l.Twin >= 0 && l.Modes&Car == 0 &&
		pos-length >= 0 && pos <= l.Length+0.01
}

// remaining is the routing cost of the rest of a link from pos.
func (n *Network) remaining(id LinkID, pos float64) float64 {
	l := &n.Links[id]
	c := math.Max(0, l.Length-pos) / l.SpeedLimit
	if l.Contra {
		c *= ContraCost
	}
	return c
}

// tramRouteFrom routes to the best platform of ps, with its cost.
func (n *Network) tramRouteFrom(from LinkID, pos float64, ps []Platform, length float64, onward func(Platform) bool) ([]LinkID, Platform, float64, bool) {
	for _, p := range ps {
		if p.Link == from && p.Pos > pos+0.5 && onward(p) {
			c := (p.Pos - pos) / n.Links[from].SpeedLimit
			if n.Links[from].Contra {
				c *= ContraCost
			}
			return []LinkID{from}, p, c, true
		}
	}
	targets := map[LinkID]bool{}
	for _, p := range ps {
		targets[p.Link] = true
	}
	for len(targets) > 0 {
		path, cost, ok := n.route(from, targets, Tram, nil, length+2)
		if !ok {
			return nil, Platform{}, 0, false
		}
		end := path[len(path)-1]
		for _, p := range ps {
			if p.Link == end && onward(p) {
				return path, p, cost + n.remaining(from, pos), true
			}
		}
		delete(targets, end)
	}
	return nil, Platform{}, 0, false
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

// TurnsAt reports whether trams turn back at call i of the calling
// pattern: the stops either side of it are the same stop, as at the ends
// of an out-and-back line. A line drawn there and back again (A B C B A)
// turns at C as well as at A, so the far end is not always the last stop
// listed.
func (l *Line) TurnsAt(i int) bool {
	calls := l.Calls()
	n := len(calls)
	if n <= 2 {
		return true
	}
	i = ((i % n) + n) % n
	return calls[(i+n-1)%n] == calls[(i+1)%n]
}

// Ends are the stops where the line's trams turn back, in calling order
// from the first stop.
func (l *Line) Ends() []StopID {
	var out []StopID
	for i, c := range l.Calls() {
		if l.TurnsAt(i) {
			out = append(out, c)
		}
	}
	return out
}

func (l *Line) outAndBack() []StopID {
	calls := append([]StopID(nil), l.Stops...)
	for i := len(l.Stops) - 2; i >= 1; i-- {
		calls = append(calls, l.Stops[i])
	}
	return calls
}

// LineCycle checks the line can be run with its trams and works out its
// calling pattern; it returns the platform where service starts.
func (n *Network) LineCycle(l *Line) (Platform, error) {
	if c, _ := n.LineCapacity(l); c == 0 {
		_, _, err := n.lineCycle(l, SetLength(MinSections))
		return Platform{}, err
	}
	start, pattern, err := n.lineCycle(l, n.TramLength(l))
	if err == nil {
		l.pattern = pattern
	}
	return start, err
}

func (n *Network) lineCycle(l *Line, length float64) (Platform, []StopID, error) {
	calls := l.outAndBack()
	first := n.Stops[calls[0]]
	if first == nil {
		return Platform{}, nil, errors.New(locale.T("线路用到了已删除的车站", "The line uses a deleted stop"))
	}
	ends := map[StopID]bool{l.Stops[0]: true, l.Stops[len(l.Stops)-1]: true}
	var lastErr error
	for _, start := range n.Platforms(first) {
		if n.Links[start.Link].Contra {
			continue // service starts in the normal direction
		}
		cur, ok := start, true
		pattern := []StopID{calls[0]}
		for i := 1; i <= len(calls) && ok; i++ {
			next := calls[i%len(calls)]
			if n.Stops[next] == nil {
				return Platform{}, nil, errors.New(locale.T("线路用到了已删除的车站", "The line uses a deleted stop"))
			}
			_, p, found := n.TramRoute(cur.Link, cur.Head(length), next, length, calls[(i+1)%len(calls)])
			if !found {
				// A stop on a one-way street is only served in its direction.
				if !ends[next] && n.Roads[n.Stops[next].Road].OneWay() {
					continue
				}
				ok = false
				lastErr = fmt.Errorf(locale.T("电车无法从 %s 开到 %s", "Trams can't run from %s to %s"), n.Stops[pattern[len(pattern)-1]].Name, n.Stops[next].Name)
				break
			}
			cur = p
			if i < len(calls) {
				pattern = append(pattern, next)
			}
		}
		if ok {
			return start, pattern, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf(locale.T("%s 没有站台", "%s has no platform"), first.Name)
	}
	return Platform{}, nil, lastErr
}

// CyclePath is the track a tram of the line runs over one whole cycle,
// from the starting platform round to it again, with the platforms it
// stops at.
func (n *Network) CyclePath(l *Line) ([]LinkID, []Platform, error) {
	cur, err := n.LineCycle(l)
	if err != nil {
		return nil, nil, err
	}
	length := n.TramLength(l)
	calls := l.Calls()
	path := []LinkID{cur.Link}
	plats := []Platform{cur}
	for i := 1; i <= len(calls); i++ {
		seg, p, ok := n.TramRoute(cur.Link, cur.Head(length), calls[i%len(calls)], length, calls[(i+1)%len(calls)])
		if !ok {
			return nil, nil, fmt.Errorf("no route")
		}
		path = append(path, seg[1:]...)
		plats = append(plats, p)
		cur = p
	}
	return path, plats, nil
}

// CycleLength is the distance a tram of the line runs over one whole
// cycle, head to head from the starting platform round to it again,
// changes of ends included.
func (n *Network) CycleLength(l *Line) float64 {
	cur, err := n.LineCycle(l)
	if err != nil {
		return 0
	}
	length := n.TramLength(l)
	calls := l.Calls()
	d := 0.0
	for i := 1; i <= len(calls); i++ {
		seg, p, ok := n.TramRoute(cur.Link, cur.Head(length), calls[i%len(calls)], length, calls[(i+1)%len(calls)])
		if !ok {
			return 0
		}
		pos := cur.Head(length)
		for k, id := range seg {
			lk := &n.Links[id]
			if k == len(seg)-1 {
				d += math.Max(p.Head(length)-pos, 0)
				break
			}
			if seg[k+1] == lk.Twin {
				end := lk.Length // runs to the end, then changes ends
				if k == 0 && n.TurnsInPlace(id, pos, length) {
					end = pos // changes ends at the platform
				}
				d += math.Max(end-pos, 0)
				pos = n.Links[lk.Twin].Length - math.Max(0, end-length)
				continue
			}
			d += math.Max(lk.Length-pos, 0)
			pos = 0
		}
		cur = p
	}
	return d
}
