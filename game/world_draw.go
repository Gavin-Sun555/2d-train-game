package game

import (
	"image/color"
	"math"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// Palette after 2d-bus-game's default daytime theme.
var (
	colGround    = rgb(56, 94, 68)
	colGrain     = rgb(68, 108, 78)
	colPaving    = rgb(118, 128, 136)
	colAsphalt   = rgb(32, 43, 55)
	colCurb      = rgb(162, 174, 182)
	colWater     = rgb(28, 92, 116)
	colShallows  = rgb(48, 126, 144)
	colFoliage   = rgb(34, 86, 58)
	colLeafLight = rgb(94, 152, 98)
	colShadow    = rgba(8, 14, 22, 96)
	colMarking   = rgba(232, 236, 238, 230)
	colYellow    = rgb(236, 190, 64)
	colTrackBed  = rgb(70, 112, 70)
	colRail      = rgb(178, 184, 190)
	colSleeper   = rgb(84, 76, 66)
	colPlaza     = rgb(124, 134, 140)
	colLawn      = rgb(76, 122, 80)
)

// drawGround paints tiles: water, pavements and park lawns.
func (g *Game) drawGround(c *canvas) {
	m := g.city
	const ts = city.TileSize
	x0, y0 := max(0, int(c.view[0]/ts)-1), max(0, int(c.view[1]/ts)-1)
	x1, y1 := min(m.W-1, int(c.view[2]/ts)+1), min(m.H-1, int(c.view[3]/ts)+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			fx, fy := float64(x)*ts, float64(y)*ts
			switch m.At(x, y).Kind {
			case city.Water, city.Bridge:
				c.rect(fx, fy, ts, ts, colWater)
			case city.Road:
				c.rect(fx, fy, ts, ts, colPaving)
			case city.Park:
				c.rect(fx, fy, ts, ts, colLawn)
			}
		}
	}
	// Shallows along the banks.
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			k := m.At(x, y).Kind
			if k != city.Water && k != city.Bridge {
				continue
			}
			fx, fy := float64(x)*ts, float64(y)*ts
			wet := func(dx, dy int) bool {
				k := m.At(x+dx, y+dy).Kind
				return k == city.Water || k == city.Bridge || x+dx < 0 || y+dy < 0 || x+dx >= m.W || y+dy >= m.H
			}
			const b = 3.0
			if !wet(0, -1) {
				c.rect(fx, fy, ts, b, colShallows)
			}
			if !wet(0, 1) {
				c.rect(fx, fy+ts-b, ts, b, colShallows)
			}
			if !wet(-1, 0) {
				c.rect(fx, fy, b, ts, colShallows)
			}
			if !wet(1, 0) {
				c.rect(fx+ts-b, fy, b, ts, colShallows)
			}
		}
	}
}

// tree draws a canopy with shadow and sunlit highlight.
func (c *canvas) tree(p street.Vec, r float64) {
	c.circle(add(p, street.Vec{X: r * 0.35, Y: r * 0.45}), r, colShadow)
	c.circle(p, r, colFoliage)
	c.circle(add(p, street.Vec{X: -r * 0.25, Y: -r * 0.25}), r*0.62, colLeafLight)
}

// --- buildings --------------------------------------------------------------

var (
	frameCols = []color.RGBA{rgb(54, 92, 118), rgb(76, 102, 118), rgb(62, 96, 116), rgb(138, 134, 122), rgb(96, 104, 112)}
	deckCols  = []color.RGBA{rgb(104, 144, 166), rgb(144, 164, 174), rgb(122, 152, 168), rgb(182, 176, 162), rgb(150, 156, 160)}
	roofWarm  = []color.RGBA{rgb(150, 96, 74), rgb(168, 120, 86), rgb(128, 84, 70), rgb(112, 118, 126), rgb(158, 140, 110)}
)

// archBlock is a building mass: drop shadow, bevelled frame with a band of
// windows, and a recessed roof deck.
func (c *canvas) archBlock(x, y, w, h float64, frame, deck color.RGBA, shadow float64, windows bool) {
	cx, cy := x+w/2, y+h/2
	p := street.Vec{X: cx, Y: cy}
	u := street.Vec{X: 1}
	c.bevelBox(add(p, street.Vec{X: shadow * 0.72, Y: shadow * 0.95}), u, w, h, 1.5, colShadow)
	c.bevelBox(p, u, w, h, 1.5, frame)
	if windows && c.px() > 0.8 {
		c.bevelBox(p, u, w-1.2, h-1.2, 1.2, tint(frame, rgb(18, 34, 48), 0.55))
		c.bevelBox(p, u, w-2.0, h-2.0, 1.0, tint(frame, rgb(160, 200, 220), 0.25))
	}
	c.bevelBox(p, u, w-3, h-3, 1, deck)
}

func (c *canvas) hvac(x, y, w, h float64) {
	c.rect(x-w/2+0.4, y-h/2+0.5, w, h, colShadow)
	c.rect(x-w/2, y-h/2, w, h, rgb(150, 160, 166))
	if c.px() > 2 {
		for i := 1; i < 4; i++ {
			c.rect(x-w/2+w*float64(i)/4-0.1, y-h/2+0.3, 0.2, h-0.6, rgb(96, 106, 112))
		}
	}
}

func (c *canvas) skylight(x, y, w, h float64, cols, rows int) {
	c.rect(x-w/2, y-h/2, w, h, rgb(70, 92, 104))
	cw, ch := w/float64(cols), h/float64(rows)
	for i := 0; i < cols; i++ {
		for k := 0; k < rows; k++ {
			c.rect(x-w/2+float64(i)*cw+0.3, y-h/2+float64(k)*ch+0.3, cw-0.6, ch-0.6, rgb(126, 176, 196))
		}
	}
}

func (c *canvas) solar(x, y, w, h float64, cols, rows int) {
	cw, ch := w/float64(cols), h/float64(rows)
	for i := 0; i < cols; i++ {
		for k := 0; k < rows; k++ {
			c.rect(x-w/2+float64(i)*cw+0.2, y-h/2+float64(k)*ch+0.2, cw-0.4, ch-0.4, rgb(36, 58, 100))
		}
	}
}

// house is a small pitched-roof house: two roof planes shaded differently.
func (c *canvas) house(x, y, w, h float64, roof color.RGBA, alongX bool) {
	c.rect(x+0.8, y+1.0, w, h, colShadow)
	if alongX {
		c.rect(x, y, w, h/2, tint(roof, rgb(255, 240, 220), 0.18))
		c.rect(x, y+h/2, w, h/2, tint(roof, rgb(0, 0, 0), 0.18))
		c.rect(x, y+h/2-0.15, w, 0.3, tint(roof, rgb(255, 255, 255), 0.35))
	} else {
		c.rect(x, y, w/2, h, tint(roof, rgb(255, 240, 220), 0.18))
		c.rect(x+w/2, y, w/2, h, tint(roof, rgb(0, 0, 0), 0.18))
		c.rect(x+w/2-0.15, y, 0.3, h, tint(roof, rgb(255, 255, 255), 0.35))
	}
}

func (g *Game) drawLots(c *canvas) {
	for _, lot := range g.city.Lots {
		if !c.visible(lot.X-10, lot.Y-10, lot.X+lot.W+10, lot.Y+lot.H+10) {
			continue
		}
		g.drawLot(c, lot)
	}
}

func (g *Game) drawLot(c *canvas, l city.Lot) {
	v := l.Variant
	x, y, w, h := l.X, l.Y, l.W, l.H
	cx, cy := x+w/2, y+h/2
	at := func(fx, fy float64) street.Vec { return street.Vec{X: x + fx*w, Y: y + fy*h} }
	switch l.Style {
	case city.LotPark:
		c.rect(x, y, w, h, colLawn)
		c.seg(at(0.05, 0.1), at(0.95, 0.9), 2.2, 1, rgb(176, 168, 140))
		c.seg(at(0.9, 0.08), at(0.1, 0.92), 2.2, 1, rgb(176, 168, 140))
		if v%3 == 0 {
			c.circle(at(0.5, 0.5), math.Min(w, h)*0.18, colWater)
			c.circle(at(0.5, 0.5), math.Min(w, h)*0.15, colShallows)
		}
		for i := 0; i < 14; i++ {
			fx := 0.08 + 0.84*frac(float64(v+i*37)*0.6180339)
			fy := 0.08 + 0.84*frac(float64(v*3+i*53)*0.4142135)
			c.tree(at(fx, fy), 3+1.5*frac(float64(i+v)*0.7))
		}
	case city.LotPlaza:
		c.rect(x, y, w, h, colPlaza)
		for i := 1; i < 6; i++ {
			c.rect(x, y+h*float64(i)/6, w, 0.25, rgb(112, 122, 128))
			c.rect(x+w*float64(i)/6, y, 0.25, h, rgb(112, 122, 128))
		}
		c.circle(street.Vec{X: cx, Y: cy}, math.Min(w, h)*0.14, rgb(170, 176, 178))
		c.circle(street.Vec{X: cx, Y: cy}, math.Min(w, h)*0.11, colShallows)
		for _, f := range [][2]float64{{0.12, 0.12}, {0.88, 0.12}, {0.12, 0.88}, {0.88, 0.88}, {0.5, 0.1}, {0.5, 0.9}} {
			c.tree(at(f[0], f[1]), 3.2)
		}
	case city.LotTower:
		c.rect(x, y, w, h, colPlaza)
		fr, dk := frameCols[v%len(frameCols)], deckCols[v%len(deckCols)]
		c.archBlock(x+w*0.08, y+h*0.1, w*0.84, h*0.72, tint(fr, rgb(150, 150, 150), 0.25), tint(dk, rgb(200, 200, 200), 0.2), 3, true)
		c.archBlock(x+w*0.18, y+h*0.16, w*0.5, h*0.5, fr, dk, 9, true)
		c.skylight(x+w*0.43, y+h*0.32, w*0.18, h*0.14, 3, 2)
		c.hvac(x+w*0.3, y+h*0.52, 4, 3)
		c.hvac(x+w*0.56, y+h*0.52, 4, 3)
		if v%2 == 0 {
			c.solar(x+w*0.76, y+h*0.45, w*0.16, h*0.3, 2, 4)
		}
		c.tree(at(0.1, 0.92), 3)
		c.tree(at(0.9, 0.92), 3)
	case city.LotShops:
		n := 3 + v%3
		uw := (w - 2) / float64(n)
		for i := 0; i < n; i++ {
			fr := roofWarm[(v+i)%len(roofWarm)]
			c.archBlock(x+1+float64(i)*uw, y+1, uw-1, h*0.45, fr, tint(fr, rgb(230, 220, 200), 0.3), 3, false)
			c.hvac(x+1+float64(i)*uw+uw/2, y+h*0.24, 2.5, 1.8)
			fr2 := roofWarm[(v+i+2)%len(roofWarm)]
			c.archBlock(x+1+float64(i)*uw, y+h*0.55, uw-1, h*0.43, fr2, tint(fr2, rgb(230, 220, 200), 0.3), 3, false)
		}
	case city.LotHomes:
		c.rect(x, y, w, h, colLawn)
		cols, rows := 3, 3
		hw, hh := w/float64(cols), h/float64(rows)
		for i := 0; i < cols; i++ {
			for k := 0; k < rows; k++ {
				if (v+i*3+k)%7 == 0 {
					continue
				}
				roof := roofWarm[(v+i+k*2)%len(roofWarm)]
				hx, hy := x+float64(i)*hw, y+float64(k)*hh
				c.house(hx+hw*0.15, hy+hh*0.15, hw*0.6, hh*0.55, roof, (i+k)%2 == 0)
				c.rect(hx+hw*0.78, hy+hh*0.2, hw*0.12, hh*0.7, rgb(150, 152, 150))
				c.tree(street.Vec{X: hx + hw*0.3, Y: hy + hh*0.85}, 2)
			}
		}
	case city.LotCourt:
		fr, dk := roofWarm[v%len(roofWarm)], deckCols[(v+1)%len(deckCols)]
		_ = dk
		deck := tint(fr, rgb(225, 215, 200), 0.35)
		t := math.Min(w, h) * 0.24
		c.rect(x+t, y+t, w-2*t, h-2*t, colLawn)
		c.tree(street.Vec{X: cx, Y: cy}, math.Min(w, h)*0.12)
		c.archBlock(x, y, w, t, fr, deck, 6, true)
		c.archBlock(x, y+h-t, w, t, fr, deck, 6, true)
		c.archBlock(x, y+t, t, h-2*t, fr, deck, 6, true)
		c.archBlock(x+w-t, y+t, t, h-2*t, fr, deck, 6, true)
		c.hvac(x+w*0.25, y+t/2, 3, 2)
		c.hvac(x+w*0.75, y+h-t/2, 3, 2)
	}
}

func frac(f float64) float64 { return f - math.Floor(f) }

// --- streets ----------------------------------------------------------------

func (g *Game) roadVisible(c *canvas, r street.RoadID) bool {
	a, b := g.net.Ends(r)
	m := g.net.Roads[r].HalfWidth() + 10
	return c.visible(math.Min(a.X, b.X)-m, math.Min(a.Y, b.Y)-m, math.Max(a.X, b.X)+m, math.Max(a.Y, b.Y)+m)
}

func dashed(c *canvas, a, b street.Vec, dash, gap, width float64, col color.RGBA) {
	l := dist(a, b)
	for t := 0.0; t < l; t += dash + gap {
		c.seg(a.Lerp(b, t/l), a.Lerp(b, math.Min(t+dash, l)/l), width, 1, col)
	}
}

// --- tram infrastructure ----------------------------------------------------

// crossoverMark outlines where a crossover would go (or is) on a road.
func (g *Game) crossoverMark(c *canvas, p, d street.Vec, rd street.Road, col color.RGBA) {
	l, r := rd.Sides()
	w := math.Max(l, r) * 2
	if rd.Tram != street.NoTram && !rd.Single {
		w = rd.CentreGap() + 2*street.LaneWidth
	}
	// col is opaque; the fill is a translucent wash of it (premultiplied).
	k := 0.28
	c.box(p, d, street.CrossoverLength, w, color.RGBA{uint8(float64(col.R) * k), uint8(float64(col.G) * k), uint8(float64(col.B) * k), uint8(255 * k)})
	n := street.Vec{X: -d.Y, Y: d.X}
	h, o := street.CrossoverLength/2, w/2-street.LaneWidth/2
	for _, s := range []float64{-1, 1} {
		c.seg(add(add(p, scale(d, -h)), scale(n, s*o)), add(add(p, scale(d, h)), scale(n, -s*o)), 0.35, 1.5, col)
	}
}

func (g *Game) drawTrack(c *canvas) {
	g.drawTrackAtLevel(c, 0)
}

func (g *Game) drawStreetTrees(c *canvas) {
	if c.px() < 0.35 {
		return
	}
	n := g.net
	for _, rd := range n.Roads {
		if rd.Tramway || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		d := unit(sub(b, a))
		right := street.Vec{X: -d.Y, Y: d.X}
		l := dist(a, b)
		off := rd.HalfWidth() + 2.6
		for s := n.Radius(rd.A) + 8; s < l-n.Radius(rd.B)-6; s += 15 {
			for _, side := range []float64{-1, 1} {
				p := add(a.Lerp(b, s/l), scale(right, side*off))
				if g.city.TileAt(p).Kind != city.Road {
					continue
				}
				c.tree(p, 2.3)
			}
		}
	}
}

// drawStops draws ground platforms with shelters.
func (g *Game) drawStops(c *canvas) {
	g.drawStopsAtLevel(c, 0)
}

// platformLine is the centre line of a platform's island and how far it
// lies to the right of its track (negative: to the left), as drawn.
func (g *Game) platformLine(p street.Platform) ([]street.Vec, float64, bool) {
	n := g.net
	l := &n.Links[p.Link]
	if l.Twin >= 0 && l.Twin < l.ID {
		// Both directions of a track share one drawn platform: the
		// lower-numbered link's.
		tw := &n.Links[l.Twin]
		p.Pos, p.Link, l = tw.Length-(p.Pos-p.Len), l.Twin, tw
	}
	st := n.Stops[p.Stop]
	if st == nil {
		return nil, 0, false
	}
	rd := n.Roads[st.Road]
	pts := l.Slice(p.Pos-p.Len-1, p.Pos+1)
	if len(pts) < 2 {
		return nil, 0, false
	}
	side := street.LaneWidth/2 + 0.9
	if rd.Single {
		side = 1.35 + 0.9
	}
	switch {
	case rd.OneWay() && !rd.Single:
		// On a one-way road the track takes the leftmost lane (seen in the
		// road's direction, A to B), along the kerb: the platform is on the
		// pavement there, not out in the road. The link drawn may run
		// either way.
		ra, rb := n.Ends(rd.ID)
		if d, e := sub(pts[len(pts)-1], pts[0]), sub(rb, ra); d.X*e.X+d.Y*e.Y > 0 {
			side = -side
		}
	case rd.Class.Median() > 0 && !rd.Single:
		side = -side
	}
	return offset(pts, side), side, true
}

// doorSide is the side of a stopped tram its platform is on, seen in its
// direction of travel: 1 right, -1 left (0 if it is at no platform).
func (g *Game) doorSide(v *traffic.Vehicle, head, dir street.Vec) float64 {
	st := g.net.Stops[g.sim.TramStop(v)]
	if st == nil {
		return 0
	}
	id, _ := v.Link()
	tw := g.net.Links[id].Twin
	for _, p := range g.net.Platforms(st) {
		if p.Link != id && p.Link != tw {
			continue
		}
		isl, _, ok := g.platformLine(p)
		if !ok {
			return 0
		}
		mid := isl[0].Lerp(isl[len(isl)-1], 0.5)
		right := street.Vec{X: -dir.Y, Y: dir.X}
		if off := sub(mid, head); off.X*right.X+off.Y*right.Y >= 0 {
			return 1
		}
		return -1
	}
	return 0
}
