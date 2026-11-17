package game

import (
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"math"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// Motorways, expressways (I-5, SR 99, SR 520...) and their ramps are a road
// type of their own: drawn with shoulders and lane lines, viaducts above
// the streets they cross, but not part of the street network. No tram
// track can be laid on them.

type highway struct {
	nodes  []int        // map node of each point
	pts    []street.Vec // centre line
	w      []float64    // carriageway width at each point (tapers where lanes change)
	width  float64      // full carriageway width, m
	lanes  int
	link   bool // ramp
	bridge bool
	level  int // -2..2
	box    [4]float64
	// joined is, for the first / last point, the narrowest width of the
	// pieces it runs on into (0: a free end).
	joined [2]float64
}

// taperLength is how far a carriageway narrows or widens to meet a
// neighbour with a different number of lanes.
const taperLength = 40.0

func newHighways(m *mapdata.Map) []highway {
	var out []highway
	for _, r := range m.Highways {
		h := highway{lanes: max(r.Fwd+r.Back, 1), bridge: r.Bridge, level: min(max(r.Layer, -2), 2)}
		h.link = r.Class == "motorway_link" || r.Class == "trunk_link"
		h.width = float64(h.lanes) * 3.6
		if !h.link {
			h.width += 3 // shoulders
		} else {
			h.width += 1.5
		}
		for _, id := range r.Nodes {
			h.nodes = append(h.nodes, id)
			h.pts = append(h.pts, vec(m.Nodes[id]))
		}
		out = append(out, h)
	}
	// Pieces meeting end to end with a different width taper into each
	// other instead of stepping.
	ends := map[int][]int{}
	for i, h := range out {
		ends[h.nodes[0]] = append(ends[h.nodes[0]], i)
		ends[h.nodes[len(h.nodes)-1]] = append(ends[h.nodes[len(h.nodes)-1]], i)
	}
	for i := range out {
		h := &out[i]
		for e, node := range []int{h.nodes[0], h.nodes[len(h.nodes)-1]} {
			for _, o := range ends[node] {
				if o != i && (h.joined[e] == 0 || out[o].width < h.joined[e]) {
					h.joined[e] = out[o].width
				}
			}
		}
		h.w = make([]float64, len(h.pts))
		for k := range h.w {
			h.w[k] = h.width
		}
		for _, first := range []bool{true, false} {
			node := h.nodes[len(h.nodes)-1]
			if first {
				node = h.nodes[0]
			}
			if len(ends[node]) != 2 {
				continue // a free end, or a split / merge
			}
			o := ends[node][0]
			if o == i {
				o = ends[node][1]
			}
			if out[o].width < h.width {
				h.taper(first, out[o].width)
			}
		}
	}
	for i := range out {
		out[i].bounds()
	}
	return out
}

// taper narrows one end of the carriageway to width over taperLength,
// adding a point where the taper starts.
func (h *highway) taper(first bool, width float64) {
	if first {
		h.reverse()
		defer h.reverse()
	}
	// Walk back from the last point.
	left := math.Min(taperLength, h.length()*0.45)
	k := len(h.pts) - 1
	for k > 0 && left > 0 {
		d := dist(h.pts[k], h.pts[k-1])
		if d >= left {
			p := h.pts[k].Lerp(h.pts[k-1], left/d)
			h.pts = append(h.pts[:k], append([]street.Vec{p}, h.pts[k:]...)...)
			h.nodes = append(h.nodes[:k], append([]int{-1}, h.nodes[k:]...)...)
			h.w = append(h.w[:k], append([]float64{h.width}, h.w[k:]...)...)
			break
		}
		left -= d
		k--
	}
	// Widths fall linearly from the taper's start to the end.
	total, run := 0.0, []float64{0}
	start := k
	for j := start; j+1 < len(h.pts); j++ {
		total += dist(h.pts[j], h.pts[j+1])
		run = append(run, total)
	}
	for j := start; j < len(h.pts); j++ {
		t := 1.0
		if total > 0 {
			t = run[j-start] / total
		}
		h.w[j] = math.Min(h.w[j], h.width+(width-h.width)*t)
	}
}

func (h *highway) reverse() {
	for i, j := 0, len(h.pts)-1; i < j; i, j = i+1, j-1 {
		h.pts[i], h.pts[j] = h.pts[j], h.pts[i]
		h.nodes[i], h.nodes[j] = h.nodes[j], h.nodes[i]
		h.w[i], h.w[j] = h.w[j], h.w[i]
	}
	h.joined[0], h.joined[1] = h.joined[1], h.joined[0]
}

func (h *highway) length() float64 {
	d := 0.0
	for i := 0; i+1 < len(h.pts); i++ {
		d += dist(h.pts[i], h.pts[i+1])
	}
	return d
}

func (h *highway) bounds() {
	h.box = [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range h.pts {
		h.box = [4]float64{math.Min(h.box[0], p.X), math.Min(h.box[1], p.Y), math.Max(h.box[2], p.X), math.Max(h.box[3], p.Y)}
	}
}

// joinStreets moves ramp ends onto the street junctions they meet.
func (s *scene) joinStreets(n *street.Network) {
	for i := range s.highways {
		h := &s.highways[i]
		for k, id := range h.nodes {
			if j, ok := n.Gates[id]; ok && id >= 0 {
				h.pts[k] = n.Junctions[j].Pos
			}
		}
		h.bounds()
	}
}

func vec(p mapdata.Pt) street.Vec { return street.Vec{X: p[0], Y: p[1]} }

var (
	colHighway     = rgb(70, 72, 76)
	colHighwayEdge = rgb(160, 160, 154)
	colViaductSide = rgb(205, 203, 196)
	colViaductShad = rgba(0, 0, 0, 60)
	colTunnel      = rgba(60, 64, 72, 70)
)

func (c *canvas) highwayVisible(h *highway, pad float64) bool {
	return c.visible(h.box[0]-pad, h.box[1]-pad, h.box[2]+pad, h.box[3]+pad)
}

// band fills the carriageway (plus extra on each side), point widths
// included, with round joins.
func (h *highway) band(c *canvas, extra float64, col colorT, off street.Vec) {
	for i := 0; i+1 < len(h.pts); i++ {
		a, b := add(h.pts[i], off), add(h.pts[i+1], off)
		l := dist(a, b)
		if l < 0.01 {
			continue
		}
		n := street.Vec{X: -(b.Y - a.Y) / l, Y: (b.X - a.X) / l}
		wa, wb := (h.w[i]+extra)/2, (h.w[i+1]+extra)/2
		c.fan([]street.Vec{add(a, scale(n, wa)), add(b, scale(n, wb)), sub(b, scale(n, wb)), sub(a, scale(n, wa))}, col)
	}
	for k := 1; k < len(h.pts)-1; k++ {
		c.circle(add(h.pts[k], off), (h.w[k]+extra)/2, col)
	}
	// Round ends where it runs on into another piece, so pieces meeting at
	// an angle (or on different levels) leave no notch.
	for e, k := range []int{0, len(h.pts) - 1} {
		if h.joined[e] > 0 {
			c.circle(add(h.pts[k], off), (math.Min(h.w[k], h.joined[e])+extra)/2, col)
		}
	}
}

// drawHighways draws the motorways on one level: 0 at street level, below
// the streets; 1 and 2 above them. Tunnels (level < 0) are only shown faint
// from the ground, and fully in an underground view.
func (g *Game) drawHighways(c *canvas, level int) {
	hs := g.scene.highways
	none := street.Vec{}
	for i := range hs {
		h := &hs[i]
		if h.level != level || !c.highwayVisible(h, 30) {
			continue
		}
		switch {
		case level > 0:
			h.band(c, 1.6, colViaductShad, scale(sunShadow, 9*float64(level)))
			h.band(c, 1.6, colViaductSide, none)
		case level == 0:
			h.band(c, 2.5, colHighwayEdge, none)
		}
	}
	for i := range hs {
		h := &hs[i]
		if h.level == level && c.highwayVisible(h, 30) {
			h.band(c, 0, colHighway, none)
		}
	}
	if c.px() < 0.45 {
		return
	}
	for i := range hs {
		h := &hs[i]
		if h.level == level && c.highwayVisible(h, 30) {
			g.drawHighwayLines(c, h)
		}
	}
}

// drawTunnels shows the motorway tunnels faintly from above.
func (g *Game) drawTunnels(c *canvas) {
	for i := range g.scene.highways {
		h := &g.scene.highways[i]
		if h.level < 0 && c.highwayVisible(h, 30) {
			h.band(c, 0, colTunnel, street.Vec{})
		}
	}
}

// drawHighwayLines paints edge lines and dashed lane lines.
func (g *Game) drawHighwayLines(c *canvas, h *highway) {
	for i := 0; i+1 < len(h.pts); i++ {
		a, b := h.pts[i], h.pts[i+1]
		l := dist(a, b)
		if l < 0.5 {
			continue
		}
		d := scale(sub(b, a), 1/l)
		n := street.Vec{X: -d.Y, Y: d.X}
		inset := 0.7
		if !h.link {
			inset += 0.8
		}
		ha, hb := h.w[i]/2-inset, h.w[i+1]/2-inset
		for _, s := range []float64{-1, 1} {
			c.seg(add(a, scale(n, s*ha)), add(b, scale(n, s*hb)), 0.2, 0.6, colLine)
		}
		if h.w[i] != h.width || h.w[i+1] != h.width {
			continue // no lane lines where it tapers
		}
		half := h.width/2 - inset
		lane := (2 * half) / float64(h.lanes)
		for k := 1; k < h.lanes; k++ {
			s := -half + lane*float64(k)
			for t := 1.5; t+3 < l; t += 12 {
				p := add(add(a, scale(d, t)), scale(n, s))
				c.seg(p, add(p, scale(d, 3)), 0.15, 0.5, colLine)
			}
		}
	}
}

// highwayAt reports whether p lies on a motorway or ramp, and how far it is
// from its centre line.
func (g *Game) highwayAt(p street.Vec) (*highway, float64, bool) {
	if g.scene == nil {
		return nil, 0, false
	}
	var best *highway
	bd := math.Inf(1)
	for i := range g.scene.highways {
		h := &g.scene.highways[i]
		if h.level < 0 {
			continue
		}
		if p.X < h.box[0]-h.width || p.X > h.box[2]+h.width || p.Y < h.box[1]-h.width || p.Y > h.box[3]+h.width {
			continue
		}
		for k := 0; k+1 < len(h.pts); k++ {
			if d := segDist(p, h.pts[k], h.pts[k+1]); d < h.width/2+1 && d < bd {
				best, bd = h, d
			}
		}
	}
	return best, bd, best != nil
}

// onHighway reports whether the cursor points at a motorway rather than at
// a street (a street passing under or over it wins when nearer).
func (g *Game) onHighway() (*highway, bool) {
	h, d, ok := g.highwayAt(g.cursor)
	if !ok {
		return nil, false
	}
	if _, ok := g.hoverJunction(); ok {
		return nil, false
	}
	if r, _, ok := g.net.NearestRoad(g.cursor, g.pick(12)); ok {
		a, b := g.net.Ends(r)
		if segDist(g.cursor, a, b) <= d+1 {
			return nil, false
		}
	}
	return h, true
}

func highwayRefusal() string {
	return locale.T("高速公路和匝道上不能建地面有轨电车", "Ground-level trams can't be built on motorways or ramps")
}

// markHighway outlines a motorway piece in red.
func (g *Game) markHighway(c *canvas, h *highway) {
	h.band(c, 2, colBad, street.Vec{})
}
