package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/city"
)

// mapPxPerMetre is the resolution of the pre-rendered city background.
const mapPxPerMetre = 2

var (
	colGrass     = color.RGBA{0xdc, 0xe6, 0xc8, 0xff}
	colPark      = color.RGBA{0xb4, 0xd4, 0x96, 0xff}
	colTree      = color.RGBA{0x8c, 0xb8, 0x72, 0xff}
	colRoad      = color.RGBA{0xfb, 0xfa, 0xf6, 0xff}
	colRoadEdge  = color.RGBA{0xd2, 0xcc, 0xc0, 0xff}
	colWater     = color.RGBA{0xa6, 0xcb, 0xe8, 0xff}
	colBridge    = color.RGBA{0xe9, 0xe4, 0xda, 0xff}
	colLot       = color.RGBA{0xe8, 0xe3, 0xd9, 0xff}
	buildingCols = []color.RGBA{
		{0xcf, 0xc6, 0xb8, 0xff},
		{0xc4, 0xbb, 0xad, 0xff},
		{0xd8, 0xcd, 0xbd, 0xff},
		{0xbd, 0xb6, 0xab, 0xff},
		{0xd3, 0xc2, 0xb0, 0xff},
		{0xc9, 0xc1, 0xb6, 0xff},
	}
)

// renderCity draws the static map once into an offscreen image.
func renderCity(m *city.Map) *ebiten.Image {
	wm, hm := m.SizeMetres()
	img := ebiten.NewImage(int(wm)*mapPxPerMetre, int(hm)*mapPxPerMetre)
	const s = city.TileSize * mapPxPerMetre
	img.Fill(colGrass)
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			t := m.At(x, y)
			px, py := float32(x)*s, float32(y)*s
			switch t.Kind {
			case city.Road:
				vector.FillRect(img, px, py, s, s, colRoad, false)
			case city.Bridge:
				vector.FillRect(img, px, py, s, s, colBridge, false)
			case city.Water:
				vector.FillRect(img, px, py, s, s, colWater, false)
			case city.Park:
				vector.FillRect(img, px, py, s, s, colPark, false)
				if t.Variant == 0 {
					vector.FillCircle(img, px+s/2, py+s/2, s*0.3, colTree, true)
				}
			case city.Building:
				vector.FillRect(img, px, py, s, s, colLot, false)
				in := float32(2 * mapPxPerMetre)
				vector.FillRect(img, px+in, py+in, s-2*in, s-2*in, buildingCols[int(t.Variant)%len(buildingCols)], false)
			}
		}
	}
	// Kerbs: a thin line wherever a road meets something that is not road.
	isRoad := func(k city.Kind) bool { return k == city.Road || k == city.Bridge }
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if !isRoad(m.At(x, y).Kind) {
				continue
			}
			px, py := float32(x)*s, float32(y)*s
			if !isRoad(m.At(x, y-1).Kind) {
				vector.FillRect(img, px, py, s, 1.5, colRoadEdge, false)
			}
			if !isRoad(m.At(x, y+1).Kind) {
				vector.FillRect(img, px, py+s-1.5, s, 1.5, colRoadEdge, false)
			}
			if !isRoad(m.At(x-1, y).Kind) {
				vector.FillRect(img, px, py, 1.5, s, colRoadEdge, false)
			}
			if !isRoad(m.At(x+1, y).Kind) {
				vector.FillRect(img, px+s-1.5, py, 1.5, s, colRoadEdge, false)
			}
		}
	}
	return img
}
