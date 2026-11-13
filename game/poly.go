package game

import (
	"math"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// triangulate splits a simple polygon into triangles by ear clipping and
// returns vertex index triples. Degenerate input falls back to a fan.
func triangulate(pts []street.Vec) []int {
	n := len(pts)
	if n < 3 {
		return nil
	}
	area := 0.0
	for i := range pts {
		j := (i + 1) % n
		area += pts[i].X*pts[j].Y - pts[j].X*pts[i].Y
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	if area < 0 {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			idx[i], idx[j] = idx[j], idx[i]
		}
	}
	var out []int
	guard := 0
	for len(idx) > 3 && guard < 4*n*n {
		guard++
		m := len(idx)
		found := false
		for i := 0; i < m; i++ {
			a, b, c := idx[(i+m-1)%m], idx[i], idx[(i+1)%m]
			pa, pb, pc := pts[a], pts[b], pts[c]
			if cross2(pa, pb, pc) <= 1e-9 {
				continue // reflex or flat corner
			}
			ear := true
			for _, k := range idx {
				if k == a || k == b || k == c {
					continue
				}
				if inTriangle(pts[k], pa, pb, pc) {
					ear = false
					break
				}
			}
			if !ear {
				continue
			}
			out = append(out, a, b, c)
			idx = append(idx[:i], idx[i+1:]...)
			found = true
			break
		}
		if !found {
			break
		}
	}
	if len(idx) == 3 {
		return append(out, idx...)
	}
	// Self-intersecting or degenerate: fan the rest.
	for i := 1; i+1 < len(idx); i++ {
		out = append(out, idx[0], idx[i], idx[i+1])
	}
	return out
}

func cross2(a, b, c street.Vec) float64 {
	return (b.X-a.X)*(c.Y-b.Y) - (b.Y-a.Y)*(c.X-b.X)
}

func inTriangle(p, a, b, c street.Vec) bool {
	d1, d2, d3 := cross2(a, b, p), cross2(b, c, p), cross2(c, a, p)
	return d1 >= 0 && d2 >= 0 && d3 >= 0
}

// mesh is a triangulated polygon with its bounding box.
type mesh struct {
	pts  []street.Vec
	tris []int
	box  [4]float64
}

func newMesh(pts []street.Vec) mesh {
	m := mesh{pts: pts, tris: triangulate(pts), box: [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}}
	for _, p := range pts {
		m.box[0], m.box[1] = math.Min(m.box[0], p.X), math.Min(m.box[1], p.Y)
		m.box[2], m.box[3] = math.Max(m.box[2], p.X), math.Max(m.box[3], p.Y)
	}
	return m
}

func (c *canvas) mesh(m *mesh, col colorT) {
	c.trisAt(m.pts, m.tris, street.Vec{}, col)
}

// trisAt adds indexed triangles, translated by off.
func (c *canvas) trisAt(pts []street.Vec, tris []int, off street.Vec, col colorT) {
	if len(c.vs)+len(pts) > 60000 {
		c.flush()
	}
	base := uint16(len(c.vs))
	pc := premul(col)
	for _, p := range pts {
		c.vertex(add(p, off), pc)
	}
	for _, t := range tris {
		c.is = append(c.is, base+uint16(t))
	}
}

func polyContains(poly []street.Vec, p street.Vec) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// convexHull returns the hull of points (monotone chain).
func convexHull(ps []street.Vec) []street.Vec {
	if len(ps) < 3 {
		return ps
	}
	pts := append([]street.Vec(nil), ps...)
	for i := 1; i < len(pts); i++ {
		for k := i; k > 0 && (pts[k].X < pts[k-1].X || (pts[k].X == pts[k-1].X && pts[k].Y < pts[k-1].Y)); k-- {
			pts[k], pts[k-1] = pts[k-1], pts[k]
		}
	}
	var h []street.Vec
	for pass := 0; pass < 2; pass++ {
		start := len(h)
		for _, p := range pts {
			for len(h) >= start+2 && cross2(h[len(h)-2], h[len(h)-1], p) <= 0 {
				h = h[:len(h)-1]
			}
			h = append(h, p)
		}
		h = h[:len(h)-1]
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	return h
}
