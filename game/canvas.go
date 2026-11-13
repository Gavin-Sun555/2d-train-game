package game

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// canvas batches world-space shapes into triangles drawn at screen
// resolution, so the map stays sharp at every zoom level.
type canvas struct {
	dst  *ebiten.Image
	g    *Game
	vs   []ebiten.Vertex
	is   []uint16
	view [4]float64 // visible world rect: minX, minY, maxX, maxY
}

var whitePixel = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
}()

func newCanvas(dst *ebiten.Image, g *Game) *canvas {
	c := &canvas{dst: dst, g: g}
	a := g.toWorld(0, 0)
	b := g.toWorld(g.w, g.h)
	c.view = [4]float64{a.X, a.Y, b.X, b.Y}
	return c
}

// visible reports whether a world-space box (with margin) is on screen.
func (c *canvas) visible(x0, y0, x1, y1 float64) bool {
	return x1 >= c.view[0] && x0 <= c.view[2] && y1 >= c.view[1] && y0 <= c.view[3]
}

func (c *canvas) visibleAround(p street.Vec, r float64) bool {
	return c.visible(p.X-r, p.Y-r, p.X+r, p.Y+r)
}

// px is the size of one metre on screen.
func (c *canvas) px() float64 { return c.g.zoom }

func (c *canvas) flush() {
	if len(c.is) == 0 {
		return
	}
	c.dst.DrawTriangles(c.vs, c.is, whitePixel, &ebiten.DrawTrianglesOptions{AntiAlias: true})
	c.vs, c.is = c.vs[:0], c.is[:0]
}

func (c *canvas) vertex(p street.Vec, col color.RGBA) {
	x, y := c.g.toScreen(p)
	c.vs = append(c.vs, ebiten.Vertex{DstX: x, DstY: y, SrcX: 0, SrcY: 0,
		ColorR: float32(col.R) / 255, ColorG: float32(col.G) / 255, ColorB: float32(col.B) / 255, ColorA: float32(col.A) / 255})
}

// fan fills a convex polygon.
func (c *canvas) fan(pts []street.Vec, col color.RGBA) {
	if len(pts) < 3 {
		return
	}
	if len(c.vs)+len(pts) > 60000 {
		c.flush()
	}
	base := uint16(len(c.vs))
	for _, p := range pts {
		c.vertex(p, premul(col))
	}
	for i := 1; i+1 < len(pts); i++ {
		c.is = append(c.is, base, base+uint16(i), base+uint16(i+1))
	}
}

// premul converts straight alpha to the premultiplied colour Ebitengine
// expects for vertex colours.
func premul(col color.RGBA) color.RGBA {
	if col.A == 255 {
		return col
	}
	a := float64(col.A) / 255
	return color.RGBA{uint8(float64(col.R) * a), uint8(float64(col.G) * a), uint8(float64(col.B) * a), col.A}
}

// rect fills an axis-aligned world rectangle.
func (c *canvas) rect(x, y, w, h float64, col color.RGBA) {
	c.fan([]street.Vec{{X: x, Y: y}, {X: x + w, Y: y}, {X: x + w, Y: y + h}, {X: x, Y: y + h}}, col)
}

// box fills a rectangle centred at p, length along u and width across it.
func (c *canvas) box(p, u street.Vec, length, width float64, col color.RGBA) {
	r := street.Vec{X: -u.Y, Y: u.X}
	hl, hw := length/2, width/2
	c.fan([]street.Vec{
		add(add(p, scale(u, hl)), scale(r, hw)),
		add(sub(p, scale(u, hl)), scale(r, hw)),
		sub(sub(p, scale(u, hl)), scale(r, hw)),
		sub(add(p, scale(u, hl)), scale(r, hw)),
	}, col)
}

// bevelBox is a box with chamfered corners (vehicle bodies, buildings).
func (c *canvas) bevelBox(p, u street.Vec, length, width, cut float64, col color.RGBA) {
	cut = math.Min(cut, math.Min(length, width)*0.45)
	r := street.Vec{X: -u.Y, Y: u.X}
	hl, hw := length/2, width/2
	at := func(a, b float64) street.Vec { return add(add(p, scale(u, a)), scale(r, b)) }
	c.fan([]street.Vec{
		at(hl, -hw+cut), at(hl, hw-cut), at(hl-cut, hw), at(-hl+cut, hw),
		at(-hl, hw-cut), at(-hl, -hw+cut), at(-hl+cut, -hw), at(hl-cut, -hw),
	}, col)
}

// seg draws a thick line in world units (minPx keeps it visible when small).
func (c *canvas) seg(a, b street.Vec, width, minPx float64, col color.RGBA) {
	l := dist(a, b)
	if l == 0 {
		return
	}
	width = math.Max(width, minPx/c.px())
	c.box(a.Lerp(b, 0.5), scale(sub(b, a), 1/l), l, width, col)
}

func (c *canvas) polyline(pts []street.Vec, width, minPx float64, col color.RGBA) {
	for i := 0; i+1 < len(pts); i++ {
		c.seg(pts[i], pts[i+1], width, minPx, col)
	}
}

// circle fills a disc, tessellated to its on-screen size.
func (c *canvas) circle(p street.Vec, r float64, col color.RGBA) {
	n := int(math.Min(32, math.Max(6, r*c.px()*0.8)))
	pts := make([]street.Vec, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = street.Vec{X: p.X + r*math.Cos(a), Y: p.Y + r*math.Sin(a)}
	}
	c.fan(pts, col)
}

func dist(a, b street.Vec) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

type colorT = color.RGBA

func rgb(r, g, b uint8) color.RGBA     { return color.RGBA{r, g, b, 255} }
func rgba(r, g, b, a uint8) color.RGBA { return color.RGBA{r, g, b, a} }

func tint(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 255}
}

func sub(a, b street.Vec) street.Vec           { return street.Vec{X: a.X - b.X, Y: a.Y - b.Y} }
func add(a, b street.Vec) street.Vec           { return street.Vec{X: a.X + b.X, Y: a.Y + b.Y} }
func scale(a street.Vec, k float64) street.Vec { return street.Vec{X: a.X * k, Y: a.Y * k} }
func unit(a street.Vec) street.Vec {
	l := math.Hypot(a.X, a.Y)
	if l == 0 {
		return street.Vec{}
	}
	return street.Vec{X: a.X / l, Y: a.Y / l}
}

// offset shifts a polyline sideways (positive = right of travel).
func offset(pts []street.Vec, o float64) []street.Vec {
	out := make([]street.Vec, len(pts))
	for i := range pts {
		var d street.Vec
		switch {
		case i == 0:
			d = sub(pts[1], pts[0])
		case i == len(pts)-1:
			d = sub(pts[i], pts[i-1])
		default:
			d = sub(pts[i+1], pts[i-1])
		}
		d = unit(d)
		out[i] = add(pts[i], street.Vec{X: -d.Y * o, Y: d.X * o})
	}
	return out
}
