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

// carPose places a car on its path by its two axles, each following the
// lane: it points from the rear axle to the front one and sits halfway
// between them. Through a sharp corner the car swings round smoothly,
// instead of snapping to the new direction when its nose passes the
// corner (which is what placing it by the nose's segment did).
func carPose(pts []street.Vec, length float64) (centre, dir street.Vec) {
	front, fd := along(pts, 0.2*length)
	rear, _ := along(pts, 0.8*length)
	d := sub(front, rear)
	if l := math.Hypot(d.X, d.Y); l > 1e-6 {
		dir = scale(d, 1/l)
	} else {
		dir = fd
	}
	return front.Lerp(rear, 0.5), dir
}

// vehicleElevation returns the vertical level (-1..+1) at a vehicle's head.
func (g *Game) vehicleElevation(v *traffic.Vehicle) float64 {
	id, s := v.Link()
	return g.net.LinkElevation(id, s)
}

// followedTramUnderground reports whether the camera is following a tram that
// is currently inside an underground tunnel.
func (g *Game) followedTramUnderground() bool {
	if g.page != pageRun || g.follow == 0 {
		return false
	}
	v := g.sim.Find(g.follow)
	return v != nil && g.vehicleElevation(v) < -0.55
}

// drawVehicles draws ground-level cars and trams (including trams on tunnel
// portals above the tunnel mouth). Elevated trams and deep-tunnel trams are
// drawn in their own passes so bridges, portals and buildings layer properly.
func (g *Game) drawVehicles(c *canvas) {
	g.drawVehiclesBand(c, -0.75, 0.20, true)
}

// drawVehiclesBand draws vehicles whose elevation lies in [minZ, maxZ].
func (g *Game) drawVehiclesBand(c *canvas, minZ, maxZ float64, includeCars bool) {
	if g.page != pageRun {
		return // building and planning show the network, not the service
	}
	for _, v := range g.sim.Vehicles {
		if v.Kind == traffic.CarKind && !includeCars {
			continue
		}
		if v.Kind == traffic.TramKind {
			z := g.vehicleElevation(v)
			if z < minZ || z > maxZ {
				continue
			}
		}
		g.drawOneVehicle(c, v)
	}
}

func (g *Game) drawOneVehicle(c *canvas, v *traffic.Vehicle) {
	pts := g.sim.Body(v)
	if len(pts) < 2 || !c.visibleAround(pts[len(pts)-1], v.Length+5) {
		return
	}
	switch v.Kind {
	case traffic.CarKind:
		centre, d := carPose(pts, v.Length)
		c.car(centre, d, v.Length, v.Width, carPalette[int(v.Variant)%len(carPalette)], v.Variant == 8)
	case traffic.TramKind:
		doors := tramDoors{per: 2}
		model := street.DefaultTramModel()
		if l := g.net.Lines[v.Line]; l != nil {
			doors.per = l.DoorsPerSection()
			model = l.TramModel()
		}
		if ds, open := v.Doors(); ds != traffic.DoorsClosed && v.Dwelling {
			head, d := along(pts, 0)
			doors.side, doors.open = g.doorSide(v, head, d), open
		}
		c.tram(pts, v, model, lineColors[g.lineIndex(v.Line)%len(lineColors)], doors, g.sim.Capacity(v))
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
	if c.px() >= 8 {
		c.bevelBox(p, u, l+0.06, w+0.06, 0.46, tint(col, rgb(10, 14, 20), 0.55))
	}
	c.bevelBox(p, u, l, w, 0.45, col)
	c.box(at(-0.06*l, 0), u, 0.62*l, w-0.45, rgb(26, 46, 59))
	c.box(at(0.18*l, 0), u, 0.08*l, w-0.7, rgb(76, 109, 127))
	c.bevelBox(at(-0.06*l, 0), u, 0.33*l, w-0.75, 0.15, tint(col, rgb(245, 248, 249), 0.18))
	if c.px() >= 10 {
		for _, sd := range []float64{-1, 1} {
			c.box(at(0.16*l, sd*(w/2+0.06)), u, 0.22, 0.14, col)
		}
	}
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

// tramDoors is how a tram's doors are drawn: per section, which side opens
// (1 right, -1 left, 0 none) and how far open (0..1).
type tramDoors struct {
	per        int
	side, open float64
}

// noseEndProfile returns half-profile offsets (backFromTip, widthFrac) from
// left (-1) to right (+1) for a cab nose or an inner module joint. Every
// profile is strictly convex so polygon fans stay crisp at all zoom levels.
func noseEndProfile(nose street.NoseKind, cab bool, maxDepth float64) [][2]float64 {
	if !cab {
		d := math.Min(0.20, maxDepth)
		return [][2]float64{{d, -1}, {0, -0.86}, {0, 0.86}, {d, 1}}
	}
	scaleD := func(d float64, pts [][2]float64) [][2]float64 {
		k := math.Min(1, maxDepth/math.Max(d, 0.01))
		out := make([][2]float64, len(pts))
		for i, p := range pts {
			out[i] = [2]float64{p[0] * k, p[1]}
		}
		return out
	}
	switch nose {
	case street.NoseSleek:
		return scaleD(1.50, [][2]float64{
			{1.50, -1.0}, {0.92, -0.88}, {0.42, -0.68}, {0.10, -0.40},
			{0, -0.18}, {0, 0.18},
			{0.10, 0.40}, {0.42, 0.68}, {0.92, 0.88}, {1.50, 1.0},
		})
	case street.NoseFaceted:
		return scaleD(1.05, [][2]float64{
			{1.05, -1.0}, {0.32, -0.74}, {0, -0.36},
			{0, 0.36}, {0.32, 0.74}, {1.05, 1.0},
		})
	case street.NoseStreamlined:
		return scaleD(1.32, [][2]float64{
			{1.32, -1.0}, {0.72, -0.86}, {0.26, -0.62}, {0, -0.32},
			{0, 0.32}, {0.26, 0.62}, {0.72, 0.86}, {1.32, 1.0},
		})
	case street.NoseAngular:
		return scaleD(0.84, [][2]float64{
			{0.84, -1.0}, {0, -0.60}, {0, 0.60}, {0.84, 1.0},
		})
	case street.NoseWedge:
		return scaleD(1.28, [][2]float64{
			{1.28, -1.0}, {0.48, -0.78}, {0, -0.34},
			{0, 0.34}, {0.48, 0.78}, {1.28, 1.0},
		})
	case street.NoseBoxy:
		return scaleD(0.50, [][2]float64{
			{0.50, -1.0}, {0.16, -0.90}, {0, -0.70},
			{0, 0.70}, {0.16, 0.90}, {0.50, 1.0},
		})
	default: // NosePanoramic
		return scaleD(1.12, [][2]float64{
			{1.12, -1.0}, {0.64, -0.88}, {0.24, -0.66}, {0.04, -0.36},
			{0, 0},
			{0.04, 0.36}, {0.24, 0.66}, {0.64, 0.88}, {1.12, 1.0},
		})
	}
}

// sectionPoly builds the world-space convex hull of one tram module.
func sectionPoly(m, u, r street.Vec, l, w float64, nose street.NoseKind, frontCab, rearCab bool) []street.Vec {
	hl, hw := l/2, w/2
	maxD := l * 0.34
	fp := noseEndProfile(nose, frontCab, maxD)
	rp := noseEndProfile(nose, rearCab, maxD)
	pts := make([]street.Vec, 0, len(fp)+len(rp))
	at := func(a, s float64) street.Vec { return add(add(m, scale(u, a)), scale(r, s)) }
	for _, p := range fp {
		pts = append(pts, at(hl-p[0], p[1]*hw))
	}
	for i := len(rp) - 1; i >= 0; i-- {
		p := rp[i]
		pts = append(pts, at(-hl+p[0], p[1]*hw))
	}
	return pts
}

func (c *canvas) tram(pts []street.Vec, v *traffic.Vehicle, model street.TramModel, livery color.RGBA, doors tramDoors, cap int) {
	px := c.px()
	w := v.Width
	if w <= 0 {
		w = model.Width
	}
	secs := tramSections(v.Length, v.Cars)
	n := len(secs)
	panto := n / 2 // pantograph on the middle module
	accent := rgb(model.Accent[0], model.Accent[1], model.Accent[2])
	roofCol := rgb(228, 232, 237)
	if model.Vintage {
		roofCol = rgb(196, 186, 166) // classic painted canvas roof
		if model.DarkRoof {
			roofCol = rgb(72, 68, 62)
		}
	} else if model.DarkRoof {
		roofCol = rgb(52, 58, 66)
	}

	// 1. Soft drop shadows under all modules and joints first.
	for k, sec := range secs {
		f, _ := along(pts, sec.start)
		b, _ := along(pts, sec.end)
		u := unit(sub(f, b))
		r := street.Vec{X: -u.Y, Y: u.X}
		m := add(f.Lerp(b, 0.5), street.Vec{X: 0.48, Y: 0.62})
		c.fan(sectionPoly(m, u, r, dist(f, b), w, model.Nose, k == 0, k == n-1), rgba(2, 6, 23, 96))
	}
	for i := 0; i+1 < n; i++ {
		f, _ := along(pts, secs[i].end)
		b, _ := along(pts, secs[i+1].start)
		c.box(add(f.Lerp(b, 0.5), street.Vec{X: 0.45, Y: 0.58}), unit(sub(f, b)), 1.1, w-0.28, rgba(2, 6, 23, 85))
	}

	// 2. Articulated gangway bellows (or classic coupled vestibule on vintage sets).
	for i := 0; i+1 < n; i++ {
		f, _ := along(pts, secs[i].end)
		b, _ := along(pts, secs[i+1].start)
		u := unit(sub(f, b))
		m := f.Lerp(b, 0.5)
		if model.Vintage {
			// Coupled vintage cars: draw drawbar + narrow vestibule diaphragm.
			c.box(m, u, 1.16, 0.34, rgb(32, 34, 38))
			c.box(m, u, 0.72, w*0.56, rgb(48, 46, 44))
			continue
		}
		bw := w - 0.28
		c.box(m, u, 1.16, bw, rgb(40, 44, 50))
		if px >= 4.5 {
			const folds = 5
			spanLen := math.Max(dist(f, b)+0.24, 0.72)
			foldL := spanLen / float64(folds*2)
			for j := 0; j < folds*2; j++ {
				t := (float64(j) + 0.5) / float64(folds*2)
				p := b.Lerp(f, t)
				col := rgb(58, 64, 72)
				fw := bw - 0.04
				if j%2 == 1 {
					col = rgb(30, 34, 40)
					fw = bw - 0.14
				}
				c.box(p, u, foldL*1.05, fw, col)
			}
			c.box(m, u, spanLen*0.75, w*0.48, tint(roofCol, rgb(90, 98, 108), 0.35))
		}
	}

	fillRatio := 0.0
	if cap > 0 {
		fillRatio = math.Min(1, float64(v.Load())/float64(cap))
	}

	// 3. Modules: bogie skirts, sculpted hull, interior, glass, doors, roof & lamps.
	for k, sec := range secs {
		f, _ := along(pts, sec.start)
		b, _ := along(pts, sec.end)
		u := unit(sub(f, b))
		l := dist(f, b)
		m := f.Lerp(b, 0.5)
		r := street.Vec{X: -u.Y, Y: u.X}
		at := func(a, s float64) street.Vec { return add(add(m, scale(u, a)), scale(r, s)) }
		frontCab, rearCab := k == 0, k == n-1

		// Underframe bogie side-skirt hints (or exposed cast-iron truck frames on vintage trams).
		if px >= 8 && l > 3.2 {
			for _, ax := range []float64{-0.72, 0.72} {
				for _, sd := range []float64{-1, 1} {
					c.box(at(ax, sd*(w/2+0.02)), u, 0.62, 0.12, rgb(24, 28, 34))
					if px >= 16 {
						rimCol := rgb(92, 100, 110)
						if model.Vintage {
							rimCol = rgb(124, 96, 68)
						}
						c.box(at(ax, sd*(w/2+0.04)), u, 0.42, 0.06, rimCol)
					}
				}
			}
			c.box(at(0, 0), u, 1.9, w+0.02, rgba(18, 22, 28, 160))
		}
		// Classic life-guard / cowcatcher slats under the front nose of vintage streetcars.
		if model.Vintage && frontCab && px >= 6 {
			for _, s := range []float64{-0.32, -0.16, 0, 0.16, 0.32} {
				c.box(at(l/2+0.06, s*w), u, 0.18, 0.05, rgb(44, 42, 38))
			}
			c.box(at(l/2+0.12, 0), u, 0.05, w*0.72, rgb(58, 54, 48))
		}

		// Outer hull rim + main livery shell.
		if px >= 5 {
			c.fan(sectionPoly(m, u, r, l, w, model.Nose, frontCab, rearCab), tint(livery, rgb(8, 12, 18), 0.52))
			c.fan(sectionPoly(m, u, r, l-0.08, w-0.08, model.Nose, frontCab, rearCab), livery)
		} else {
			c.fan(sectionPoly(m, u, r, l, w, model.Nose, frontCab, rearCab), livery)
		}
		if px < 1.5 {
			c.fan(sectionPoly(m, u, r, math.Max(l-0.75, 0.5), math.Max(w-0.68, 0.5), model.Nose, frontCab, rearCab), roofCol)
			continue
		}

		// Subtle sunlit/shaded body bevels & accent / cream window-belt trim.
		if px >= 4 {
			bodySpan := l - 0.9
			if frontCab {
				bodySpan -= 0.45
			}
			if rearCab {
				bodySpan -= 0.45
			}
			shift := 0.0
			if frontCab && !rearCab {
				shift = -0.22
			} else if rearCab && !frontCab {
				shift = 0.22
			}
			if bodySpan > 0.6 {
				if model.Vintage {
					// Classic ivory/cream window sash band along both sides.
					for _, sd := range []float64{-1, 1} {
						c.box(at(shift, sd*(w/2-0.28)), u, bodySpan+0.16, 0.38, accent)
					}
				} else {
					c.box(at(shift, -(w/2-0.11)), u, bodySpan, 0.10, tint(livery, rgb(255, 255, 255), 0.24))
					c.box(at(shift, w/2-0.11), u, bodySpan, 0.10, tint(livery, rgb(0, 0, 0), 0.20))
					if px >= 6 {
						for _, sd := range []float64{-1, 1} {
							c.box(at(shift, sd*(w/2-0.20)), u, bodySpan, 0.06, tint(livery, accent, 0.72))
						}
					}
				}
			}
		}

		// Side window bands & interior seats visible at high zoom.
		winLen := l - 1.0
		winShift := 0.0
		if frontCab && rearCab {
			winLen = l - 2.1
		} else if frontCab {
			winLen = l - 1.55
			winShift = -0.28
		} else if rearCab {
			winLen = l - 1.55
			winShift = 0.28
		}
		if winLen > 0.5 {
			bay := 1.05
			if model.Vintage {
				bay = 0.72 // closely spaced sash windows
			} else if model.PanoramicWindow {
				bay = 1.42
			}
			for _, sd := range []float64{-1, 1} {
				wy := sd * (w/2 - 0.36)
				glassCol := rgb(22, 36, 48)
				if model.Vintage {
					glassCol = rgb(36, 44, 46)
				}
				c.box(at(winShift, wy), u, winLen, 0.30, glassCol)
				if px >= 9 {
					floorCol := rgb(74, 82, 92)
					seatCol := tint(livery, rgb(64, 96, 128), 0.65)
					if model.Vintage {
						floorCol = rgb(118, 92, 62) // warm wooden floor & incandescent glow
						seatCol = rgb(146, 98, 54)  // rattan / wooden slat benches
					}
					c.box(at(winShift, sd*(w/2-0.44)), u, winLen-0.12, 0.12, floorCol)
					bays := max(int(winLen/bay), 1)
					step := winLen / float64(bays)
					for bIdx := 0; bIdx < bays; bIdx++ {
						bx := winShift - winLen/2 + (float64(bIdx)+0.5)*step
						c.box(at(bx, sd*(w/2-0.42)), u, step*0.58, 0.11, seatCol)
						slotHash := float64((v.ID*17+k*7+bIdx*3+int(sd+2))%10) / 10.0
						if fillRatio > slotHash {
							c.circle(at(bx, sd*(w/2-0.40)), 0.095, rgb(214, 188, 162))
						}
						if px >= 14 {
							sheen := rgba(195, 228, 246, 75)
							if model.Vintage {
								sheen = rgba(255, 232, 172, 75)
							}
							c.box(at(bx+0.06, sd*(w/2-0.31)), u, step*0.42, 0.04, sheen)
						}
					}
				}
				if px >= 5 {
					pillars := int(winLen / bay)
					pillarCol := tint(livery, rgb(20, 26, 34), 0.35)
					if model.Vintage {
						pillarCol = tint(accent, rgb(110, 82, 52), 0.40)
					}
					for pIdx := 1; pIdx < pillars; pIdx++ {
						pxPos := winShift - winLen/2 + float64(pIdx)*(winLen/float64(pillars))
						c.box(at(pxPos, wy), u, 0.075, 0.31, pillarCol)
					}
				}
			}
		}

		// Recessed roof deck.
		roofInsetW := w - 0.96
		if model.Vintage {
			roofInsetW = w - 0.78
		}
		roofL := l - 0.85
		roofShift := 0.0
		if frontCab && rearCab {
			roofL = l - 1.85
		} else if frontCab {
			roofL = l - 1.35
			roofShift = -0.25
		} else if rearCab {
			roofL = l - 1.35
			roofShift = 0.25
		}
		if roofL > 0.4 && roofInsetW > 0.4 {
			rm := add(m, scale(u, roofShift))
			if px >= 6 {
				c.bevelBox(rm, u, roofL+0.08, roofInsetW+0.08, 0.22, tint(roofCol, rgb(20, 26, 32), 0.35))
			}
			c.bevelBox(rm, u, roofL, roofInsetW, 0.20, roofCol)
			c.tramRoof(k, n, panto, rm, u, r, roofL, roofInsetW, w, model, livery, accent, roofCol)
		}

		// Doors along both sides.
		c.tramDoors(k, n, at, u, l, w, livery, model.Vintage, doors)

		// Front and rear cabs.
		if frontCab {
			c.tramCab(at, m, u, r, l, w, model, livery, accent, true)
		}
		if rearCab {
			c.tramCab(at, m, u, r, l, w, model, livery, accent, false)
		}
	}
}

// tramCab draws a tram's front (head=true) or rear (head=false) cab end.
func (c *canvas) tramCab(at func(a, s float64) street.Vec, m, u, r street.Vec, l, w float64, model street.TramModel, livery, accent color.RGBA, head bool) {
	px := c.px()
	cu := u
	if !head {
		cu = scale(u, -1)
	}
	cr := street.Vec{X: -cu.Y, Y: cu.X}
	cat := func(a, s float64) street.Vec {
		return add(add(m, scale(cu, l/2-a)), scale(cr, s))
	}

	glassDepth, glassStart := 0.86, 0.18
	switch model.Nose {
	case street.NoseSleek:
		glassDepth, glassStart = 1.12, 0.24
	case street.NoseStreamlined, street.NoseWedge:
		glassDepth, glassStart = 0.98, 0.22
	case street.NoseBoxy:
		glassDepth, glassStart = 0.66, 0.12
	case street.NoseAngular:
		glassDepth, glassStart = 0.78, 0.16
	}
	if model.Vintage {
		glassDepth = math.Min(glassDepth, 0.68)
		glassStart = 0.16
	}

	if px >= 5 {
		c.bevelBox(cat(glassStart*0.75+0.05, 0), cu, glassStart*0.8, w*0.42, 0.06, tint(livery, accent, 0.65))
	}
	if !model.Vintage && px >= 6 && (model.Nose == street.NoseStreamlined || model.Nose == street.NoseWedge) {
		c.bevelBox(cat(0.06, 0), cu, 0.12, w*0.44, 0.04, rgb(28, 32, 38))
		if px >= 14 {
			c.box(cat(0.02, 0), cu, 0.07, 0.22, rgb(58, 64, 72))
		}
	}

	// Windscreen surround + glass (3-pane divided vestibule sash on vintage streetcars).
	gw := w - 0.42
	cut := 0.28
	switch model.Nose {
	case street.NoseSleek, street.NoseStreamlined:
		gw = w - 0.48
		cut = 0.38
	case street.NoseBoxy:
		gw = w - 0.34
		cut = 0.16
	}
	glassMid := glassStart + glassDepth/2
	frameCol := rgb(18, 26, 34)
	if model.Vintage {
		frameCol = tint(accent, rgb(86, 62, 38), 0.35)
	}
	c.bevelBox(cat(glassMid, 0), cu, glassDepth, gw, cut, frameCol)
	c.bevelBox(cat(glassMid+0.02, 0), cu, glassDepth-0.12, gw-0.12, cut*0.85, rgb(28, 46, 60))
	if model.Vintage && px >= 6 {
		// Classic three-window vestibule mullions.
		for _, sd := range []float64{-0.22, 0.22} {
			c.box(cat(glassMid, sd*gw), cu, glassDepth, 0.06, frameCol)
		}
	}

	if px >= 7 {
		if model.Vintage {
			// Brass master controller & handbrake wheel in vintage vestibule,
			// plus classic cream-on-black rollsign box on the roof brow!
			if head && px >= 10 {
				c.circle(cat(glassStart+0.30, -0.18), 0.09, rgb(198, 162, 72))
				c.circle(cat(glassStart+0.30, 0.20), 0.08, rgb(172, 138, 62))
			}
			signA := glassStart + glassDepth - 0.10
			c.box(cat(signA, 0), cu, 0.16, gw*0.52, rgb(22, 22, 20))
			c.box(cat(signA, 0), cu, 0.10, gw*0.42, rgb(246, 234, 188))
		} else {
			c.bevelBox(cat(glassStart+0.34, 0), cu, 0.28, gw*0.66, 0.08, rgb(38, 44, 52))
			if head && px >= 11 {
				c.box(cat(glassStart+0.36, 0.16), cu, 0.10, 0.22, rgb(76, 210, 235))
				c.box(cat(glassStart+0.36, -0.18), cu, 0.09, 0.16, rgb(180, 214, 140))
				c.circle(cat(glassStart+0.58, 0), 0.15, rgb(56, 64, 76))
			}
			signA := glassStart + glassDepth - 0.16
			c.box(cat(signA, 0), cu, 0.18, gw*0.64, rgb(14, 16, 20))
			c.box(cat(signA, -gw*0.21), cu, 0.12, gw*0.14, livery)
			ledCol := rgb(255, 186, 52)
			if !head {
				ledCol = rgb(224, 140, 44)
			}
			c.box(cat(signA, gw*0.06), cu, 0.09, gw*0.34, ledCol)
		}
	}
	if !model.Vintage && px >= 12 {
		for _, sd := range []float64{-0.26, 0.22} {
			c.seg(cat(glassStart+0.10, sd*gw), cat(glassStart+0.48, (sd+0.18)*gw), 0.035, 0.9, rgb(18, 20, 24))
		}
	}
	if head && !model.Vintage && px >= 8 {
		mirrorA := glassStart + glassDepth + 0.06
		for _, sd := range []float64{-1, 1} {
			c.seg(cat(mirrorA, sd*(w/2-0.04)), cat(mirrorA-0.10, sd*(w/2+0.15)), 0.06, 1.0, rgb(32, 36, 42))
			c.box(cat(mirrorA-0.12, sd*(w/2+0.15)), cu, 0.16, 0.08, livery)
		}
	}

	// Single central brass/chrome headlamp on pre-1950s streetcars vs modern twin lamps.
	if model.SingleHeadlamp {
		if !head && model.Panto == street.PantoTrolleyPole && px >= 6 {
			// Trolley rope retriever drum mounted on the rear dash.
			c.circle(cat(0.06, 0.22), 0.11, rgb(52, 54, 58))
		}
		rimCol := rgb(204, 168, 78) // polished brass rim
		lampCol := rgb(255, 242, 188)
		if !head {
			lampCol = rgb(198, 34, 42)
		}
		c.circle(cat(0.08, 0), 0.18, rimCol)
		c.circle(cat(0.06, 0), 0.13, lampCol)
		if head && px >= 10 {
			c.circle(cat(0.05, 0), 0.07, rgb(255, 253, 238))
		}
		if model.TopLamp && px >= 5 {
			c.circle(cat(glassStart+glassDepth-0.02, 0), 0.10, lampCol)
		}
		return
	}

	lampMain := rgb(252, 249, 224)
	lampGlow := rgba(255, 252, 232, 165)
	if !head {
		lampMain = rgb(206, 32, 44)
		lampGlow = rgba(235, 48, 58, 150)
	}
	lampSpread := w/2 - 0.38
	lampBack := 0.14
	switch model.Nose {
	case street.NoseSleek:
		lampBack = 0.34
		lampSpread = w/2 - 0.34
		for _, sd := range []float64{-1, 1} {
			p0 := cat(0.16, sd*(lampSpread-0.14))
			p1 := cat(0.56, sd*(lampSpread+0.10))
			if px >= 5 {
				c.seg(p0, p1, 0.16, 1.2, rgb(24, 28, 34))
			}
			c.seg(p0, p1, 0.09, 1.0, lampMain)
		}
	case street.NoseStreamlined, street.NoseWedge:
		lampBack = 0.26
		lampSpread = w/2 - 0.36
		for _, sd := range []float64{-1, 1} {
			if px >= 5 {
				c.bevelBox(cat(lampBack, sd*lampSpread), cu, 0.24, 0.42, 0.06, rgb(24, 28, 34))
			}
			c.box(cat(lampBack-0.02, sd*lampSpread), cu, 0.16, 0.32, lampMain)
			if px >= 10 {
				c.circle(cat(lampBack-0.03, sd*(lampSpread-0.05)), 0.07, lampGlow)
			}
		}
	default:
		lampBack = 0.28
		lampSpread = w/2 - 0.42
		if model.Nose == street.NoseBoxy {
			lampBack = 0.12
			lampSpread = w/2 - 0.36
		}
		for _, sd := range []float64{-1, 1} {
			if px >= 5 {
				c.bevelBox(cat(lampBack, sd*lampSpread), cu, 0.20, 0.40, 0.06, rgb(24, 28, 34))
			}
			c.box(cat(lampBack-0.02, sd*lampSpread), cu, 0.14, 0.30, lampMain)
			if px >= 10 {
				c.circle(cat(lampBack-0.02, sd*(lampSpread-0.05)), 0.065, lampGlow)
			}
		}
	}
	if model.TopLamp && px >= 4 {
		browA := glassStart + glassDepth - 0.02
		if head {
			c.circle(cat(browA, 0), 0.11, rgb(255, 250, 224))
		} else if px >= 8 {
			c.circle(cat(browA, 0), 0.08, rgb(175, 28, 38))
		}
	}
	_ = at
}

// tramRoof draws rooftop shrouds, clerestory lanterns, HVAC pods, batteries
// and the pantograph / trolley pole on one module.
func (c *canvas) tramRoof(k, n, panto int, rm, u, r street.Vec, rl, rw, bodyW float64, model street.TramModel, livery, accent, roofCol color.RGBA) {
	px := c.px()
	rat := func(a, s float64) street.Vec { return add(add(rm, scale(u, a)), scale(r, s)) }

	// Vintage clerestory lantern deck or wooden roof walk planks & ventilators.
	switch model.Roof {
	case street.RoofClerestory:
		clW := rw * 0.54
		clL := math.Max(rl-0.45, rl*0.78)
		c.bevelBox(rat(0, 0), u, clL+0.10, clW+0.12, 0.16, tint(roofCol, rgb(40, 30, 20), 0.42))
		c.bevelBox(rat(0, 0), u, clL, clW, 0.14, tint(roofCol, rgb(245, 240, 228), 0.18))
		if px >= 8 {
			// Tiny stained/amber transom ventilator lights along the clerestory sides.
			vents := max(int(clL/0.65), 2)
			for i := 0; i < vents; i++ {
				va := -clL*0.42 + float64(i)*(clL*0.84/float64(max(vents-1, 1)))
				for _, sd := range []float64{-1, 1} {
					c.box(rat(va, sd*(clW/2+0.02)), u, 0.34, 0.05, rgb(224, 186, 108))
				}
			}
		}
		if k == panto {
			c.tramPantograph(rat, u, rl, model.Panto)
		}
		return
	case street.RoofVintageArch:
		if px >= 6 {
			// Wooden roof walk boards along the crown + torpedo ventilators.
			for _, s := range []float64{-0.14, 0.14} {
				c.box(rat(0, s), u, rl-0.3, 0.09, tint(roofCol, rgb(118, 88, 56), 0.45))
			}
			for _, va := range []float64{-rl * 0.30, rl * 0.30} {
				for _, sd := range []float64{-1, 1} {
					c.circle(rat(va, sd*rw*0.32), 0.09, tint(roofCol, rgb(32, 34, 38), 0.50))
				}
			}
		}
		if k == panto {
			c.tramPantograph(rat, u, rl, model.Panto)
		}
		return
	}

	if px >= 7 {
		ribCol := tint(roofCol, rgb(30, 36, 44), 0.18)
		for _, sd := range []float64{-1, 1} {
			c.box(rat(0, sd*(rw/2-0.14)), u, rl-0.2, 0.07, ribCol)
		}
	}
	if px >= 10 && rl > 1.2 {
		c.box(rat(0, rw*0.24), u, rl, 0.05, rgb(212, 84, 38))
	}
	if model.Roof == street.RoofShrouded && px >= 4 {
		shroud := tint(livery, accent, 0.55)
		for _, sd := range []float64{-1, 1} {
			c.box(rat(0, sd*(rw/2-0.06)), u, rl-0.1, 0.18, shroud)
		}
	}
	if k == panto {
		c.tramPantograph(rat, u, rl, model.Panto)
		return
	}

	podShift := -rl * 0.12
	if k == 0 && n > 1 {
		podShift = -rl * 0.16
	} else if k == n-1 && n > 1 {
		podShift = rl * 0.16
	}
	podLen := math.Min(2.3, rl*0.58)
	podW := math.Min(1.36, rw*0.78)
	if podLen < 0.6 || podW < 0.5 {
		return
	}

	drawHVAC := func(centreA, length, width float64) {
		c.bevelBox(rat(centreA, 0), u, length, width, 0.12, rgb(156, 164, 172))
		c.bevelBox(rat(centreA, 0), u, length-0.22, width-0.22, 0.08, rgb(126, 134, 142))
		if px >= 7 && length >= 1.1 {
			fanR := math.Min(width*0.26, length*0.18)
			for _, fa := range []float64{-length * 0.24, length * 0.24} {
				c.circle(rat(centreA+fa, 0), fanR, rgb(52, 58, 66))
				if px >= 13 {
					c.circle(rat(centreA+fa, 0), fanR*0.36, rgb(110, 118, 126))
				}
			}
		}
	}

	switch model.Roof {
	case street.RoofLRV:
		drawHVAC(podShift, podLen, podW)
		if rl > 3.6 && px >= 6 {
			otherA := -podShift * 0.9
			if math.Abs(otherA-podShift) > 1.3 {
				c.bevelBox(rat(otherA, 0), u, 0.95, podW*0.72, 0.08, rgb(112, 120, 128))
			}
		}
	case street.RoofBattery:
		c.bevelBox(rat(podShift, 0), u, podLen, podW, 0.10, rgb(76, 84, 94))
		if px >= 8 {
			fins := 5
			for i := 0; i < fins; i++ {
				fa := podShift - podLen*0.36 + float64(i)*(podLen*0.72/float64(fins-1))
				c.box(rat(fa, 0), u, 0.06, podW-0.22, rgb(48, 54, 62))
			}
			c.box(rat(podShift, podW*0.32), u, 0.24, 0.14, rgb(238, 186, 42))
		}
	case street.RoofModular:
		drawHVAC(podShift, podLen*0.88, podW)
		if rl > 3.4 && px >= 7 {
			ra := podShift + math.Copysign(podLen*0.66, -podShift+0.01)
			if math.Abs(ra)+0.5 < rl/2 {
				c.box(rat(ra, 0), u, 0.78, podW*0.68, rgb(92, 98, 106))
				if px >= 12 {
					for _, s := range []float64{-0.2, 0, 0.2} {
						c.box(rat(ra, s*podW), u, 0.64, 0.06, rgb(54, 58, 64))
					}
				}
			}
		}
	case street.RoofShrouded:
		drawHVAC(podShift, podLen*0.86, podW*0.86)
	default:
		drawHVAC(0, math.Min(1.75, rl*0.48), podW*0.84)
	}
	_ = bodyW
}

// tramPantograph draws a modern Z-pantograph, classic diamond pantograph or
// vintage swivelling trolley pole.
func (c *canvas) tramPantograph(rat func(a, s float64) street.Vec, u street.Vec, rl float64, kind street.PantoKind) {
	px := c.px()
	if kind == street.PantoTrolleyPole {
		// Swivelling central spring base, long slender tapered pole reaching
		// backward to a brass harp & trolley wheel on the wire!
		poleReach := math.Min(2.35, rl*0.48+0.65)
		c.box(rat(0.25, 0), u, 0.76, 0.44, rgb(62, 64, 68))
		c.circle(rat(0.35, 0), 0.20, rgb(44, 46, 50))
		if px >= 8 {
			// Twin tension springs flanking the base pivot.
			for _, sd := range []float64{-0.14, 0.14} {
				c.box(rat(0.05, sd), u, 0.44, 0.07, rgb(92, 96, 102))
			}
		}
		// Slender pole + subtle shadow.
		c.seg(rat(0.35, 0.05), rat(-poleReach, 0.06), 0.07, 0.9, rgba(10, 14, 20, 90))
		c.seg(rat(0.35, 0), rat(-poleReach, 0), 0.075, 1.1, rgb(42, 44, 48))
		// Brass collector harp & wheel at the pole tip.
		c.box(rat(-poleReach, 0), u, 0.24, 0.16, rgb(206, 168, 74))
		if px >= 9 {
			c.circle(rat(-poleReach, 0), 0.06, rgb(120, 126, 132))
		}
		return
	}

	c.bevelBox(rat(0, 0), u, 2.2, 1.24, 0.10, rgb(124, 130, 138))
	if px >= 7 {
		for _, ia := range []float64{-0.88, 0.88} {
			for _, is := range []float64{-0.48, 0.48} {
				c.circle(rat(ia, is), 0.10, rgb(232, 234, 236))
				c.circle(rat(ia, is), 0.06, rgb(146, 98, 72))
			}
		}
	}
	armCol := rgb(38, 42, 48)
	stripCol := rgb(30, 34, 38)
	switch kind {
	case street.PantoDiamond:
		c.seg(rat(-0.95, -0.45), rat(0, -0.78), 0.07, 0.8, armCol)
		c.seg(rat(0, -0.78), rat(0.95, -0.45), 0.07, 0.8, armCol)
		c.seg(rat(-0.95, 0.45), rat(0, 0.78), 0.07, 0.8, armCol)
		c.seg(rat(0, 0.78), rat(0.95, 0.45), 0.07, 0.8, armCol)
		c.seg(rat(-0.95, -0.45), rat(0.95, 0.45), 0.05, 0.6, armCol)
		c.seg(rat(-0.95, 0.45), rat(0.95, -0.45), 0.05, 0.6, armCol)
		c.tramPanhead(rat, u, 0, stripCol, px)
	default: // PantoSingleArm (Z-type)
		c.box(rat(0.55, 0), u, 0.34, 0.56, rgb(64, 70, 78))
		c.seg(rat(0.55, -0.16), rat(-0.82, -0.06), 0.085, 0.9, armCol)
		c.seg(rat(0.55, 0.16), rat(-0.82, 0.06), 0.085, 0.9, armCol)
		if px >= 8 {
			c.box(rat(-0.82, 0), u, 0.16, 0.34, rgb(58, 64, 72))
		}
		c.seg(rat(-0.82, 0), rat(0.18, -0.42), 0.07, 0.8, armCol)
		c.seg(rat(-0.82, 0), rat(0.18, 0.42), 0.07, 0.8, armCol)
		if px >= 10 {
			c.seg(rat(0.46, 0), rat(-0.74, 0), 0.04, 0.6, rgb(76, 82, 90))
		}
		c.tramPanhead(rat, u, 0.18, stripCol, px)
	}
}

func (c *canvas) tramPanhead(rat func(a, s float64) street.Vec, u street.Vec, a float64, col color.RGBA, px float64) {
	if px < 7 {
		c.box(rat(a, 0), u, 0.26, 1.92, col)
		return
	}
	for _, da := range []float64{-0.11, 0.11} {
		c.box(rat(a+da, 0), u, 0.07, 1.72, col)
		if px >= 12 {
			c.box(rat(a+da, 0), u, 0.04, 1.42, rgb(86, 92, 98))
		}
		for _, sd := range []float64{-1, 1} {
			c.seg(rat(a+da, sd*0.86), rat(a+da*1.7, sd*1.02), 0.055, 0.7, rgb(172, 178, 186))
		}
	}
}

// drawSignals draws road signal heads at the requested level.
func (g *Game) drawSignals(c *canvas) {
	g.drawSignalsAt(c, 0)
}

func (g *Game) drawSignalsAt(c *canvas, level int) {
	if c.px() < 0.5 || g.page != pageRun {
		return
	}
	t := g.sim.Seconds()
	for _, h := range g.net.SignalHeads() {
		if h.Level != level || !c.visibleAround(h.Pos, 5) {
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

var (
	colTramLampOn  = rgb(255, 246, 214) // tram signals show white bars
	colTramLampOff = rgb(58, 56, 50)
)

// drawTramSignals draws the ground-level tram-only signals.
func (g *Game) drawTramSignals(c *canvas) {
	g.drawTramSignalsAt(c, 0)
}

func (g *Game) drawTramSignalsAt(c *canvas, level int) {
	if c.px() < 0.5 || g.page != pageRun {
		return
	}
	t := g.sim.Seconds()
	for _, h := range g.net.TramSignalHeads() {
		if h.Level != level || !c.visibleAround(h.Pos, 5) {
			continue
		}
		asp := g.net.Signal(h.Junction).AspectAt(h.Phase, t)
		d := h.Dir
		r := street.Vec{X: -d.Y, Y: d.X}
		c.box(add(h.Pos, street.Vec{X: 0.35, Y: 0.45}), r, 2.2, 2.2, colShadow)
		c.bevelBox(h.Pos, r, 2.2, 2.2, 0.45, rgb(20, 22, 26))
		c.bevelBox(h.Pos, r, 1.9, 1.9, 0.38, rgb(214, 170, 52))
		c.bevelBox(h.Pos, r, 1.6, 1.6, 0.32, rgb(16, 16, 18))
		on := func(a street.Aspect) colorT {
			if asp == a {
				return colTramLampOn
			}
			return colTramLampOff
		}
		symbols := []struct {
			a    street.Aspect
			draw func(col colorT)
		}{
			{street.Red, func(col colorT) { c.box(h.Pos, r, 1.2, 0.26, col) }},
			{street.Green, func(col colorT) { c.box(h.Pos, d, 1.2, 0.26, col) }},
			{street.Amber, func(col colorT) { c.circle(h.Pos, 0.22, col) }},
		}
		for _, lit := range []bool{false, true} {
			for _, sym := range symbols {
				if (asp == sym.a) == lit {
					sym.draw(on(sym.a))
				}
			}
		}
	}
}

func (c *canvas) tramDoors(k, n int, at func(a, s float64) street.Vec, u street.Vec, l, w float64, livery colorT, vintage bool, d tramDoors) {
	px := c.px()
	per := max(d.per, 1)
	span := l - 2.4
	if k == 0 && n == 1 {
		span = math.Max(l-3.4, 1.4)
	} else if k == 0 || k == n-1 {
		span = math.Max(l-3.6, 1.4)
	}
	const doorLen = 1.3
	for i := 0; i < per; i++ {
		a := -span/2 + span*(float64(i)+0.5)/float64(per)
		if k == 0 && n > 1 {
			a -= 0.65
		} else if k == n-1 && n > 1 {
			a += 0.65
		}
		for _, sd := range []float64{-1, 1} {
			edge := sd * (w/2 - 0.08)
			if d.side != sd || d.open <= 0 {
				c.box(at(a, edge), u, doorLen, 0.16, tint(livery, rgb(0, 0, 0), 0.35))
				if px >= 6 {
					leaf := doorLen/2 - 0.08
					for _, dir := range []float64{-1, 1} {
						c.box(at(a+dir*(leaf/2+0.04), sd*(w/2-0.12)), u, leaf*0.78, 0.11, rgb(24, 38, 50))
					}
				}
				c.box(at(a, edge), u, 0.05, 0.17, rgb(22, 26, 32))
				if px >= 13 {
					if vintage {
						c.circle(at(a+doorLen/2+0.08, sd*(w/2-0.04)), 0.035, rgb(208, 172, 78)) // brass grab handle
					} else {
						c.circle(at(a+0.16, sd*(w/2-0.05)), 0.035, rgb(84, 224, 130))
					}
				}
				continue
			}
			gap := doorLen * d.open
			c.box(at(a, sd*(w/2-0.30)), u, gap, 0.58, rgb(24, 26, 30))
			if px >= 8 {
				c.box(at(a, sd*(w/2-0.38)), u, gap*0.92, 0.32, rgb(92, 86, 76))
			}
			if d.open > 0.85 {
				stepCol := rgb(255, 222, 96)
				if vintage {
					stepCol = rgb(168, 132, 82)
				}
				c.box(at(a, sd*(w/2+0.16)), u, gap, 0.32, stepCol)
			}
			leaf := doorLen / 2
			for _, dir := range []float64{-1, 1} {
				centre := a + dir*(leaf/2+gap/2)
				posY := sd * (w/2 + 0.07*d.open)
				c.box(at(centre, posY), u, leaf, 0.18, tint(livery, rgb(255, 255, 255), 0.15))
				if px >= 7 {
					c.box(at(centre, posY), u, leaf*0.72, 0.10, rgb(26, 42, 54))
				}
			}
		}
	}
}
