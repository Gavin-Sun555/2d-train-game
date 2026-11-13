// Package street is the at-grade street network: roads of different widths,
// junctions, traffic signals, and the tram tracks, stops and lines the
// player builds on those roads. It derives a lane graph that the traffic
// simulation drives cars and trams on. Engine-free and deterministic.
package street

import (
	"fmt"
	"math"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

type Vec = sim.Vec2

const LaneWidth = 3.5 // m

// Class is the width/importance of a road.
type Class int

const (
	Street    Class = iota // 1 lane each way
	Avenue                 // 2 lanes each way
	Boulevard              // 3 lanes each way with a central median
)

func (c Class) String() string { return [...]string{"street", "avenue", "boulevard"}[c] }

func (c Class) LanesPerDir() int { return int(c) + 1 }

func (c Class) Median() float64 {
	if c == Boulevard {
		return 3
	}
	return 0
}

func (c Class) SpeedLimit() float64 { return [...]float64{40, 50, 60}[c] / 3.6 }

// TramMode says how a road carries tram track. Track always occupies the
// innermost lane of each direction.
type TramMode int

const (
	NoTram    TramMode = iota
	Shared             // track in the lane, cars drive on it too (mixed traffic)
	Dedicated          // the lane becomes tram-only (own right of way)
)

func (m TramMode) String() string { return [...]string{"none", "shared", "dedicated"}[m] }

type JunctionID int
type RoadID int
type StopID int
type LineID int

type Junction struct {
	ID  JunctionID
	Pos Vec
}

type Road struct {
	ID    RoadID
	A, B  JunctionID
	Class Class
	Name  string
	// Fwd and Back are lane counts in the A→B and B→A directions; both zero
	// means the class default each way. Back == 0 < Fwd is a one-way road.
	Fwd, Back int
	Tram      TramMode
	// Single is a single track along the road centre, used in both
	// directions (always its own right of way). Otherwise a road with Tram
	// set has double track in the innermost lanes.
	Single bool
}

// Lanes is the number of lanes in one direction (fwd = A→B).
func (r Road) Lanes(fwd bool) int {
	if r.Fwd == 0 && r.Back == 0 {
		return r.Class.LanesPerDir()
	}
	if fwd {
		return r.Fwd
	}
	return r.Back
}

func (r Road) OneWay() bool { return r.Lanes(false) == 0 }

// CentreGap is the width between the two innermost lanes: the median, or
// the reserved strip of a single centre track.
func (r Road) CentreGap() float64 {
	if r.OneWay() {
		return 0
	}
	if r.Tram != NoTram && r.Single {
		return math.Max(r.Class.Median(), CentreTrackWidth)
	}
	return r.Class.Median()
}

// LaneOffset is how far lane i of a direction lies to the right of the
// centre line, seen in that direction of travel.
func (r Road) LaneOffset(fwd bool, i int) float64 {
	if r.OneWay() {
		return -float64(r.Lanes(true))*LaneWidth/2 + (float64(i)+0.5)*LaneWidth
	}
	return r.CentreGap()/2 + (float64(i)+0.5)*LaneWidth
}

// Sides returns the carriageway width left and right of the centre line,
// looking from A to B.
func (r Road) Sides() (left, right float64) {
	if r.OneWay() {
		w := float64(r.Lanes(true)) * LaneWidth / 2
		return w, w
	}
	g := r.CentreGap() / 2
	return g + float64(r.Lanes(false))*LaneWidth, g + float64(r.Lanes(true))*LaneWidth
}

// HalfWidth is the larger distance from the centre line to a kerb.
func (r Road) HalfWidth() float64 {
	l, rt := r.Sides()
	return math.Max(l, rt)
}

// Stop is a tram stop on a road, with a platform for each direction.
type Stop struct {
	ID   StopID
	Road RoadID
	T    float64 // position along the road from A, 0..1
	Name string
}

// Line is a tram service calling at stops in order, then back again.
type Line struct {
	ID    LineID
	Stops []StopID
	// Trams is the number of trams running on the line and Sections the
	// modules in each tram set; zero means the default (see Fleet, Cars).
	Trams, Sections int

	pattern []StopID // calling pattern for the current track (see Calls)
}

// Network is the editable street plan plus its derived lane graph.
type Network struct {
	Junctions []Junction
	Roads     []Road
	Stops     map[StopID]*Stop
	Lines     map[LineID]*Line
	nextID    int

	// Derived by Rebuild; never edit directly.
	Links   []Link
	signals map[JunctionID]*Signal
	heads   []SignalHead
	roadsAt [][]RoadID
	lanes   map[laneKey][]LinkID // pieces of each lane, in travel order
	Blocks  []Block
	radius  []float64
}

// Generate lays the road network over a city map: one road per street
// corridor between neighbouring crossings, dropping pieces cut by water and
// dead ends. The boulevards are the middle arterials.
func Generate(m *city.Map) *Network {
	n := &Network{Stops: map[StopID]*Stop{}, Lines: map[LineID]*Line{}}
	xs, ys := city.StreetLines(m.W), city.StreetLines(m.H)
	class := func(l city.StreetLine, mid int) Class {
		switch {
		case l.Arterial && l.Tile == mid:
			return Boulevard
		case l.Arterial:
			return Avenue
		}
		return Street
	}
	jid := make([][]JunctionID, len(xs))
	for i, x := range xs {
		jid[i] = make([]JunctionID, len(ys))
		for j, y := range ys {
			jid[i][j] = JunctionID(len(n.Junctions))
			n.Junctions = append(n.Junctions, Junction{ID: jid[i][j], Pos: Vec{X: x.Centre, Y: y.Centre}})
		}
	}
	passable := func(a, b Vec) bool {
		l := math.Hypot(b.X-a.X, b.Y-a.Y)
		for d := 0.0; d <= l; d += 2 {
			k := m.TileAt(a.Lerp(b, d/l)).Kind
			if k != city.Road && k != city.Bridge {
				return false
			}
		}
		return true
	}
	add := func(a, b JunctionID, c Class) {
		if passable(n.Junctions[a].Pos, n.Junctions[b].Pos) {
			n.Roads = append(n.Roads, Road{ID: RoadID(len(n.Roads)), A: a, B: b, Class: c})
		}
	}
	for j, y := range ys {
		for i := 0; i+1 < len(xs); i++ {
			add(jid[i][j], jid[i+1][j], class(y, 24))
		}
	}
	for i, x := range xs {
		for j := 0; j+1 < len(ys); j++ {
			add(jid[i][j], jid[i][j+1], class(x, 48))
		}
	}
	n.pruneDeadEnds()
	n.Rebuild()
	return n
}

// pruneDeadEnds removes roads ending in a junction with no other road (cars
// cannot turn around), then drops unused junctions and renumbers.
func (n *Network) pruneDeadEnds() {
	for {
		deg := make([]int, len(n.Junctions))
		for _, r := range n.Roads {
			deg[r.A]++
			deg[r.B]++
		}
		kept := n.Roads[:0]
		for _, r := range n.Roads {
			if deg[r.A] > 1 && deg[r.B] > 1 {
				kept = append(kept, r)
			}
		}
		if len(kept) == len(n.Roads) {
			break
		}
		n.Roads = kept
	}
	used := map[JunctionID]bool{}
	for _, r := range n.Roads {
		used[r.A], used[r.B] = true, true
	}
	remap := map[JunctionID]JunctionID{}
	var js []Junction
	for _, j := range n.Junctions {
		if used[j.ID] {
			remap[j.ID] = JunctionID(len(js))
			j.ID = remap[j.ID]
			js = append(js, j)
		}
	}
	n.Junctions = js
	for i := range n.Roads {
		n.Roads[i].ID = RoadID(i)
		n.Roads[i].A, n.Roads[i].B = remap[n.Roads[i].A], remap[n.Roads[i].B]
	}
}

func (n *Network) id() int { n.nextID++; return n.nextID }

// RoadsAt lists the roads meeting at a junction.
func (n *Network) RoadsAt(j JunctionID) []RoadID { return n.roadsAt[j] }

// Radius is how far from the junction centre the lanes stop.
func (n *Network) Radius(j JunctionID) float64 { return n.radius[j] }

func (n *Network) Ends(r RoadID) (Vec, Vec) {
	rd := n.Roads[r]
	return n.Junctions[rd.A].Pos, n.Junctions[rd.B].Pos
}

func (n *Network) Length(r RoadID) float64 {
	a, b := n.Ends(r)
	return dist(a, b)
}

// NearestRoad returns the road whose centre line is closest to q (within
// maxDist) and the position along it.
func (n *Network) NearestRoad(q Vec, maxDist float64) (RoadID, float64, bool) {
	best, bestT, bestD := RoadID(-1), 0.0, maxDist
	for _, r := range n.Roads {
		a, b := n.Ends(r.ID)
		if t, d := project(q, a, b); d <= bestD {
			best, bestT, bestD = r.ID, t, d
		}
	}
	return best, bestT, best >= 0
}

// NearestJunction returns the junction closest to q within maxDist.
func (n *Network) NearestJunction(q Vec, maxDist float64) (JunctionID, bool) {
	best, bestD := JunctionID(-1), maxDist
	for _, j := range n.Junctions {
		if d := dist(j.Pos, q); d <= bestD {
			best, bestD = j.ID, d
		}
	}
	return best, best >= 0
}

// RoadPath returns the shortest chain of roads between two junctions.
func (n *Network) RoadPath(from, to JunctionID) ([]RoadID, bool) {
	return n.RoadPathCost(from, to, func(r RoadID) float64 { return n.Length(r) })
}

// RoadPathCost is RoadPath with a custom cost per road (negative = closed).
func (n *Network) RoadPathCost(from, to JunctionID, cost func(RoadID) float64) ([]RoadID, bool) {
	if from == to {
		return nil, false
	}
	dist := make([]float64, len(n.Junctions))
	prev := make([]RoadID, len(n.Junctions))
	done := make([]bool, len(n.Junctions))
	for i := range dist {
		dist[i], prev[i] = math.Inf(1), -1
	}
	dist[from] = 0
	for {
		u := JunctionID(-1)
		for i := range dist {
			if !done[i] && !math.IsInf(dist[i], 1) && (u < 0 || dist[i] < dist[u]) {
				u = JunctionID(i)
			}
		}
		if u < 0 {
			return nil, false
		}
		if u == to {
			break
		}
		done[u] = true
		for _, r := range n.roadsAt[u] {
			v := n.other(r, u)
			c := cost(r)
			if c < 0 {
				continue
			}
			if d := dist[u] + c; d < dist[v] {
				dist[v], prev[v] = d, r
			}
		}
	}
	var out []RoadID
	for j := to; j != from; j = n.other(prev[j], j) {
		out = append(out, prev[j])
	}
	for i, k := 0, len(out)-1; i < k; i, k = i+1, k-1 {
		out[i], out[k] = out[k], out[i]
	}
	return out, true
}

func (n *Network) other(r RoadID, j JunctionID) JunctionID {
	if n.Roads[r].A == j {
		return n.Roads[r].B
	}
	return n.Roads[r].A
}

// --- tram construction ------------------------------------------------------

// SetTram lays (or with NoTram removes) tram track on roads: double track in
// the inner lanes with the given right of way, or a single centre track
// (always dedicated). Removing track also removes the stops on those roads
// and any line left with < 2 stops.
func (n *Network) SetTram(roads []RoadID, mode TramMode, single bool) {
	defer n.fixTermini()
	for _, r := range roads {
		// A one-way road carries one track, with the traffic.
		single := single && !n.Roads[r].OneWay()
		if single && mode != NoTram {
			mode = Dedicated
		}
		n.Roads[r].Tram, n.Roads[r].Single = mode, single && mode != NoTram
		if mode == NoTram {
			for _, id := range n.StopIDs() {
				if n.Stops[id].Road == r {
					n.deleteStop(id)
				}
			}
		}
	}
	n.Rebuild()
}

// MinPlatformClearance keeps a stop far enough from the stop lines for a
// whole tram to stand at the platform without blocking a junction.
const MinPlatformClearance = TramLength/2 + 1

var stopNames = []string{
	"Central", "Market St", "City Hall", "Riverside", "Park Gate", "Museum",
	"Harbour Rd", "University", "Old Town", "Library", "Hospital", "Stadium",
	"Union Sq", "Mill Lane", "Bridge St", "Garden Ave", "Cathedral", "Exchange",
	"North End", "South Quay", "Theatre", "Arcade", "Elm Row", "Foundry",
}

// AddStop places a stop on a road that has tram track.
func (n *Network) AddStop(r RoadID, t float64) (StopID, error) {
	rd := n.Roads[r]
	if rd.Tram == NoTram {
		return 0, fmt.Errorf("stops need tram track on the road")
	}
	l := n.Length(r)
	d := t * l
	if d < MinPlatformClearance+n.radius[rd.A] || l-d < MinPlatformClearance+n.radius[rd.B] {
		return 0, fmt.Errorf("too close to a junction")
	}
	for _, s := range n.Stops {
		if s.Road == r && math.Abs(s.T-t)*l < 50 {
			return 0, fmt.Errorf("too close to %s", s.Name)
		}
	}
	id := StopID(n.id())
	name := stopNames[len(n.Stops)%len(stopNames)]
	if k := len(n.Stops) / len(stopNames); k > 0 {
		name = fmt.Sprintf("%s %d", name, k+1)
	}
	n.Stops[id] = &Stop{ID: id, Road: r, T: t, Name: name}
	if len(n.Platforms(n.Stops[id])) == 0 {
		delete(n.Stops, id)
		return 0, fmt.Errorf("no room for a platform here (terminus turnback track)")
	}
	return id, nil
}

func (n *Network) deleteStop(id StopID) {
	delete(n.Stops, id)
	for _, lid := range n.LineIDs() {
		l := n.Lines[lid]
		kept := l.Stops[:0]
		for _, s := range l.Stops {
			if s != id {
				kept = append(kept, s)
			}
		}
		l.Stops = kept
		if len(l.Stops) < 2 {
			delete(n.Lines, lid)
		}
	}
}

func (n *Network) StopIDs() []StopID {
	ids := make([]StopID, 0, len(n.Stops))
	for id := range n.Stops {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (n *Network) LineIDs() []LineID {
	ids := make([]LineID, 0, len(n.Lines))
	for id := range n.Lines {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// StopPos is the stop's position on the road centre line.
func (n *Network) StopPos(s *Stop) Vec {
	a, b := n.Ends(s.Road)
	return a.Lerp(b, s.T)
}

// StopNear returns the stop closest to q within maxDist.
func (n *Network) StopNear(q Vec, maxDist float64) (*Stop, bool) {
	var best *Stop
	bestD := maxDist
	for _, id := range n.StopIDs() {
		if d := dist(n.StopPos(n.Stops[id]), q); d <= bestD {
			best, bestD = n.Stops[id], d
		}
	}
	return best, best != nil
}

// AddLine creates a tram line through the stops (then back), checking that a
// tram can actually travel the whole loop.
func (n *Network) AddLine(stops []StopID) (LineID, error) {
	if len(stops) < 2 {
		return 0, fmt.Errorf("a line needs at least two stops")
	}
	l := &Line{ID: LineID(n.id()), Stops: append([]StopID(nil), stops...)}
	if _, err := n.LineCycle(l); err != nil {
		return 0, err
	}
	n.Lines[l.ID] = l
	return l.ID, nil
}

func (n *Network) DeleteLine(id LineID) { delete(n.Lines, id) }

// --- geometry ---------------------------------------------------------------

func dist(a, b Vec) float64      { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func sub(a, b Vec) Vec           { return Vec{X: a.X - b.X, Y: a.Y - b.Y} }
func add(a, b Vec) Vec           { return Vec{X: a.X + b.X, Y: a.Y + b.Y} }
func scale(a Vec, k float64) Vec { return Vec{X: a.X * k, Y: a.Y * k} }
func dot(a, b Vec) float64       { return a.X*b.X + a.Y*b.Y }
func cross(a, b Vec) float64     { return a.X*b.Y - a.Y*b.X }

func unit(a Vec) Vec {
	l := math.Hypot(a.X, a.Y)
	if l == 0 {
		return Vec{}
	}
	return Vec{X: a.X / l, Y: a.Y / l}
}

func project(q, a, b Vec) (float64, float64) {
	ab := sub(b, a)
	l2 := dot(ab, ab)
	if l2 == 0 {
		return 0, dist(q, a)
	}
	t := math.Max(0, math.Min(1, dot(sub(q, a), ab)/l2))
	return t, dist(q, a.Lerp(b, t))
}

// BoxHalf is the half-size of the junction's carriageway square (the widest
// road meeting there).
func (n *Network) BoxHalf(j JunctionID) float64 {
	if len(n.roadsAt[j]) <= 2 {
		return 0
	}
	return n.radius[j] - JunctionApproach
}
