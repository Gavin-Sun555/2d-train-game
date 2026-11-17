package game

import (
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"math"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/hajimehoshi/ebiten/v2"
)

// Realistic palette, after aerial imagery of a North American downtown.
var (
	colUrban     = rgb(190, 187, 180) // paved ground between buildings
	colSidewalk  = rgb(212, 209, 202)
	colKerb      = rgb(232, 230, 224)
	colRoad      = rgb(84, 86, 90)
	colRoadJoint = rgb(80, 82, 86)
	colLine      = rgba(238, 238, 232, 235)
	colCentreY   = rgb(226, 184, 62)
	colRealWater = rgb(58, 98, 122)
	colRealTree  = rgb(64, 96, 58)
	colTreeLight = rgb(98, 132, 80)
	colBShadow   = rgba(0, 0, 0, 70)
)

var areaColours = map[string]colorT{
	"park":    rgb(112, 142, 92),
	"grass":   rgb(122, 150, 100),
	"wood":    rgb(78, 108, 68),
	"pitch":   rgb(104, 148, 90),
	"plaza":   rgb(200, 196, 188),
	"parking": rgb(132, 132, 132),
	"water":   colRealWater,
}

var (
	lowRoofs  = []colorT{rgb(188, 180, 168), rgb(168, 170, 170), rgb(208, 208, 204), rgb(142, 138, 132), rgb(196, 186, 170), rgb(122, 126, 130), rgb(176, 160, 146)}
	tallRoofs = []colorT{rgb(112, 120, 128), rgb(90, 98, 108), rgb(150, 156, 160), rgb(178, 182, 184), rgb(72, 84, 98), rgb(126, 136, 130), rgb(196, 198, 198)}
)

type building struct {
	m      mesh
	height float64
	roof   colorT
	units  []street.Vec // rooftop plant
}

type area struct {
	m    mesh
	kind string
}

// scene is the pre-processed real-world map: triangulated footprints and
// ground areas, ready to draw every frame.
type scene struct {
	src       *mapdata.Map
	areas     []area
	buildings []building
	trees     []street.Vec // where trees stand (see trees.go)
	mapTrees  []street.Vec // trees as mapped
	highways  []highway
	landmarks []landmark
}

func newScene(m *mapdata.Map) *scene {
	s := &scene{src: m}
	toVec := func(ps []mapdata.Pt) []street.Vec {
		out := make([]street.Vec, len(ps))
		for i, p := range ps {
			out[i] = street.Vec{X: p[0], Y: p[1]}
		}
		return out
	}
	for _, a := range m.Areas {
		if _, ok := areaColours[a.Kind]; ok {
			s.areas = append(s.areas, area{newMesh(toVec(a.Pts)), a.Kind})
		}
	}
	// Water first so parks on piers and shores sit on top of it.
	sort.SliceStable(s.areas, func(i, j int) bool { return s.areas[i].kind == "water" && s.areas[j].kind != "water" })
	for i, b := range m.Buildings {
		pts := toVec(b.Pts)
		bd := building{m: newMesh(pts), height: b.Height}
		h := hash(i)
		if b.Height >= 25 {
			bd.roof = tallRoofs[h%len(tallRoofs)]
		} else {
			bd.roof = lowRoofs[h%len(lowRoofs)]
		}
		w, d := bd.m.box[2]-bd.m.box[0], bd.m.box[3]-bd.m.box[1]
		cx, cy := (bd.m.box[0]+bd.m.box[2])/2, (bd.m.box[1]+bd.m.box[3])/2
		for k := 0; k < min(6, int(w*d/500)); k++ {
			p := street.Vec{X: cx + (frac(float64(h+k*31)*0.618)-0.5)*w*0.6, Y: cy + (frac(float64(h*7+k*17)*0.414)-0.5)*d*0.6}
			if polyContains(pts, p) {
				bd.units = append(bd.units, p)
			}
		}
		s.buildings = append(s.buildings, bd)
	}
	s.highways = newHighways(m)
	s.landmarks = newLandmarks(m)
	for _, t := range m.Trees {
		s.mapTrees = append(s.mapTrees, street.Vec{X: t[0], Y: t[1]})
	}
	return s
}

func hash(i int) int {
	x := uint32(i)*2654435761 + 12345
	x ^= x >> 13
	return int(x % 1000003)
}

func (c *canvas) meshVisible(m *mesh, pad float64) bool {
	return c.visible(m.box[0]-pad, m.box[1]-pad, m.box[2]+pad, m.box[3]+pad)
}

// sunShadow is the shadow offset per metre of building height (the sun in
// the south-west, shadows falling to the north-east... drawn to the lower
// right to match the vehicles).
var sunShadow = street.Vec{X: 0.16, Y: 0.21}

// Building shadows are drawn as one flat layer: overlapping shadows never
// darken each other, and a tall tower's shadow stays light enough to see
// the track and trams in it.
const (
	shadowCap   = 90.0 // m: towers taller than this cast no longer shadows
	shadowAlpha = 0.2
)

var colShadowTone = rgb(24, 34, 52)

func (g *Game) drawSceneGround(c *canvas) {
	for i := range g.scene.areas {
		a := &g.scene.areas[i]
		if c.meshVisible(&a.m, 5) {
			c.mesh(&a.m, areaColours[a.kind])
		}
	}
}

// drawSceneShadows lays the building shadows over the ground, streets and
// track (but under vehicles and roofs) as a single translucent layer.
func (g *Game) drawSceneShadows(dst *ebiten.Image) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	if g.shadowImg == nil || g.shadowImg.Bounds().Dx() != w || g.shadowImg.Bounds().Dy() != h {
		g.shadowImg = ebiten.NewImage(w, h)
	}
	g.shadowImg.Clear()
	c := newCanvas(g.shadowImg, g)
	// The footprint swept along the sun direction.
	for i := range g.scene.buildings {
		b := &g.scene.buildings[i]
		off := scale(sunShadow, math.Min(b.height, shadowCap))
		if !c.visible(b.m.box[0], b.m.box[1], b.m.box[2]+off.X, b.m.box[3]+off.Y) {
			continue
		}
		c.trisAt(b.m.pts, b.m.tris, off, colShadowTone)
		n := len(b.m.pts)
		for k := 0; k < n; k++ {
			p, q := b.m.pts[k], b.m.pts[(k+1)%n]
			c.fan([]street.Vec{p, q, add(q, off), add(p, off)}, colShadowTone)
		}
	}
	c.flush()
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleAlpha(shadowAlpha)
	dst.DrawImage(g.shadowImg, op)
}

func (g *Game) drawSceneBuildings(c *canvas) {
	bs := g.scene.buildings
	detail := c.px() > 1.4
	for i := range bs {
		b := &bs[i]
		if !c.meshVisible(&b.m, 2) {
			continue
		}
		c.mesh(&b.m, b.roof)
		// Parapet: a slightly darker rim; the sunlit edges a touch lighter.
		n := len(b.m.pts)
		rim := tint(b.roof, rgb(0, 0, 0), 0.22)
		lit := tint(b.roof, rgb(255, 255, 255), 0.25)
		for k := 0; k < n; k++ {
			p, q := b.m.pts[k], b.m.pts[(k+1)%n]
			col := rim
			if e := sub(q, p); e.X*sunShadow.Y-e.Y*sunShadow.X > 0 {
				col = lit
			}
			c.seg(p, q, 0.7, 0.6, col)
		}
		if detail {
			for _, u := range b.units {
				c.rect(u.X-1.4+0.6, u.Y-1+0.8, 2.8, 2, colBShadow)
				c.rect(u.X-1.4, u.Y-1, 2.8, 2, tint(b.roof, rgb(235, 236, 236), 0.45))
			}
		}
	}
}

func (g *Game) drawSceneTrees(c *canvas) {
	if c.px() < 0.3 {
		return
	}
	for _, t := range g.scene.trees {
		if !c.visibleAround(t, 6) {
			continue
		}
		r := 3.2
		c.circle(add(t, street.Vec{X: 1.2, Y: 1.6}), r, colBShadow)
		c.circle(t, r, colRealTree)
		c.circle(add(t, street.Vec{X: -0.8, Y: -0.8}), r*0.6, colTreeLight)
	}
}

// --- streets ----------------------------------------------------------------

const sidewalkWidth = 3.5

// roadFrame returns a road's ends, unit direction and right-hand normal.
func (g *Game) roadFrame(r street.RoadID) (a, b, d, right street.Vec) {
	a, b = g.net.Ends(r)
	d = unit(sub(b, a))
	return a, b, d, street.Vec{X: -d.Y, Y: d.X}
}

// band fills the strip from `left` metres left of the centre line to
// `right` metres right of it, between the road's ends.
func (g *Game) band(c *canvas, r street.RoadID, left, right float64, col colorT) {
	a, b, _, rt := g.roadFrame(r)
	c.fan([]street.Vec{add(a, scale(rt, right)), add(b, scale(rt, right)), sub(b, scale(rt, left)), sub(a, scale(rt, left))}, col)
}

// junctionHull is the paved area where car streets meet, widened by extra.
func (g *Game) junctionHull(j street.JunctionID, extra float64) []street.Vec {
	n := g.net
	c := n.Junctions[j].Pos
	reach := math.Max(n.Radius(j), 1)
	var pts []street.Vec
	for _, r := range n.RoadsAt(j) {
		rd := n.Roads[r]
		if rd.Tramway {
			continue
		}
		a, b, d, rt := g.roadFrame(r)
		_ = b
		left, right := rd.Sides()
		out := d
		if rd.B == j {
			out = scale(d, -1)
			_ = a
		}
		p := add(c, scale(out, reach))
		pts = append(pts, add(p, scale(rt, right+extra)), sub(p, scale(rt, left+extra)))
		q := add(c, scale(out, 0.1))
		pts = append(pts, add(q, scale(rt, right+extra)), sub(q, scale(rt, left+extra)))
	}
	return convexHull(pts)
}

func (g *Game) drawRealStreets(c *canvas) {
	n := g.net
	vis := func(r street.RoadID) bool { return !n.Roads[r].Tramway && g.roadVisible(c, r) }
	jvis := func(j street.Junction) bool {
		return j.Level == 0 && g.hasStreetAt(j.ID) && c.visibleAround(j.Pos, n.Radius(j.ID)+25)
	}
	// Sidewalks and kerbs.
	for _, rd := range n.Roads {
		if vis(rd.ID) {
			l, r := rd.Sides()
			g.band(c, rd.ID, l+sidewalkWidth, r+sidewalkWidth, colSidewalk)
		}
	}
	for _, j := range n.Junctions {
		if jvis(j) {
			c.fan(g.junctionHull(j.ID, sidewalkWidth), colSidewalk)
			g.jointTapers(c, j.ID, sidewalkWidth, colSidewalk)
		}
	}
	for _, rd := range n.Roads {
		if vis(rd.ID) {
			l, r := rd.Sides()
			g.band(c, rd.ID, l+0.3, r+0.3, colKerb)
		}
	}
	for _, j := range n.Junctions {
		if jvis(j) {
			g.jointTapers(c, j.ID, 0.3, colKerb)
		}
	}
	// Carriageways.
	for _, rd := range n.Roads {
		if vis(rd.ID) {
			l, r := rd.Sides()
			g.band(c, rd.ID, l, r, colRoad)
		}
	}
	for _, j := range n.Junctions {
		if jvis(j) {
			col := colRoadJoint
			if len(n.RoadsAt(j.ID)) <= 2 {
				col = colRoad // a bend, not a crossing: no patch
			}
			c.fan(g.junctionHull(j.ID, 0), col)
			g.jointTapers(c, j.ID, 0, colRoad)
		}
	}
	if c.px() < 0.45 {
		return
	}
	for _, rd := range n.Roads {
		if vis(rd.ID) {
			g.drawLaneLines(c, rd)
		}
	}
	for _, j := range n.Junctions {
		if jvis(j) && len(n.RoadsAt(j.ID)) == 2 {
			g.drawBendJoin(c, j.ID)
		}
	}
	// Track bed of own-right-of-way track through bends.
	for i := range n.Links {
		l := &n.Links[i]
		if !g.bendTrack(l) || !c.visibleAround(l.Pts[0], l.Length+5) {
			continue
		}
		w := street.LaneWidth - 0.5
		if l.Centre {
			w = n.Roads[n.Links[l.From].Road].CentreGap() - 0.2
		}
		c.polyline(l.Pts, w, 1, colTrackBed)
		for _, p := range l.Pts {
			c.circle(p, w/2, colTrackBed)
		}
	}
	for _, rd := range n.Roads {
		if vis(rd.ID) {
			g.drawCrossings(c, rd)
		}
	}
}

func (g *Game) drawLaneLines(c *canvas, rd street.Road) {
	n := g.net
	for _, fwd := range []bool{true, false} {
		lanes := rd.Lanes(fwd)
		for i := 0; i < lanes; i++ {
			for _, id := range n.LanePieces(rd.ID, fwd, i) {
				l := &n.Links[id]
				p0, p1 := l.Pts[0], l.Pts[len(l.Pts)-1]
				d := unit(sub(p1, p0))
				right := street.Vec{X: -d.Y, Y: d.X}
				if l.Track && l.Modes == street.Tram {
					c.seg(p0, p1, street.LaneWidth-0.5, 1, colTrackBed)
				}
				if i < lanes-1 {
					o := scale(right, street.LaneWidth/2)
					dashed(c, add(p0, o), add(p1, o), 3, 9, 0.15, colLine)
				}
			}
		}
	}
	if rd.OneWay() {
		return
	}
	// Centre: double yellow, a planted median, or a single track's strip.
	ps := n.LanePieces(rd.ID, true, 0)
	first, last := &n.Links[ps[0]], &n.Links[ps[len(ps)-1]]
	p0, p1 := first.Pts[0], last.Pts[len(last.Pts)-1]
	d := unit(sub(p1, p0))
	right := street.Vec{X: -d.Y, Y: d.X}
	centre := scale(right, -rd.LaneOffset(true, 0))
	m0, m1 := add(p0, centre), add(p1, centre)
	switch {
	case rd.Tram != street.NoTram && rd.Single:
		c.seg(m0, m1, rd.CentreGap()-0.2, 1, colTrackBed)
		for _, s := range []float64{-1, 1} {
			o := scale(right, s*(rd.CentreGap()/2))
			c.seg(add(m0, o), add(m1, o), 0.15, 1, colCentreY)
		}
	case rd.Class.Median() > 0:
		c.seg(m0, m1, rd.Class.Median()-0.2, 1, colKerb)
		c.seg(m0, m1, rd.Class.Median()-0.8, 1, areaColours["grass"])
	default:
		for _, s := range []float64{-0.18, 0.18} {
			o := scale(right, s)
			c.seg(add(m0, o), add(m1, o), 0.12, 1, colCentreY)
		}
	}
}

// drawCrossings paints zebra crossings and stop lines at crossings.
func (g *Game) drawCrossings(c *canvas, rd street.Road) {
	n := g.net
	a, b, d, right := g.roadFrame(rd.ID)
	left, rt := rd.Sides()
	for _, end := range []struct {
		j   street.JunctionID
		p   street.Vec
		dir street.Vec
	}{{rd.A, a, d}, {rd.B, b, scale(d, -1)}} {
		if len(n.RoadsAt(end.j)) < 3 {
			continue
		}
		box := n.BoxHalf(end.j)
		for o := -left + 0.4; o < rt-0.4; o += 1.2 {
			p := add(add(end.p, scale(end.dir, box+2.4)), scale(right, o+0.3))
			c.box(p, end.dir, 3.0, 0.55, colLine)
		}
		if n.Signal(end.j) == nil {
			continue
		}
		// Stop line across the lanes arriving here.
		sl := add(end.p, scale(end.dir, n.Radius(end.j)-0.2))
		arriving := street.Vec{}
		lo, hi := 0.0, 0.0
		if end.j == rd.B && rd.Lanes(true) > 0 { // forward lanes, right of d
			lo, hi = rd.LaneOffset(true, 0)-street.LaneWidth/2, rd.LaneOffset(true, rd.Lanes(true)-1)+street.LaneWidth/2
			arriving = right
		} else if end.j == rd.A && rd.Lanes(false) > 0 {
			lo, hi = rd.LaneOffset(false, 0)-street.LaneWidth/2, rd.LaneOffset(false, rd.Lanes(false)-1)+street.LaneWidth/2
			arriving = scale(right, -1)
		} else {
			continue
		}
		c.seg(add(sl, scale(arriving, lo)), add(sl, scale(arriving, hi)), 0.45, 1, colLine)
	}
}

// drawStreetNames labels streets along their direction.
func (g *Game) drawStreetNames(dst *ebiten.Image) {
	if g.zoom < 0.9 {
		return
	}
	n := g.net
	type placed struct {
		name string
		p    street.Vec
	}
	var done []placed
	order := make([]int, len(n.Roads))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return n.Length(street.RoadID(order[i])) > n.Length(street.RoadID(order[j])) })
	for _, i := range order {
		rd := n.Roads[i]
		if rd.Name == "" {
			continue
		}
		a, b, d, _ := g.roadFrame(rd.ID)
		mid := a.Lerp(b, 0.5)
		x, y := g.toScreen(mid)
		if x < -50 || y < 0 || int(x) > g.w+50 || int(y) > g.h {
			continue
		}
		w, _ := textui.MeasureText(rd.Name, 12)
		if n.Length(rd.ID)*g.zoom < w+40 {
			continue
		}
		dup := false
		for _, p := range done {
			if p.name == rd.Name && dist(p.p, mid) < 260 {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		done = append(done, placed{rd.Name, mid})
		ang := math.Atan2(d.Y, d.X)
		if ang > math.Pi/2 {
			ang -= math.Pi
		} else if ang < -math.Pi/2 {
			ang += math.Pi
		}
		for _, o := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			textui.DrawTextRotated(dst, rd.Name, float64(x)+o[0], float64(y)+o[1], 12, ang, colRoad)
		}
		textui.DrawTextRotated(dst, rd.Name, float64(x), float64(y), 12, ang, rgb(250, 250, 246))
	}
}

// drawAttribution credits the map data, as its licence requires: small
// text straight on the map, with a soft halo so it stays legible.
func (g *Game) drawAttribution(dst *ebiten.Image) {
	if g.scene == nil {
		return
	}
	s := locale.T("地图数据 © OpenStreetMap contributors", "Map data © OpenStreetMap contributors")
	const size = 10
	w, _ := textui.MeasureText(s, size)
	x, y := float64(g.w)-w-10, float64(g.h)-86
	for _, o := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		textui.DrawText(dst, s, x+o[0], y+o[1], size, rgba(90, 90, 90, 90)) // premultiplied: white at 35%
	}
	textui.DrawText(dst, s, x, y, size, rgb(40, 42, 46))
}

// sidesOut is a road's carriageway width left and right of its centre line,
// looking away from junction j along it.
func sidesOut(n *street.Network, r street.RoadID, j street.JunctionID) (left, right float64) {
	rd := n.Roads[r]
	l, rt := rd.Sides()
	if rd.A == j {
		return l, rt
	}
	return rt, l
}

func (g *Game) streetRoadsAt(j street.JunctionID) []street.RoadID {
	var out []street.RoadID
	for _, r := range g.net.RoadsAt(j) {
		if !g.net.Roads[r].Tramway {
			out = append(out, r)
		}
	}
	return out
}

// jointTapers smooths the step where a street changes width at a plain
// joint (two streets, no crossing): the wider side narrows into the other
// road over a short taper instead of ending square.
func (g *Game) jointTapers(c *canvas, j street.JunctionID, extra float64, col colorT) {
	n := g.net
	rs := g.streetRoadsAt(j)
	if len(rs) != 2 {
		return
	}
	at := n.Junctions[j].Pos
	out := func(r street.RoadID) (street.Vec, float64) {
		a, b := n.Ends(r)
		if n.Roads[r].B == j {
			a, b = b, a
		}
		return unit(sub(b, a)), dist(a, b)
	}
	rightN := func(d street.Vec) street.Vec { return street.Vec{X: -d.Y, Y: d.X} }
	leftN := func(d street.Vec) street.Vec { return street.Vec{X: d.Y, Y: -d.X} }
	d0, len0 := out(rs[0])
	d1, len1 := out(rs[1])
	l0, r0 := sidesOut(n, rs[0], j)
	l1, r1 := sidesOut(n, rs[1], j)
	// Travelling from road 0 into road 1: its left kerb is road 0's
	// outward right, and its right kerb road 0's outward left.
	sides := []struct {
		w0, w1 float64
		n0, n1 street.Vec
	}{
		{r0, l1, rightN(d0), leftN(d1)},
		{l0, r1, leftN(d0), rightN(d1)},
	}
	for _, s := range sides {
		w0, w1 := s.w0+extra, s.w1+extra
		switch {
		case w0 > w1+0.3:
			t := math.Min(15, 0.4*len1)
			edge := add(at, scale(s.n1, w1))
			c.fan([]street.Vec{add(at, scale(s.n0, w0)), add(edge, scale(d1, t)), edge}, col)
		case w1 > w0+0.3:
			t := math.Min(15, 0.4*len0)
			edge := add(at, scale(s.n0, w0))
			c.fan([]street.Vec{add(at, scale(s.n1, w1)), add(edge, scale(d0, t)), edge}, col)
		}
	}
}

// bendTrack reports whether a connector is ground tram track with its own right
// of way carried through a bend in the road (a joint of two roads, not a
// crossing).
func (g *Game) bendTrack(l *street.Link) bool {
	n := g.net
	if l.Kind != street.ConnectorLink || l.Modes&street.Tram == 0 || len(n.RoadsAt(l.Junction)) > 2 || n.Junctions[l.Junction].Level != 0 {
		return false
	}
	from, to := &n.Links[l.From], &n.Links[l.To]
	return from.Modes == street.Tram && to.Modes == street.Tram && from.Track && to.Track
}

// centreEnd is where a two-way road's centre line ends at junction j.
func (g *Game) centreEnd(r street.RoadID, j street.JunctionID) (street.Vec, bool) {
	n := g.net
	rd := n.Roads[r]
	if rd.OneWay() {
		return street.Vec{}, false
	}
	ps := n.LanePieces(r, true, 0)
	if len(ps) == 0 {
		return street.Vec{}, false
	}
	first, last := &n.Links[ps[0]], &n.Links[ps[len(ps)-1]]
	p0, p1 := first.Pts[0], last.Pts[len(last.Pts)-1]
	d := unit(sub(p1, p0))
	right := street.Vec{X: -d.Y, Y: d.X}
	centre := scale(right, -rd.LaneOffset(true, 0))
	if rd.A == j {
		return add(p0, centre), true
	}
	return add(p1, centre), true
}

// drawBendJoin carries a road's centre markings through a bend, so a
// gently kinked street reads as one continuous street.
func (g *Game) drawBendJoin(c *canvas, j street.JunctionID) {
	n := g.net
	// Centre line, median or single-track strip.
	rs := g.streetRoadsAt(j)
	if len(rs) != 2 {
		return
	}
	a, b := n.Roads[rs[0]], n.Roads[rs[1]]
	pa, okA := g.centreEnd(a.ID, j)
	pb, okB := g.centreEnd(b.ID, j)
	if !okA || !okB {
		return
	}
	single := func(rd street.Road) bool { return rd.Tram != street.NoTram && rd.Single }
	switch {
	case single(a) && single(b):
		c.seg(pa, pb, math.Min(a.CentreGap(), b.CentreGap())-0.2, 1, colTrackBed)
	case !single(a) && !single(b) && a.Class.Median() > 0 && b.Class.Median() > 0:
		m := math.Min(a.Class.Median(), b.Class.Median())
		c.seg(pa, pb, m-0.2, 1, colKerb)
		c.seg(pa, pb, m-0.8, 1, areaColours["grass"])
	case !single(a) && !single(b) && a.Class.Median() == 0 && b.Class.Median() == 0:
		d := unit(sub(pb, pa))
		right := street.Vec{X: -d.Y, Y: d.X}
		for _, s := range []float64{-0.18, 0.18} {
			o := scale(right, s)
			c.seg(add(pa, o), add(pb, o), 0.12, 1, colCentreY)
		}
	}
}
