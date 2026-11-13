package game

import (
	"image/color"
	"math"

	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

var carPalette = []color.RGBA{
	rgb(226, 230, 234), rgb(40, 44, 52), rgb(176, 46, 52), rgb(42, 96, 170),
	rgb(140, 146, 152), rgb(214, 170, 52), rgb(58, 120, 88), rgb(236, 236, 240),
	rgb(240, 196, 40), // taxi
}

// along returns the point and heading at distance s back from the head of
// a polyline given tail→head.
func along(pts []street.Vec, s float64) (street.Vec, street.Vec) {
	for i := len(pts) - 1; i > 0; i-- {
		seg := dist(pts[i], pts[i-1])
		if s <= seg || i == 1 {
			d := unit(sub(pts[i], pts[i-1]))
			t := math.Min(1, s/math.Max(seg, 1e-9))
			return pts[i].Lerp(pts[i-1], t), d
		}
		s -= seg
	}
	return pts[0], street.Vec{X: 1}
}

func (g *Game) drawVehicles(c *canvas) {
	if g.page != pageRun {
		return // building and planning show the network, not the service
	}
	for _, v := range g.sim.Vehicles {
		pts := g.sim.Body(v)
		if len(pts) < 2 || !c.visibleAround(pts[len(pts)-1], v.Length+5) {
			continue
		}
		switch v.Kind {
		case traffic.CarKind:
			head, d := along(pts, 0)
			c.car(sub(head, scale(d, v.Length/2)), d, v.Length, v.Width, carPalette[int(v.Variant)%len(carPalette)], v.Variant == 8)
		case traffic.TramKind:
			c.tram(pts, v, lineColors[g.lineIndex(v.Line)%len(lineColors)])
		}
	}
}

// car draws a top-down car: wheels, bevelled shell, glass, roof and lamps.
func (c *canvas) car(p, u street.Vec, l, w float64, col color.RGBA, taxi bool) {
	r := street.Vec{X: -u.Y, Y: u.X}
	at := func(a, b float64) street.Vec { return add(add(p, scale(u, a)), scale(r, b)) }
	c.box(add(p, street.Vec{X: 0.35, Y: 0.45}), u, l, w, rgba(2, 6, 23, 90))
	if c.px() < 2.2 {
		c.bevelBox(p, u, l, w, 0.4, col)
		c.box(at(-0.05*l, 0), u, 0.5*l, w-0.5, rgb(30, 48, 60))
		return
	}
	for _, ax := range []float64{-0.31, 0.3} {
		for _, sd := range []float64{-1, 1} {
			c.box(at(l*ax, sd*(w/2-0.05)), u, 0.7, 0.28, rgb(16, 23, 29))
		}
	}
	c.bevelBox(p, u, l, w, 0.45, col)
	c.box(at(-0.06*l, 0), u, 0.62*l, w-0.45, rgb(26, 46, 59))
	c.box(at(0.18*l, 0), u, 0.08*l, w-0.7, rgb(76, 109, 127))
	c.bevelBox(at(-0.06*l, 0), u, 0.33*l, w-0.75, 0.15, tint(col, rgb(245, 248, 249), 0.18))
	if taxi {
		c.box(at(-0.06*l, 0), u, 0.5, 0.9, rgb(250, 213, 74))
	}
	for _, sd := range []float64{-1, 1} {
		c.box(at(l/2-0.12, sd*(w/2-0.35)), u, 0.18, 0.45, rgb(246, 247, 226))
		c.box(at(-l/2+0.1, sd*(w/2-0.3)), u, 0.16, 0.4, rgb(170, 36, 44))
	}
}

type span struct{ start, end float64 }

// tramSections splits a set from head to tail into its modules, leaving a
// gap at each joint for the bellows.
func tramSections(length float64, n int) []span {
	const joint = 0.6
	n = max(n, 1)
	l := (length - joint*float64(n-1)) / float64(n)
	out := make([]span, n)
	for i := range out {
		s := float64(i) * (l + joint)
		out[i] = span{s, s + l}
	}
	return out
}

func (c *canvas) tram(pts []street.Vec, v *traffic.Vehicle, livery color.RGBA) {
	w := v.Width
	secs := tramSections(v.Length, v.Cars)
	panto := len(secs) / 2 // pantograph on the middle module
	roof := rgb(226, 230, 234)
	// Shadows first so they never fall across the train itself.
	for _, sec := range secs {
		f, _ := along(pts, sec.start)
		b, _ := along(pts, sec.end)
		c.box(add(f.Lerp(b, 0.5), street.Vec{X: 0.5, Y: 0.65}), unit(sub(f, b)), dist(f, b), w, rgba(2, 6, 23, 100))
	}
	for i := 0; i+1 < len(secs); i++ {
		f, _ := along(pts, secs[i].end)
		b, _ := along(pts, secs[i+1].start)
		m := f.Lerp(b, 0.5)
		c.box(m, unit(sub(f, b)), 1.2, w-0.3, rgb(46, 50, 56))
	}
	for k, sec := range secs {
		f, _ := along(pts, sec.start)
		b, _ := along(pts, sec.end)
		u := unit(sub(f, b))
		l := dist(f, b)
		m := f.Lerp(b, 0.5)
		r := street.Vec{X: -u.Y, Y: u.X}
		at := func(a, s float64) street.Vec { return add(add(m, scale(u, a)), scale(r, s)) }
		cut := 0.25
		if k == 0 || k == len(secs)-1 {
			cut = 0.9 // rounded cab ends
		}
		c.bevelBox(m, u, l, w, cut, livery)
		if c.px() < 1.5 {
			c.box(m, u, l-0.8, w-0.7, roof)
			continue
		}
		// Window bands along both sides, roof between them.
		for _, sd := range []float64{-1, 1} {
			c.box(at(0, sd*(w/2-0.42)), u, l-1.2, 0.26, rgb(30, 46, 58))
		}
		c.box(m, u, l-1.0, w-1.1, roof)
		switch k {
		case panto:
			// Pantograph: base, frame and contact strip.
			c.box(m, u, 2.4, 1.3, rgb(120, 126, 132))
			c.seg(at(-1.1, -0.5), at(1.1, 0.5), 0.08, 0.7, rgb(40, 44, 48))
			c.seg(at(-1.1, 0.5), at(1.1, -0.5), 0.08, 0.7, rgb(40, 44, 48))
			c.box(at(0, 0), u, 0.25, 1.9, rgb(40, 44, 48))
		default:
			c.box(at(-l*0.18, 0), u, 2.4, 1.4, rgb(150, 158, 166))
			c.box(at(-l*0.18, 0), u, 2.0, 1.0, rgb(118, 126, 134))
		}
		// Both ends have a cab: windscreen, and lamps (white at the head).
		if k == 0 {
			c.bevelBox(at(l/2-0.5, 0), u, 0.8, w-0.5, 0.25, rgb(26, 40, 52))
			for _, sd := range []float64{-1, 1} {
				c.box(at(l/2-0.08, sd*(w/2-0.4)), u, 0.16, 0.45, rgb(250, 248, 220))
			}
		}
		if k == len(secs)-1 {
			c.bevelBox(at(-l/2+0.5, 0), u, 0.8, w-0.5, 0.25, rgb(26, 40, 52))
			for _, sd := range []float64{-1, 1} {
				c.box(at(-l/2+0.08, sd*(w/2-0.4)), u, 0.16, 0.45, rgb(190, 30, 40))
			}
		}
	}
}

// drawSignals draws signal heads: a housing with red, amber and green lamps.
func (g *Game) drawSignals(c *canvas) {
	if c.px() < 0.5 || g.page != pageRun {
		return
	}
	t := g.sim.Seconds()
	for _, h := range g.net.SignalHeads() {
		if !c.visibleAround(h.Pos, 5) {
			continue
		}
		asp := g.net.Signal(h.Junction).AspectAt(h.Phase, t)
		r := street.Vec{X: -h.Dir.Y, Y: h.Dir.X}
		c.box(add(h.Pos, street.Vec{X: 0.3, Y: 0.4}), r, 2.4, 0.9, colShadow)
		c.bevelBox(h.Pos, r, 2.4, 0.9, 0.2, rgb(24, 28, 34))
		lamps := []struct {
			a   street.Aspect
			on  color.RGBA
			off color.RGBA
		}{{street.Red, rgb(255, 64, 56), rgb(70, 24, 24)}, {street.Amber, rgb(255, 184, 40), rgb(70, 54, 20)}, {street.Green, rgb(60, 230, 110), rgb(20, 60, 34)}}
		for i, l := range lamps {
			col := l.off
			if asp == l.a {
				col = l.on
			}
			c.circle(add(h.Pos, scale(r, float64(i-1)*0.75)), 0.3, col)
		}
	}
}
