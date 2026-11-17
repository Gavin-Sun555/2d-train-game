package street

import (
	"math"
	"sort"
)

const (
	CentreTrackWidth = 3.6 // m reserved for a single centre track
	JunctionApproach = 6.0 // m from the crossing kerb line to the stop line
	BendRadius       = 2.5 // m lanes stop short of a junction that is only a bend
	// CrossoverLength is the length of a scissors crossover between the two
	// tracks of a road.
	CrossoverLength = 18.0
	// ContraSpeed is the speed limit for a tram running against the normal
	// direction of a track; ContraCost makes routing avoid it.
	ContraSpeed = 25 / 3.6
	ContraCost  = 3.0
	// DedicatedSpeed and SingleTrackSpeed are the track speed ceilings on
	// segregated right-of-way; trams are still governed by their line's
	// operating speed and vehicle top speed.
	DedicatedSpeed   = 100 / 3.6
	SingleTrackSpeed = 70 / 3.6
	RampSpeed        = 50 / 3.6
)

// BlockKind says how a track block is shared.
type BlockKind uint8

const (
	// SingleTrackBlock: one tram at a time (token working).
	SingleTrackBlock BlockKind = iota
)

type Block struct {
	ID    int
	Kind  BlockKind
	Links []LinkID
}

type laneKey struct {
	road    RoadID
	forward bool
	index   int
}

// Lane returns the first piece of a lane, or -1.
func (n *Network) Lane(r RoadID, forward bool, index int) LinkID {
	if ps := n.lanes[laneKey{r, forward, index}]; len(ps) > 0 {
		return ps[0]
	}
	return -1
}

// LanePieces returns all pieces of a lane in travel order.
func (n *Network) LanePieces(r RoadID, forward bool, index int) []LinkID {
	return n.lanes[laneKey{r, forward, index}]
}

func (n *Network) laneEnd(r RoadID, forward bool, index int) LinkID {
	ps := n.lanes[laneKey{r, forward, index}]
	return ps[len(ps)-1]
}

// CentreTrack returns the single-track link of a road in one direction.
func (n *Network) CentreTrack(r RoadID, forward bool) LinkID {
	return n.LanePieces(r, forward, -1)[0]
}

func laneModes(rd Road, index int) (Mode, bool) {
	if index != 0 || rd.Tram == NoTram || rd.Single {
		return Car, false
	}
	if rd.Tram == Dedicated {
		return Tram, true
	}
	return Car | Tram, true
}

// TrackEnd reports whether road r's track ends at junction j (no other
// road there carries track).
func (n *Network) TrackEnd(r RoadID, j JunctionID) bool {
	if n.Roads[r].Tram == NoTram {
		return false
	}
	for _, o := range n.roadsAt[j] {
		if o != r && n.Roads[o].Tram != NoTram {
			return false
		}
	}
	return true
}

func (n *Network) newLink(l Link) LinkID {
	l.ID = LinkID(len(n.Links))
	l.Twin, l.Block, l.Phase = -1, -1, -1
	n.Links = append(n.Links, l)
	return l.ID
}

func (n *Network) twin(a, b LinkID) {
	n.Links[a].Twin, n.Links[b].Twin = b, a
}

// Rebuild regenerates the lane graph after any edit to roads or track.
func (n *Network) Rebuild() {
	n.Links = nil
	n.Blocks = nil
	n.lanes = map[laneKey][]LinkID{}
	n.roadsAt = make([][]RoadID, len(n.Junctions))
	for _, r := range n.Roads {
		n.roadsAt[r.A] = append(n.roadsAt[r.A], r.ID)
		n.roadsAt[r.B] = append(n.roadsAt[r.B], r.ID)
	}
	n.radius = make([]float64, len(n.Junctions))
	for j, rs := range n.roadsAt {
		if len(rs) <= 2 {
			n.radius[j] = BendRadius // a bend or a lane change, not a crossing
			continue
		}
		for _, r := range rs {
			// Lanes stop JunctionApproach short of the crossing carriageway,
			// leaving room for a zebra crossing before the stop line.
			n.radius[j] = math.Max(n.radius[j], n.Roads[r].HalfWidth()+JunctionApproach)
		}
	}
	n.version++
	for _, rd := range n.Roads {
		n.buildRoad(rd)
	}
	n.signals = map[JunctionID]*Signal{}
	n.heads = nil
	n.tramHeads = nil
	for j := range n.Junctions {
		n.buildJunction(JunctionID(j))
	}
	n.buildSingleTrackBlocks()
	for i := range n.Blocks {
		n.signBlock(&n.Blocks[i])
	}
}

// buildRoad creates the lanes of a road (split where a terminus crossover
// will join) and its centre track if it has one.
func (n *Network) buildRoad(rd Road) {
	a, b := n.Ends(rd.ID)
	d := unit(sub(b, a))
	nr := Vec{X: -d.Y, Y: d.X} // right-hand normal (screen y points down)
	ends := func(fwd bool, off float64) (Vec, Vec) {
		if fwd {
			return add(add(a, scale(d, n.radius[rd.A])), scale(nr, off)), add(sub(b, scale(d, n.radius[rd.B])), scale(nr, off))
		}
		return sub(sub(b, scale(d, n.radius[rd.B])), scale(nr, off)), sub(add(a, scale(d, n.radius[rd.A])), scale(nr, off))
	}
	for _, fwd := range []bool{true, false} {
		for i := 0; i < rd.Lanes(fwd); i++ {
			off := rd.LaneOffset(fwd, i)
			p0, p1 := ends(fwd, off)
			l := dist(p0, p1)
			var cuts []float64
			if i == 0 && rd.Tram != NoTram && !rd.Single {
				// Crossovers split both tracks at their switches.
				for _, x := range n.crossoverPos(rd) {
					p := x - n.radius[rd.A]
					if !fwd {
						p = n.Length(rd.ID) - x - n.radius[rd.B]
					}
					cuts = append(cuts, p-CrossoverLength/2, p+CrossoverLength/2)
				}
				sort.Float64s(cuts)
			}
			modes, track := laneModes(rd, i)
			limit := rd.Class.SpeedLimit()
			if track && modes == Tram {
				limit = DedicatedSpeed
			}
			if n.IsRamp(rd.ID) {
				limit = math.Min(limit, RampSpeed)
			}
			bounds := append(append([]float64{0}, cuts...), l)
			var pieces []LinkID
			for k := 0; k+1 < len(bounds); k++ {
				id := n.newLink(Link{Kind: LaneLink, Modes: modes, Road: rd.ID, Forward: fwd, Index: i,
					Track: track, SpeedLimit: limit})
				n.Links[id].setPts([]Vec{p0.Lerp(p1, bounds[k]/l), p0.Lerp(p1, bounds[k+1]/l)})
				if len(pieces) > 0 {
					prev := pieces[len(pieces)-1]
					n.Links[prev].Next = append(n.Links[prev].Next, id)
				}
				pieces = append(pieces, id)
			}
			n.lanes[laneKey{rd.ID, fwd, i}] = pieces
			if track {
				n.buildContra(rd, fwd, i, pieces)
			}
		}
	}
	if rd.Tram != NoTram && !rd.Single && !rd.OneWay() {
		for _, x := range n.crossoverPos(rd) {
			n.buildCrossover(rd, x)
		}
	}
	if rd.Tram != NoTram && rd.Single {
		singleLimit := SingleTrackSpeed
		if n.IsRamp(rd.ID) {
			singleLimit = math.Min(singleLimit, RampSpeed)
		}
		var ids [2]LinkID
		for k, fwd := range []bool{true, false} {
			p0, p1 := ends(fwd, 0)
			ids[k] = n.newLink(Link{Kind: LaneLink, Modes: Tram, Road: rd.ID, Forward: fwd, Index: -1,
				Track: true, Centre: true, SpeedLimit: singleLimit})
			n.Links[ids[k]].setPts([]Vec{p0, p1})
			n.lanes[laneKey{rd.ID, fwd, -1}] = []LinkID{ids[k]}
		}
		n.twin(ids[0], ids[1])
		// At a track end the tram stops and changes ends onto the twin.
		if n.TrackEnd(rd.ID, rd.B) {
			n.Links[ids[0]].Next = append(n.Links[ids[0]].Next, ids[1])
		}
		if n.TrackEnd(rd.ID, rd.A) {
			n.Links[ids[1]].Next = append(n.Links[ids[1]].Next, ids[0])
		}
	}
}

// buildContra adds, for each piece of a double track, the same track
// travelled against its normal direction (its twin). A tram may change
// ends at the end of any piece: the twins are linked both ways.
func (n *Network) buildContra(rd Road, fwd bool, index int, pieces []LinkID) {
	contra := make([]LinkID, len(pieces))
	for k, id := range pieces {
		p := n.Links[id]
		c := n.newLink(Link{Kind: LaneLink, Modes: Tram, Road: rd.ID, Forward: !fwd, Index: index,
			Track: true, Contra: true, SpeedLimit: math.Min(p.SpeedLimit, ContraSpeed)})
		n.Links[c].setPts([]Vec{p.Pts[len(p.Pts)-1], p.Pts[0]})
		n.twin(id, c)
		contra[k] = c
		if k > 0 {
			n.Links[c].Next = append(n.Links[c].Next, contra[k-1])
		}
	}
	for k, id := range pieces {
		n.Links[id].Next = append(n.Links[id].Next, contra[k])
		n.Links[contra[k]].Next = append(n.Links[contra[k]].Next, id)
	}
}

// crossoverPos lists the road's crossovers as distances from A, in order.
func (n *Network) crossoverPos(rd Road) []float64 {
	l := n.Length(rd.ID)
	var out []float64
	for _, t := range rd.Crossovers {
		out = append(out, t*l)
	}
	sort.Float64s(out)
	return out
}

// buildCrossover adds a scissors crossover centred x metres from A: two
// diagonals, each usable both ways, joining a track to the other track
// travelled in the same direction (against its normal direction).
func (n *Network) buildCrossover(rd Road, x float64) {
	lo, hi := x-CrossoverLength/2, x+CrossoverLength/2
	// piece finds the track piece of a direction that starts or ends at
	// road position y (distance from A).
	piece := func(fwd, atEnd bool, y float64) LinkID {
		for _, id := range n.LanePieces(rd.ID, fwd, 0) {
			l := &n.Links[id]
			pt := l.Pts[0]
			if atEnd {
				pt = l.Pts[len(l.Pts)-1]
			}
			if math.Abs(projectAlong(n, rd, pt)-y) < 0.5 {
				return id
			}
		}
		return -1
	}
	a1, a3 := piece(true, true, lo), piece(true, false, hi)   // forward track
	b1, b3 := piece(false, true, hi), piece(false, false, lo) // backward track
	if a1 < 0 || a3 < 0 || b1 < 0 || b3 < 0 {
		return
	}
	diag := func(from, to LinkID) LinkID {
		f, t := &n.Links[from], &n.Links[to]
		id := n.newLink(Link{Kind: CrossoverLink, Modes: Tram, Road: rd.ID, SpeedLimit: 15 / 3.6})
		p0, p3 := f.Pts[len(f.Pts)-1], t.Pts[0]
		n.Links[id].setPts(bezier(p0, p3, unit(sub(p0, f.Pts[len(f.Pts)-2])), unit(sub(t.Pts[1], p3)), 0.4))
		n.Links[id].Next = []LinkID{to}
		n.Links[from].Next = append(n.Links[from].Next, id)
		return id
	}
	tw := func(id LinkID) LinkID { return n.Links[id].Twin }
	m1 := diag(a1, tw(b1)) // forward track → backward track, running A→B
	m3 := diag(b1, tw(a1)) // the same diagonal, running B→A
	m2 := diag(tw(b3), a3) // backward track → forward track, running A→B
	m4 := diag(tw(a3), b3) // the same diagonal, running B→A
	n.twin(m1, m3)
	n.twin(m2, m4)
	all := []LinkID{m1, m2, m3, m4}
	for _, x := range all {
		for _, y := range all {
			if x != y {
				n.Links[x].Conflicts = append(n.Links[x].Conflicts, y)
			}
		}
	}
}

// projectAlong is the distance from A of p's projection on the road.
func projectAlong(n *Network, rd Road, p Vec) float64 {
	a, b := n.Ends(rd.ID)
	t, _ := project(p, a, b)
	return t * dist(a, b)
}

// buildSingleTrackBlocks groups connected single track (centre links and
// the junction connectors joining them) into one-tram-at-a-time blocks.
func (n *Network) buildSingleTrackBlocks() {
	parent := map[LinkID]LinkID{}
	var find func(LinkID) LinkID
	find = func(x LinkID) LinkID {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b LinkID) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[max(ra, rb)] = min(ra, rb)
		}
	}
	single := func(id LinkID) bool { return n.Links[id].Centre }
	for i := range n.Links {
		l := &n.Links[i]
		if l.Centre || (l.Kind == ConnectorLink && single(l.From) && single(l.To)) {
			parent[l.ID] = l.ID
		}
	}
	for id := range parent {
		l := &n.Links[id]
		if l.Twin >= 0 {
			union(id, l.Twin)
		}
		if l.Kind == ConnectorLink {
			union(id, l.From)
			union(id, l.To)
		}
	}
	groups := map[LinkID][]LinkID{}
	for i := range n.Links {
		if _, ok := parent[LinkID(i)]; ok {
			r := find(LinkID(i))
			groups[r] = append(groups[r], LinkID(i))
		}
	}
	for i := range n.Links {
		if ids, ok := groups[LinkID(i)]; ok {
			b := Block{ID: len(n.Blocks), Kind: SingleTrackBlock, Links: ids}
			for _, id := range ids {
				n.Links[id].Block = b.ID
			}
			n.Blocks = append(n.Blocks, b)
		}
	}
}

// signBlock gives every link in a block a travel-direction sign: following a
// link keeps the sign, its twin has the opposite one.
func (n *Network) signBlock(b *Block) {
	if len(b.Links) == 0 {
		return
	}
	seen := map[LinkID]bool{}
	queue := []LinkID{b.Links[0]}
	n.Links[b.Links[0]].BlockSign = 1
	seen[b.Links[0]] = true
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		l := &n.Links[id]
		visit := func(o LinkID, sign int8) {
			if o < 0 || n.Links[o].Block != b.ID || seen[o] {
				return
			}
			seen[o] = true
			n.Links[o].BlockSign = sign
			queue = append(queue, o)
		}
		visit(l.Twin, -l.BlockSign)
		for _, nx := range l.Next {
			if nx != l.Twin {
				visit(nx, l.BlockSign)
			}
		}
	}
}

// approach describes one road's lanes entering (or leaving) a junction.
type approach struct {
	road  RoadID
	dir   Vec // direction of travel
	lanes []LinkID
	tram  LinkID // track entering / leaving here (-1 if none)
}

func (n *Network) approaches(j JunctionID) (in, out []approach) {
	for _, r := range n.roadsAt[j] {
		rd := n.Roads[r]
		a, b := n.Ends(r)
		d := unit(sub(b, a))
		inFwd := rd.B == j // forward lanes run A→B, so they arrive at B
		ai, ao := approach{road: r, tram: -1}, approach{road: r, tram: -1}
		if inFwd {
			ai.dir, ao.dir = d, scale(d, -1)
		} else {
			ai.dir, ao.dir = scale(d, -1), d
		}
		for i := 0; i < rd.Lanes(inFwd); i++ {
			ai.lanes = append(ai.lanes, n.laneEnd(r, inFwd, i))
		}
		for i := 0; i < rd.Lanes(!inFwd); i++ {
			ao.lanes = append(ao.lanes, n.Lane(r, !inFwd, i))
		}
		if rd.Tram != NoTram && !n.TrackEnd(r, j) {
			switch {
			case rd.Single:
				ai.tram, ao.tram = n.CentreTrack(r, inFwd), n.CentreTrack(r, !inFwd)
			default: // double track, or one track with a one-way road's traffic
				if rd.Lanes(inFwd) > 0 {
					ai.tram = n.laneEnd(r, inFwd, 0)
				}
				if rd.Lanes(!inFwd) > 0 {
					ao.tram = n.Lane(r, !inFwd, 0)
				}
			}
		}
		in, out = append(in, ai), append(out, ao)
	}
	return in, out
}

func (n *Network) modeLanes(ls []LinkID, m Mode) []LinkID {
	var out []LinkID
	for _, id := range ls {
		if n.Links[id].Modes&m != 0 {
			out = append(out, id)
		}
	}
	return out
}

func (n *Network) buildJunction(j JunctionID) {
	in, out := n.approaches(j)
	signalised := false
	if len(n.roadsAt[j]) >= 3 {
		for _, r := range n.roadsAt[j] {
			if n.Roads[r].Class >= Avenue || n.Roads[r].Tram != NoTram {
				signalised = true
			}
		}
	}
	if signalised {
		n.signals[j] = newSignal(j)
	}

	type key struct {
		from, to LinkID
		phase    int
	}
	made := map[key]LinkID{}
	connect := func(from, to LinkID, turn Turn, m Mode, phase int) LinkID {
		if id, ok := made[key{from, to, phase}]; ok {
			n.Links[id].Modes |= m
			return id
		}
		id := n.newLink(Link{Kind: ConnectorLink, Modes: m, Junction: j, From: from, To: to, Turn: turn,
			SpeedLimit: math.Min(n.Links[from].SpeedLimit, n.Links[to].SpeedLimit)})
		c := &n.Links[id]
		c.Phase = phase
		if turn != Straight {
			c.SpeedLimit = math.Min(c.SpeedLimit, 20/3.6)
		}
		f, t := &n.Links[from], &n.Links[to]
		c.setPts(bezier(f.Pts[len(f.Pts)-1], t.Pts[0], unit(sub(f.Pts[len(f.Pts)-1], f.Pts[len(f.Pts)-2])), unit(sub(t.Pts[1], t.Pts[0])), 0.45))
		made[key{from, to, phase}] = id
		n.Links[from].Next = append(n.Links[from].Next, id)
		c.Next = []LinkID{to}
		return id
	}

	// Phase 0 serves the approaches along the junction's main road (and
	// those opposite them), phase 1 the crossing ones. When a standalone
	// tramway or portal ramp meets car streets here, trams entering or
	// leaving the tramway/portal get a protected tram phase so street
	// traffic stops at red signals while trams cross or transition levels.
	ref := Vec{X: 1}
	best := -1
	hasCars, hasTramway := false, false
	for _, ai := range in {
		rd := n.Roads[ai.road]
		if rd.Tramway {
			hasTramway = true
			continue
		}
		hasCars = true
		score := int(rd.Class)*10 + rd.Lanes(true) + rd.Lanes(false)
		if score > best {
			best, ref = score, ai.dir
		}
	}
	if !hasCars {
		for _, ai := range in {
			rd := n.Roads[ai.road]
			score := int(rd.Class)*10 + rd.Lanes(true) + rd.Lanes(false)
			if score > best {
				best, ref = score, ai.dir
			}
		}
	}
	phaseOf := func(dir Vec) int {
		if !signalised {
			return -1
		}
		if math.Abs(dot(dir, ref)) > math.Cos(45*math.Pi/180) {
			return 0
		}
		return 1
	}
	protectedTramPhase := -1
	if signalised && hasCars && hasTramway {
		hasCrossCar := false
		for _, ai := range in {
			if !n.Roads[ai.road].Tramway && len(ai.lanes) > 0 && phaseOf(ai.dir) == 1 {
				hasCrossCar = true
				break
			}
		}
		if hasCrossCar {
			protectedTramPhase = 2
			n.signals[j].Phases = 3
		} else {
			protectedTramPhase = 1
		}
	}

	jLevel := n.Junctions[j].Level
	for _, ai := range in {
		if len(ai.lanes) == 0 && ai.tram < 0 {
			continue // one-way road leaving the junction, with no track
		}
		rd := n.Roads[ai.road]
		carPhase := phaseOf(ai.dir)
		tramPhase := carPhase
		if protectedTramPhase >= 0 {
			if rd.Tramway {
				tramPhase = protectedTramPhase
			} else if ai.tram >= 0 {
				// If a street tram here can enter a tramway/portal at this
				// junction, run its tram signal on the protected tram phase.
				for _, ao := range out {
					if ao.road != ai.road && ao.tram >= 0 && n.Roads[ao.road].Tramway && dot(ai.dir, ao.dir) >= -0.85 {
						tramPhase = protectedTramPhase
						break
					}
				}
			}
		}
		if signalised {
			right := Vec{X: -ai.dir.Y, Y: ai.dir.X}
			if !rd.Tramway && len(ai.lanes) > 0 {
				outer := &n.Links[ai.lanes[len(ai.lanes)-1]]
				n.heads = append(n.heads, SignalHead{Junction: j, Phase: carPhase, Dir: ai.dir,
					Pos:   add(outer.Pts[len(outer.Pts)-1], scale(right, LaneWidth/2+1.2)),
					Level: jLevel})
			}
			if ai.tram >= 0 {
				// The tram signal stands on the track's left, towards the
				// road centre, where a driver looks for it.
				track := &n.Links[ai.tram]
				off := LaneWidth/2 + 0.9
				if track.Centre {
					off = rd.CentreGap()/2 + 0.9
				}
				n.tramHeads = append(n.tramHeads, TramSignalHead{Junction: j, Phase: tramPhase, Dir: ai.dir, Track: ai.tram,
					Pos:   sub(track.Pts[len(track.Pts)-1], scale(right, off)),
					Level: jLevel})
			}
		}
		ci := n.modeLanes(ai.lanes, Car)
		var leftOut, rightOut []LinkID // car lanes of the left/right exits
		for _, ao := range out {
			if ao.road == ai.road || dot(ai.dir, ao.dir) < -0.85 {
				continue // no U-turns
			}
			turn := Straight
			if dot(ai.dir, ao.dir) < math.Cos(30*math.Pi/180) {
				turn = Left
				if cross(ai.dir, ao.dir) > 0 {
					turn = Right
				}
			}
			co := n.modeLanes(ao.lanes, Car)
			if turn == Left && len(co) > 0 {
				leftOut = co
			}
			if turn == Right && len(co) > 0 {
				rightOut = co
			}
			if len(ci) > 0 && len(co) > 0 {
				switch turn {
				case Straight:
					for k := 0; k < min(len(ci), len(co)); k++ {
						connect(ci[len(ci)-1-k], co[len(co)-1-k], Straight, Car, carPhase)
					}
				case Right:
					connect(ci[len(ci)-1], co[len(co)-1], Right, Car, carPhase)
				case Left:
					connect(ci[0], co[0], Left, Car, carPhase)
				}
			}
			if ai.tram >= 0 && ao.tram >= 0 {
				connect(ai.tram, ao.tram, turn, Tram, tramPhase)
			}
		}
		// Lanes with no way out under the usual rules (e.g. the middle lane
		// of a boulevard at a T, or both lanes at a corner) turn towards
		// the side they are on, or the only side available.
		for k, lane := range ci {
			if hasCarConnector(n, lane) {
				continue
			}
			inner := k < (len(ci)+1)/2
			switch {
			case len(leftOut) > 0 && (inner || len(rightOut) == 0):
				connect(lane, leftOut[min(k, len(leftOut)-1)], Left, Car, carPhase)
			case len(rightOut) > 0:
				fromOuter := len(ci) - 1 - k
				connect(lane, rightOut[max(len(rightOut)-1-fromOuter, 0)], Right, Car, carPhase)
			}
		}
	}

	// Connectors between two single-track roads are the same track both
	// ways: twin them.
	for k, id := range made {
		f, t := &n.Links[k.from], &n.Links[k.to]
		if f.Centre && t.Centre && f.Twin >= 0 && t.Twin >= 0 && n.Links[id].Twin < 0 {
			for k2, back := range made {
				if k2.from == t.Twin && k2.to == f.Twin && n.Links[back].Twin < 0 {
					n.twin(id, back)
					break
				}
			}
		}
	}

	// Tram movements onto or off double track can also be run backwards
	// (between two single tracks they are already twinned).
	var tramMoves []LinkID
	for _, id := range made {
		tramMoves = append(tramMoves, id)
	}
	sortLinks(tramMoves)
	for _, id := range tramMoves {
		c := n.Links[id]
		f, t := n.Links[c.From], n.Links[c.To]
		if c.Modes&Tram == 0 || c.Twin >= 0 || f.Twin < 0 || t.Twin < 0 {
			continue
		}
		back := n.newLink(Link{Kind: ConnectorLink, Modes: Tram, Junction: j, From: t.Twin, To: f.Twin,
			Turn: c.Turn, Contra: true, SpeedLimit: math.Min(c.SpeedLimit, ContraSpeed)})
		pts := make([]Vec, len(c.Pts))
		for i, p := range c.Pts {
			pts[len(pts)-1-i] = p
		}
		n.Links[back].setPts(pts)
		n.Links[back].Phase = c.Phase
		n.Links[back].Next = []LinkID{f.Twin}
		n.Links[t.Twin].Next = append(n.Links[t.Twin].Next, back)
		n.twin(id, back)
		made[key{t.Twin, f.Twin, c.Phase}] = back
	}

	// Conflicts between connectors of this junction: paths that cross, or
	// that share a piece of lane at either end (whichever way they run it).
	unitOf := func(id LinkID) LinkID {
		if tw := n.Links[id].Twin; tw >= 0 && tw < id {
			return tw
		}
		return id
	}
	var cs []LinkID
	for _, id := range made {
		cs = append(cs, id)
	}
	sortLinks(cs)
	for x := 0; x < len(cs); x++ {
		for y := x + 1; y < len(cs); y++ {
			a, b := &n.Links[cs[x]], &n.Links[cs[y]]
			if a.From == b.From {
				continue // same queue: vehicles pass one after another
			}
			af, at, bf, bt := unitOf(a.From), unitOf(a.To), unitOf(b.From), unitOf(b.To)
			if at == bt || af == bf || af == bt || at == bf || a.Twin == b.ID || polylinesCross(a.Pts, b.Pts) {
				a.Conflicts = append(a.Conflicts, b.ID)
				b.Conflicts = append(b.Conflicts, a.ID)
			}
		}
	}
}

func sortLinks(ids []LinkID) {
	for i := 1; i < len(ids); i++ {
		for k := i; k > 0 && ids[k] < ids[k-1]; k-- {
			ids[k], ids[k-1] = ids[k-1], ids[k]
		}
	}
}

// bezier is a cubic curve from p0 (heading d0) to p3 (heading d3).
func bezier(p0, p3, d0, d3 Vec, k float64) []Vec {
	h := k * dist(p0, p3)
	p1, p2 := add(p0, scale(d0, h)), sub(p3, scale(d3, h))
	const steps = 14
	pts := make([]Vec, steps+1)
	for i := 0; i <= steps; i++ {
		u := float64(i) / steps
		v := 1 - u
		pts[i] = Vec{
			X: v*v*v*p0.X + 3*v*v*u*p1.X + 3*v*u*u*p2.X + u*u*u*p3.X,
			Y: v*v*v*p0.Y + 3*v*v*u*p1.Y + 3*v*u*u*p2.Y + u*u*u*p3.Y,
		}
	}
	return pts
}
