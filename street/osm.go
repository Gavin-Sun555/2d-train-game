package street

import (
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
)

const (
	clusterDist = 22.0 // m: crossings closer than this become one junction
	bendDist    = 10.0 // m: bends this close to a junction are absorbed by it
	simplifyTol = 2.5  // m: straightening tolerance for curved roads
	minClear    = 12.0 // m of lane a road must keep between its junctions
)

func vec(p mapdata.Pt) Vec { return Vec{X: p[0], Y: p[1]} }

// FromMap builds the street network from imported real-world roads.
func FromMap(m *mapdata.Map) *Network { return fromMap(m, false) }

// FromMapLegacy builds the network as versions before road levels and ramp
// junctions did (used to read saves made then).
func FromMapLegacy(m *mapdata.Map) *Network { return fromMap(m, true) }

func fromMap(m *mapdata.Map, legacy bool) *Network {
	n := &Network{Stops: map[StopID]*Stop{}, Lines: map[LineID]*Line{}}

	// Nodes shared by several roads, and road ends, are crossings; so are
	// the street nodes where motorway ramps join ("gates").
	use := make([]int, len(m.Nodes))
	cross := make([]bool, len(m.Nodes))
	// levels: the levels of the roads through each node, as bits; nodes
	// only merge into one junction when they share a level, so a bridge
	// never joins the street it passes over.
	levels := make([]uint8, len(m.Nodes))
	for _, r := range m.Roads {
		for _, id := range r.Nodes {
			use[id]++
			if legacy {
				levels[id] = 1
			} else {
				levels[id] |= 1 << (min(max(r.Layer, -2), 2) + 2)
			}
		}
		cross[r.Nodes[0]], cross[r.Nodes[len(r.Nodes)-1]] = true, true
	}
	for i, u := range use {
		cross[i] = cross[i] || u > 1
	}
	gate := map[int]bool{}
	highways := m.Highways
	if legacy {
		highways = nil
	}
	for _, h := range highways {
		for _, id := range h.Nodes {
			if use[id] > 0 {
				cross[id], gate[id] = true, true
			}
		}
	}

	// Split roads at crossings and straighten each piece.
	type seg struct {
		a, b int // node indices
		road int
	}
	var segs []seg
	keep := make([]bool, len(m.Nodes))
	for ri, r := range m.Roads {
		start := 0
		for k := 1; k < len(r.Nodes); k++ {
			if !cross[r.Nodes[k]] && k < len(r.Nodes)-1 {
				continue
			}
			piece := r.Nodes[start : k+1]
			kept := simplify(m, piece)
			for i := 0; i+1 < len(kept); i++ {
				segs = append(segs, seg{kept[i], kept[i+1], ri})
			}
			for _, id := range kept {
				keep[id] = true
			}
			start = k
		}
	}

	// Cluster nearby nodes into junctions.
	parent := make([]int, len(m.Nodes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[max(ra, rb)] = min(ra, rb)
		}
	}
	var kept []int
	for i, k := range keep {
		if k {
			kept = append(kept, i)
		}
	}
	for x := 0; x < len(kept); x++ {
		for y := x + 1; y < len(kept); y++ {
			a, b := kept[x], kept[y]
			d := dist(vec(m.Nodes[a]), vec(m.Nodes[b]))
			if levels[a]&levels[b] == 0 {
				continue
			}
			if (cross[a] && cross[b] && d < clusterDist) || d < bendDist {
				union(a, b)
			}
		}
	}

	// Contract road segments too short to hold lanes between junctions,
	// until none are left.
	for iter := 0; iter < 30; iter++ {
		pos := clusterPositions(m, kept, find)
		deg := map[int]int{}
		for _, s := range segs {
			ra, rb := find(s.a), find(s.b)
			if ra != rb {
				deg[ra]++
				deg[rb]++
			}
		}
		changed := false
		for _, s := range segs {
			ra, rb := find(s.a), find(s.b)
			if ra == rb {
				continue
			}
			hw := roadHalfWidth(m.Roads[s.road])
			need := minClear
			for _, r := range []int{ra, rb} {
				if deg[r] > 2 {
					need += hw + JunctionApproach + 4
				}
			}
			if dist(pos[ra], pos[rb]) < need {
				union(ra, rb)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	pos := clusterPositions(m, kept, find)

	// One road per junction pair: opposite one-way carriageways merge into
	// a two-way road; otherwise keep the widest.
	type key struct{ a, b int }
	type acc struct {
		road     mapdata.Road
		fwd, bck int
	}
	pairs := map[key]*acc{}
	var order []key
	for _, s := range segs {
		ra, rb := find(s.a), find(s.b)
		if ra == rb {
			continue
		}
		r := m.Roads[s.road]
		f, b := r.Fwd, r.Back
		k := key{ra, rb}
		if ra > rb {
			k, f, b = key{rb, ra}, b, f
		}
		p, ok := pairs[k]
		if !ok {
			p = &acc{road: r}
			pairs[k] = p
			order = append(order, k)
		}
		if classRank(r.Class) > classRank(p.road.Class) {
			p.road.Class, p.road.Name = r.Class, r.Name
		}
		if abs(r.Layer) > abs(p.road.Layer) {
			p.road.Layer = r.Layer
		}
		p.fwd, p.bck = max(p.fwd, f), max(p.bck, b)
	}

	n.anchors = map[Vec]bool{}
	for id := range gate {
		n.anchors[pos[find(id)]] = true
	}
	jid := map[int]JunctionID{}
	junction := func(c int) JunctionID {
		if id, ok := jid[c]; ok {
			return id
		}
		id := JunctionID(len(n.Junctions))
		jid[c] = id
		n.Junctions = append(n.Junctions, Junction{ID: id, Pos: pos[c]})
		return id
	}
	for _, k := range order {
		p := pairs[k]
		class := Street
		switch p.road.Class {
		case "primary", "secondary":
			class = Avenue
		}
		f, b := min(p.fwd, 3), min(p.bck, 3)
		if f == 0 && b == 0 {
			continue
		}
		a, bj := junction(k.a), junction(k.b)
		if f == 0 { // one-way the other way round: flip so lanes run A→B
			a, bj, f, b = bj, a, b, 0
		}
		n.Roads = append(n.Roads, Road{ID: RoadID(len(n.Roads)), A: a, B: bj, Class: class,
			Name: p.road.Name, Fwd: f, Back: b, Level: min(max(p.road.Layer, -2), 2)})
	}
	n.pruneDeadEnds()
	n.keepLargestComponent()
	n.Rebuild()
	// One-way roads cut off by the map edge lead nowhere: drop them until
	// none is left. (A two-way road whose lanes in one direction cannot
	// turn anywhere is kept: routing never sends cars into those lanes.)
	for iter := 0; iter < 50; iter++ {
		trapped := map[RoadID]bool{}
		for i := range n.Links {
			l := &n.Links[i]
			if l.Kind == LaneLink && l.Modes&Car != 0 && len(l.Next) == 0 && n.Roads[l.Road].OneWay() &&
				!n.anchors[n.Junctions[n.Roads[l.Road].B].Pos] {
				trapped[l.Road] = true
			}
		}
		if len(trapped) == 0 {
			break
		}
		kept := n.Roads[:0]
		for _, r := range n.Roads {
			if !trapped[r.ID] {
				kept = append(kept, r)
			}
		}
		n.Roads = kept
		n.pruneDeadEnds()
		n.keepLargestComponent()
		n.Rebuild()
	}
	// Where each ramp meets the streets.
	at := map[Vec]JunctionID{}
	for _, j := range n.Junctions {
		at[j.Pos] = j.ID
	}
	n.Gates = map[int]JunctionID{}
	for id := range gate {
		if j, ok := at[pos[find(id)]]; ok && len(n.RoadsAt(j)) > 0 {
			n.Gates[id] = j
		}
	}
	return n
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func classRank(c string) int {
	return map[string]int{"primary": 4, "secondary": 3, "tertiary": 2, "unclassified": 1, "residential": 1}[c]
}

func roadHalfWidth(r mapdata.Road) float64 {
	if r.Back == 0 {
		return float64(r.Fwd) * LaneWidth / 2
	}
	return float64(max(r.Fwd, r.Back)) * LaneWidth
}

func clusterPositions(m *mapdata.Map, kept []int, find func(int) int) map[int]Vec {
	sum := map[int]Vec{}
	cnt := map[int]float64{}
	for _, id := range kept {
		r := find(id)
		p := vec(m.Nodes[id])
		sum[r] = add(sum[r], p)
		cnt[r]++
	}
	pos := map[int]Vec{}
	for r, s := range sum {
		pos[r] = scale(s, 1/cnt[r])
	}
	return pos
}

// simplify keeps the end nodes of a piece and the bends that matter
// (Douglas–Peucker).
func simplify(m *mapdata.Map, piece []int) []int {
	if len(piece) <= 2 {
		return piece
	}
	a, b := vec(m.Nodes[piece[0]]), vec(m.Nodes[piece[len(piece)-1]])
	far, at := 0.0, 0
	for i := 1; i < len(piece)-1; i++ {
		if _, d := project(vec(m.Nodes[piece[i]]), a, b); d > far {
			far, at = d, i
		}
	}
	if far < simplifyTol {
		return []int{piece[0], piece[len(piece)-1]}
	}
	// Copy: piece shares memory with the map's road, which must not change
	// (the network is built from it again for every save that is read).
	left := append([]int(nil), simplify(m, piece[:at+1])...)
	return append(left[:len(left)-1], simplify(m, piece[at:])...)
}

// keepLargestComponent drops road islands not connected to the main network.
func (n *Network) keepLargestComponent() {
	adj := make([][]RoadID, len(n.Junctions))
	for _, r := range n.Roads {
		adj[r.A] = append(adj[r.A], r.ID)
		adj[r.B] = append(adj[r.B], r.ID)
	}
	comp := make([]int, len(n.Junctions))
	for i := range comp {
		comp[i] = -1
	}
	size := map[int]int{}
	for s := range n.Junctions {
		if comp[s] >= 0 || len(adj[s]) == 0 {
			continue
		}
		stack := []JunctionID{JunctionID(s)}
		comp[s] = s
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size[s]++
			for _, r := range adj[j] {
				o := n.other(r, j)
				if comp[o] < 0 {
					comp[o] = s
					stack = append(stack, o)
				}
			}
		}
	}
	bestC, best := -1, 0
	keys := make([]int, 0, len(size))
	for c := range size {
		keys = append(keys, c)
	}
	sort.Ints(keys)
	for _, c := range keys {
		if size[c] > best {
			bestC, best = c, size[c]
		}
	}
	kept := n.Roads[:0]
	for _, r := range n.Roads {
		if comp[r.A] == bestC {
			kept = append(kept, r)
		}
	}
	n.Roads = kept
	n.pruneDeadEnds() // renumbers junctions and roads
}
