package street

import (
	"errors"
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"math"
	"sort"
)

// Road editing: the player may build new streets, demolish streets and
// change their lanes. Motorways are not part of the network and cannot be
// edited; roads with tram track must have it removed first. Every edit is
// logged by position, so a save replays the same edits on the map's own
// network and gets the same roads back.

// RoadEdit is one logged edit. A and B are the road's ends (for "add", the
// points it joins); Fwd and Back its lanes from A to B and back.
type RoadEdit struct {
	Op     string `json:"op"` // "add", "remove", "lanes", "tramway" or "rmtramway"
	A      Vec    `json:"a"`
	B      Vec    `json:"b"`
	Fwd    int    `json:"fwd,omitempty"`
	Back   int    `json:"back,omitempty"`
	LevelA int    `json:"levelA,omitempty"`
	LevelB int    `json:"levelB,omitempty"`
	Single bool   `json:"single,omitempty"`
}

const (
	MaxLanes      = 3    // per direction
	minNewRoad    = 25.0 // m: shortest piece of new road
	junctionSpace = 15.0 // m: new junctions keep this far from others
	replaySnap    = 0.5  // m
)

// Edits is the log of road edits made on the map's network.
func (n *Network) Edits() []RoadEdit { return append([]RoadEdit(nil), n.edits...) }

type snapshot struct {
	junctions []Junction
	roads     []Road
	stops     map[StopID]Stop
	lines     map[LineID]Line
	gates     map[int]JunctionID
	nextID    int
}

func (n *Network) save() snapshot {
	s := snapshot{
		junctions: append([]Junction(nil), n.Junctions...),
		stops:     map[StopID]Stop{},
		lines:     map[LineID]Line{},
		nextID:    n.nextID,
	}
	for _, r := range n.Roads {
		r.Crossovers = append([]float64(nil), r.Crossovers...)
		s.roads = append(s.roads, r)
	}
	for id, st := range n.Stops {
		s.stops[id] = *st
	}
	for id, l := range n.Lines {
		cp := *l
		cp.Stops = append([]StopID(nil), l.Stops...)
		s.lines[id] = cp
	}
	if n.Gates != nil {
		s.gates = map[int]JunctionID{}
		for k, v := range n.Gates {
			s.gates[k] = v
		}
	}
	return s
}

func (n *Network) restore(s snapshot) {
	n.Junctions, n.Roads, n.nextID, n.Gates = s.junctions, s.roads, s.nextID, s.gates
	n.Stops = map[StopID]*Stop{}
	for id, st := range s.stops {
		st := st
		n.Stops[id] = &st
	}
	n.Lines = map[LineID]*Line{}
	for id, l := range s.lines {
		l := l
		n.Lines[id] = &l
	}
	n.Rebuild()
}

// edit runs a change; if it fails or leaves the network unusable (split in
// two, or one-way lanes leading nowhere) everything is put back.
func (n *Network) edit(e *RoadEdit, change func() error) error {
	snap := n.save()
	err := change()
	if err == nil {
		n.compact()
		n.Rebuild()
		err = n.usable()
	}
	if err != nil {
		n.restore(snap)
		return err
	}
	n.edits = append(n.edits, *e)
	return nil
}

// usable reports why the network cannot carry traffic, if it cannot.
func (n *Network) usable() error {
	firstCar := -1
	for i, r := range n.Roads {
		if !r.Tramway {
			firstCar = i
			break
		}
	}
	if firstCar < 0 {
		return errors.New(locale.T("不能拆掉所有道路", "You can't demolish every road"))
	}
	seen := make([]bool, len(n.Junctions))
	stack := []JunctionID{n.Roads[firstCar].A}
	seen[n.Roads[firstCar].A] = true
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, r := range n.roadsAt[j] {
			if n.Roads[r].Tramway {
				continue
			}
			if o := n.other(r, j); !seen[o] {
				seen[o] = true
				stack = append(stack, o)
			}
		}
	}
	for _, r := range n.Roads {
		if !r.Tramway && (!seen[r.A] || !seen[r.B]) {
			return errors.New(locale.T("这样会把路网分成互不相通的两块", "That would split the road network in two"))
		}
	}
	for i := range n.Links {
		l := &n.Links[i]
		if l.Kind == LaneLink && l.Modes&Car != 0 && len(l.Next) == 0 && n.Roads[l.Road].OneWay() &&
			!n.anchors[n.Junctions[n.Roads[l.Road].B].Pos] {
			return fmt.Errorf(locale.T("单行道 %s 会通向死路", "One-way %s would lead nowhere"), n.roadName(l.Road))
		}
	}
	return nil
}

func (n *Network) roadName(r RoadID) string {
	if name := n.Roads[r].Name; name != "" {
		return name
	}
	return locale.T("（无名道路）", "(unnamed road)")
}

// compact drops junctions no road uses and renumbers roads and junctions,
// keeping stops and ramp gates pointing at the right ones.
func (n *Network) compact() {
	used := make([]bool, len(n.Junctions))
	for _, r := range n.Roads {
		used[r.A], used[r.B] = true, true
	}
	remap := make([]JunctionID, len(n.Junctions))
	var js []Junction
	for _, j := range n.Junctions {
		remap[j.ID] = -1
		if used[j.ID] {
			remap[j.ID] = JunctionID(len(js))
			j.ID = remap[j.ID]
			js = append(js, j)
		}
	}
	n.Junctions = js
	roadMap := map[RoadID]RoadID{}
	for i := range n.Roads {
		roadMap[n.Roads[i].ID] = RoadID(i)
		n.Roads[i].ID = RoadID(i)
		n.Roads[i].A, n.Roads[i].B = remap[n.Roads[i].A], remap[n.Roads[i].B]
	}
	for _, st := range n.Stops {
		st.Road = roadMap[st.Road]
	}
	for k, j := range n.Gates {
		if remap[j] < 0 {
			delete(n.Gates, k)
		} else {
			n.Gates[k] = remap[j]
		}
	}
}

func (n *Network) hasTrack(r RoadID) bool {
	if n.Roads[r].Tram != NoTram {
		return true
	}
	for _, st := range n.Stops {
		if st.Road == r {
			return true
		}
	}
	return false
}

// endpoint finds where a new road joins: a ground junction near p, or a point
// on a street (which is split there). It reports false if p is on neither.
func (n *Network) endpoint(p Vec, snap float64) (JunctionID, error) {
	if j, ok := n.NearestJunction(p, snap); ok {
		// Ensure j belongs to at least one car street.
		for _, r := range n.Roads {
			if !r.Tramway && (r.A == j || r.B == j) {
				return j, nil
			}
		}
	}
	r, t, ok := n.NearestStreet(p, snap)
	if !ok {
		return -1, errors.New(locale.T("新道路的两端都要接在现有道路或路口上", "Both ends of a new road must be on an existing road or junction"))
	}
	a, b := n.Ends(r)
	q := a.Lerp(b, t)
	if dist(q, a) < junctionSpace || dist(q, b) < junctionSpace {
		// Close to a junction: join that junction instead.
		if dist(q, a) < dist(q, b) {
			return n.Roads[r].A, nil
		}
		return n.Roads[r].B, nil
	}
	return n.split(r, q)
}

// split cuts road r at q into two roads meeting at a new junction.
func (n *Network) split(r RoadID, q Vec) (JunctionID, error) {
	if n.hasTrack(r) {
		return -1, fmt.Errorf(locale.T("%s 上有电车轨道或车站，不能在中间接新路（先拆除轨道）", "%s has tram track or a stop: a new road can't join it midway (remove the track first)"), n.roadName(r))
	}
	j := JunctionID(len(n.Junctions))
	n.Junctions = append(n.Junctions, Junction{ID: j, Pos: q})
	rd := n.Roads[r]
	tail := rd
	tail.ID, tail.A = RoadID(len(n.Roads)), j
	tail.Crossovers = nil
	n.Roads[r].B = j
	n.Roads = append(n.Roads, tail)
	return j, nil
}

func classFor(fwd, back int, old Class) Class {
	switch m := max(fwd, back); {
	case m <= 1:
		return Street
	case old == Boulevard && m >= 2:
		return Boulevard
	}
	return Avenue
}

func checkLanes(fwd, back int) error {
	if fwd < 0 || back < 0 || fwd > MaxLanes || back > MaxLanes || fwd+back == 0 {
		return fmt.Errorf(locale.T("每个方向 0–%d 条车道，至少一条", "0–%d lanes each way, at least one in all"), MaxLanes)
	}
	return nil
}

// AddRoad builds a street from a to b (each on a junction or a road,
// within snap metres), crossing any streets in between at new junctions.
func (n *Network) AddRoad(a, b Vec, fwd, back int, snap float64) error {
	if err := checkLanes(fwd, back); err != nil {
		return err
	}
	e := &RoadEdit{Op: "add", Fwd: fwd, Back: back}
	return n.edit(e, func() error {
		ja, err := n.endpoint(a, snap)
		if err != nil {
			return err
		}
		jb, err := n.endpoint(b, snap)
		if err != nil {
			return err
		}
		if ja == jb {
			return errors.New(locale.T("起点和终点是同一个路口", "The start and the end are the same junction"))
		}
		pa, pb := n.Junctions[ja].Pos, n.Junctions[jb].Pos
		e.A, e.B = pa, pb // logged where it really joins
		// Streets crossed on the way.
		type hit struct {
			s float64
			r RoadID
			q Vec
		}
		var hits []hit
		for _, rd := range n.Roads {
			if rd.A == ja || rd.A == jb || rd.B == ja || rd.B == jb {
				continue
			}
			la, lb := n.RoadLevels(rd.ID)
			ra, rb := n.Ends(rd.ID)
			if la != lb && (la == 0 || lb == 0) {
				if _, _, ok := segHit(pa, pb, ra, rb); ok {
					return errors.New(locale.T("新道路不能横穿电车高架/地下出入坡道", "A new road can't cut across a tram portal ramp"))
				}
				continue
			}
			if la != 0 || lb != 0 {
				continue // elevated viaduct or underground tunnel: grade-separated
			}
			if s, u, ok := segHit(pa, pb, ra, rb); ok {
				hits = append(hits, hit{s, rd.ID, ra.Lerp(rb, u)})
			}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].s < hits[j].s })
		chain := []JunctionID{ja}
		for _, h := range hits {
			ra, rb := n.Ends(h.r)
			if dist(h.q, ra) < junctionSpace || dist(h.q, rb) < junctionSpace {
				return fmt.Errorf(locale.T("新道路离 %s 的路口太近，请直接连到那个路口", "The new road passes too close to a junction of %s: join that junction instead"), n.roadName(h.r))
			}
			var j JunctionID
			if n.Roads[h.r].Tramway {
				j, err = n.splitPreservingTrack(h.r, h.q)
			} else {
				j, err = n.split(h.r, h.q)
			}
			if err != nil {
				return err
			}
			chain = append(chain, j)
		}
		chain = append(chain, jb)
		for i := 0; i+1 < len(chain); i++ {
			p, q := chain[i], chain[i+1]
			if d := dist(n.Junctions[p].Pos, n.Junctions[q].Pos); d < minNewRoad {
				return fmt.Errorf(locale.T("路段太短（%.0f m），至少 %.0f m", "Too short (%.0f m), at least %.0f m"), d, minNewRoad)
			}
			for _, rd := range n.Roads {
				if (rd.A == p && rd.B == q) || (rd.A == q && rd.B == p) {
					return errors.New(locale.T("这两个路口之间已经有路", "There is already a road between these junctions"))
				}
			}
			f, bk, from, to := fwd, back, p, q
			if f == 0 {
				f, bk, from, to = back, 0, q, p
			}
			n.Roads = append(n.Roads, Road{ID: RoadID(len(n.Roads)), A: from, B: to,
				Class: classFor(fwd, back, Street), Name: locale.T("新建道路", "New road"), Fwd: f, Back: bk})
		}
		return nil
	})
}

// segHit intersects segments p–q and a–b strictly inside both; s and u are
// the positions along each.
func segHit(p, q, a, b Vec) (s, u float64, ok bool) {
	r, d := sub(q, p), sub(b, a)
	den := r.X*d.Y - r.Y*d.X
	if math.Abs(den) < 1e-9 {
		return 0, 0, false
	}
	w := sub(a, p)
	s = (w.X*d.Y - w.Y*d.X) / den
	u = (w.X*r.Y - w.Y*r.X) / den
	return s, u, s > 1e-6 && s < 1-1e-6 && u > 1e-6 && u < 1-1e-6
}

// RemoveRoad demolishes a street, and any street left leading nowhere
// because of it.
func (n *Network) RemoveRoad(r RoadID) error {
	if r < 0 || int(r) >= len(n.Roads) || n.Roads[r].Tramway {
		return errors.New(locale.T("没有这条道路", "No such road"))
	}
	if n.hasTrack(r) {
		return fmt.Errorf(locale.T("%s 上有电车轨道或车站，先拆除轨道", "%s has tram track or a stop: remove the track first"), n.roadName(r))
	}
	a, b := n.Ends(r)
	return n.edit(&RoadEdit{Op: "remove", A: a, B: b}, func() error {
		gone := map[RoadID]bool{r: true}
		for changed := true; changed; {
			changed = false
			deg := make([]int, len(n.Junctions))
			for _, rd := range n.Roads {
				if !gone[rd.ID] && !rd.Tramway {
					deg[rd.A]++
					deg[rd.B]++
				}
			}
			for _, rd := range n.Roads {
				if gone[rd.ID] || rd.Tramway {
					continue
				}
				for _, j := range []JunctionID{rd.A, rd.B} {
					if deg[j] == 1 && !n.anchors[n.Junctions[j].Pos] {
						if n.hasTrack(rd.ID) {
							return fmt.Errorf(locale.T("拆除后 %s 会成为断头的电车线路", "Demolishing would leave tram track on %s at a dead end"), n.roadName(rd.ID))
						}
						gone[rd.ID], changed = true, true
						break
					}
				}
			}
		}
		kept := n.Roads[:0:0]
		for _, rd := range n.Roads {
			if !gone[rd.ID] {
				kept = append(kept, rd)
			}
		}
		n.Roads = kept
		return nil
	})
}

// SetRoadLanes changes a street's lanes: fwd from A to B, back from B to A
// (0 makes it one-way).
func (n *Network) SetRoadLanes(r RoadID, fwd, back int) error {
	if err := checkLanes(fwd, back); err != nil {
		return err
	}
	if r < 0 || int(r) >= len(n.Roads) || n.Roads[r].Tramway {
		return errors.New(locale.T("没有这条道路", "No such road"))
	}
	if n.hasTrack(r) {
		return fmt.Errorf(locale.T("%s 上有电车轨道或车站，先拆除轨道再调整车道", "%s has tram track or a stop: remove the track before changing lanes"), n.roadName(r))
	}
	a, b := n.Ends(r)
	return n.edit(&RoadEdit{Op: "lanes", A: a, B: b, Fwd: fwd, Back: back}, func() error {
		rd := &n.Roads[r]
		rd.Class = classFor(fwd, back, rd.Class)
		rd.Fwd, rd.Back = fwd, back
		if fwd == 0 {
			rd.A, rd.B, rd.Fwd, rd.Back = rd.B, rd.A, back, 0
		}
		return nil
	})
}

// roadBetween finds the car street joining junctions at a and b (either way
// round); flipped reports it runs from b to a.
func (n *Network) roadBetween(a, b Vec) (RoadID, bool, bool) {
	ja, ok1 := n.NearestJunction(a, replaySnap)
	jb, ok2 := n.NearestJunction(b, replaySnap)
	if !ok1 || !ok2 {
		return -1, false, false
	}
	for _, rd := range n.Roads {
		if rd.Tramway {
			continue
		}
		switch {
		case rd.A == ja && rd.B == jb:
			return rd.ID, false, true
		case rd.A == jb && rd.B == ja:
			return rd.ID, true, true
		}
	}
	return -1, false, false
}

// ApplyEdits replays logged edits (from a save) on the map's network.
func (n *Network) ApplyEdits(edits []RoadEdit) error {
	for i, e := range edits {
		var err error
		switch e.Op {
		case "add":
			err = n.AddRoad(e.A, e.B, e.Fwd, e.Back, replaySnap)
		case "remove", "lanes":
			r, flipped, ok := n.roadBetween(e.A, e.B)
			if !ok {
				err = errors.New(locale.T("找不到道路", "road not found"))
			} else if e.Op == "remove" {
				err = n.RemoveRoad(r)
			} else {
				f, b := e.Fwd, e.Back
				if flipped {
					f, b = b, f
				}
				err = n.SetRoadLanes(r, f, b)
			}
		case "tramway":
			err = n.addTramway(e.A, e.B, e.LevelA, e.LevelB, e.Single, replaySnap, true)
		case "rmtramway":
			r, ok := n.tramwayBetween(e.A, e.B, e.LevelA, e.LevelB)
			if !ok {
				err = errors.New(locale.T("找不到独立轨道段", "tramway segment not found"))
			} else {
				err = n.RemoveTramway(r)
			}
		default:
			err = fmt.Errorf(locale.T("未知的修改 %q", "unknown edit %q"), e.Op)
		}
		if err != nil {
			return fmt.Errorf(locale.T("第 %d 处道路修改无法重做：%v", "Road edit %d couldn't be replayed: %v"), i+1, err)
		}
	}
	return nil
}
