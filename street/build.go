package street

import "math"

const (
	CentreTrackWidth = 3.6 // m reserved for a single centre track
	JunctionApproach = 6.0 // m from the crossing kerb line to the stop line
	BendRadius       = 2.5 // m lanes stop short of a junction that is only a bend
	// TurnbackStub is the track beyond a terminus crossover where a tram
	// stands to change ends; CrossoverLength is the length of the switches.
	TurnbackStub    = TramLength + 6
	CrossoverLength = 18.0
)

// BlockKind says how a track block is shared.
type BlockKind uint8

const (
	// SingleTrackBlock: one tram at a time (token working).
	SingleTrackBlock BlockKind = iota
	// TurnbackBlock: the stub beyond a terminus crossover. A tram crossing
	// over against the normal direction needs it empty; vehicles running
	// the normal way only need no tram coming the other way.
	TurnbackBlock
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
	for _, rd := range n.Roads {
		n.buildRoad(rd)
	}
	for _, rd := range n.Roads {
		if rd.Tram != NoTram && !rd.Single && !rd.OneWay() {
			for _, j := range []JunctionID{rd.A, rd.B} {
				if n.TrackEnd(rd.ID, j) {
					n.buildTurnback(rd, j)
				}
			}
		}
	}
	n.signals = map[JunctionID]*Signal{}
	n.heads = nil
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
		startJ, endJ := rd.A, rd.B
		if !fwd {
			startJ, endJ = rd.B, rd.A
		}
		for i := 0; i < rd.Lanes(fwd); i++ {
			off := rd.LaneOffset(fwd, i)
			p0, p1 := ends(fwd, off)
			l := dist(p0, p1)
			var cuts []float64
			if i == 0 && rd.Tram != NoTram && !rd.Single && !rd.OneWay() && n.turnbackFits(rd) {
				if n.TrackEnd(rd.ID, startJ) {
					cuts = append(cuts, TurnbackStub) // the stub this lane departs from
				}
				if n.TrackEnd(rd.ID, endJ) {
					cuts = append(cuts, l-TurnbackStub-CrossoverLength) // crossover leaves here
				}
			}
			modes, track := laneModes(rd, i)
			bounds := append(append([]float64{0}, cuts...), l)
			var pieces []LinkID
			for k := 0; k+1 < len(bounds); k++ {
				id := n.newLink(Link{Kind: LaneLink, Modes: modes, Road: rd.ID, Forward: fwd, Index: i,
					Track: track, SpeedLimit: rd.Class.SpeedLimit()})
				n.Links[id].setPts([]Vec{p0.Lerp(p1, bounds[k]/l), p0.Lerp(p1, bounds[k+1]/l)})
				if len(pieces) > 0 {
					prev := pieces[len(pieces)-1]
					n.Links[prev].Next = append(n.Links[prev].Next, id)
				}
				pieces = append(pieces, id)
			}
			n.lanes[laneKey{rd.ID, fwd, i}] = pieces
		}
	}
	if rd.Tram != NoTram && rd.Single {
		var ids [2]LinkID
		for k, fwd := range []bool{true, false} {
			p0, p1 := ends(fwd, 0)
			ids[k] = n.newLink(Link{Kind: LaneLink, Modes: Tram, Road: rd.ID, Forward: fwd, Index: -1,
				Track: true, Centre: true, SpeedLimit: math.Min(rd.Class.SpeedLimit(), 40/3.6)})
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

func (n *Network) turnbackFits(rd Road) bool {
	l := n.Length(rd.ID) - n.radius[rd.A] - n.radius[rd.B]
	need := TurnbackStub + CrossoverLength + 4
	if n.TrackEnd(rd.ID, rd.A) && n.TrackEnd(rd.ID, rd.B) {
		need += TurnbackStub + CrossoverLength
	}
	return l >= need
}

// fixTermini turns the last road of a double track that is too short for a
// terminus crossover into a single-track stub: trams switch onto it at the
// junction and change ends at its far end.
func (n *Network) fixTermini() {
	changed := false
	for i := range n.Roads {
		rd := &n.Roads[i]
		if rd.Tram == NoTram || rd.Single || rd.OneWay() {
			continue
		}
		if (n.TrackEnd(rd.ID, rd.A) || n.TrackEnd(rd.ID, rd.B)) && !n.turnbackFits(*rd) {
			rd.Tram, rd.Single = Dedicated, true
			changed = true
		}
	}
	if changed {
		n.Rebuild()
	}
}

// buildTurnback adds the terminus crossover at junction j where road rd's
// double track ends: a tram arriving on the inbound track crosses over to
// the outbound track, runs against its normal direction into the stub,
// changes ends there and departs normally.
func (n *Network) buildTurnback(rd Road, j JunctionID) {
	if !n.turnbackFits(rd) {
		return
	}
	inFwd := rd.B == j
	in := n.LanePieces(rd.ID, inFwd, 0)
	out := n.LanePieces(rd.ID, !inFwd, 0)
	from := in[len(in)-2] // the piece ending where the crossover leaves
	stub := out[0]        // outbound piece from the junction to the crossover
	contra := n.newLink(Link{Kind: LaneLink, Modes: Tram, Road: rd.ID, Forward: inFwd, Index: 0,
		Track: true, SpeedLimit: 15 / 3.6})
	sp := n.Links[stub].Pts
	n.Links[contra].setPts([]Vec{sp[len(sp)-1], sp[0]})
	n.twin(stub, contra)
	n.Links[contra].Next = []LinkID{stub} // change ends at the stub end

	x := n.newLink(Link{Kind: CrossoverLink, Modes: Tram, Road: rd.ID, SpeedLimit: 15 / 3.6})
	n.Links[x].setPts(bezier(n.Links[from].Pts[1], n.Links[contra].Pts[0], unit(sub(n.Links[from].Pts[1], n.Links[from].Pts[0])), unit(sub(n.Links[contra].Pts[1], n.Links[contra].Pts[0])), 0.4))
	n.Links[x].Next = []LinkID{contra}
	n.Links[from].Next = append(n.Links[from].Next, x)

	b := Block{ID: len(n.Blocks), Kind: TurnbackBlock, Links: []LinkID{stub, contra, x}}
	for _, id := range b.Links {
		n.Links[id].Block = b.ID
	}
	n.Blocks = append(n.Blocks, b)
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
	// The turnback stub's normal direction is the positive one.
	if b.Kind == TurnbackBlock && n.Links[b.Links[0]].BlockSign < 0 {
		for _, id := range b.Links {
			n.Links[id].BlockSign = -n.Links[id].BlockSign
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

	type key struct{ from, to LinkID }
	made := map[key]LinkID{}
	connect := func(from, to LinkID, turn Turn, m Mode, phase int) LinkID {
		if id, ok := made[key{from, to}]; ok {
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
		made[key{from, to}] = id
		n.Links[from].Next = append(n.Links[from].Next, id)
		c.Next = []LinkID{to}
		return id
	}

	// Phase 0 serves the approaches along the junction's main road (and
	// those opposite them), phase 1 the crossing ones.
	ref := Vec{X: 1}
	best := -1
	for _, ai := range in {
		rd := n.Roads[ai.road]
		score := int(rd.Class)*10 + rd.Lanes(true) + rd.Lanes(false)
		if score > best {
			best, ref = score, ai.dir
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
	for _, ai := range in {
		if len(ai.lanes) == 0 {
			continue // one-way road leaving the junction
		}
		phase := phaseOf(ai.dir)
		if signalised {
			outer := &n.Links[ai.lanes[len(ai.lanes)-1]]
			right := Vec{X: -ai.dir.Y, Y: ai.dir.X}
			n.heads = append(n.heads, SignalHead{Junction: j, Phase: phase, Dir: ai.dir,
				Pos: add(outer.Pts[len(outer.Pts)-1], scale(right, LaneWidth/2+1.2))})
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
						connect(ci[len(ci)-1-k], co[len(co)-1-k], Straight, Car, phase)
					}
				case Right:
					connect(ci[len(ci)-1], co[len(co)-1], Right, Car, phase)
				case Left:
					connect(ci[0], co[0], Left, Car, phase)
				}
			}
			if ai.tram >= 0 && ao.tram >= 0 {
				connect(ai.tram, ao.tram, turn, Tram, phase)
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
				connect(lane, leftOut[min(k, len(leftOut)-1)], Left, Car, phase)
			case len(rightOut) > 0:
				fromOuter := len(ci) - 1 - k
				connect(lane, rightOut[max(len(rightOut)-1-fromOuter, 0)], Right, Car, phase)
			}
		}
	}

	// Connectors between two single-track roads are the same track both
	// ways: twin them.
	for k, id := range made {
		f, t := &n.Links[k.from], &n.Links[k.to]
		if f.Centre && t.Centre && f.Twin >= 0 && t.Twin >= 0 {
			if back, ok := made[key{t.Twin, f.Twin}]; ok && n.Links[id].Twin < 0 {
				n.twin(id, back)
			}
		}
	}

	// Conflicts between connectors of this junction.
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
			if a.To == b.To || a.Twin == b.ID || polylinesCross(a.Pts, b.Pts) {
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
