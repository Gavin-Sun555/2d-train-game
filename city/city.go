// Package city holds the static city the player builds transit on. It is
// engine-free and deterministic for a given seed.
package city

import (
	"math"
	"math/rand/v2"

	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

// TileSize is the edge length of one map tile in metres.
const TileSize = 20.0

type Kind uint8

const (
	Grass Kind = iota
	Road
	Bridge // road deck over water
	Water
	Park
	Building
)

func (k Kind) String() string {
	return [...]string{"grass", "road", "bridge", "water", "park", "building"}[k]
}

type Tile struct {
	Kind    Kind
	Variant uint8 // cosmetic: building height/colour, etc.
}

type Map struct {
	W, H  int // tiles
	Tiles []Tile
	Seed  uint64
	Lots  []Lot
}

func (m *Map) At(tx, ty int) Tile {
	if tx < 0 || ty < 0 || tx >= m.W || ty >= m.H {
		return Tile{Kind: Water} // outside the map is never buildable
	}
	return m.Tiles[ty*m.W+tx]
}

func (m *Map) set(tx, ty int, t Tile) { m.Tiles[ty*m.W+tx] = t }

// TileAt returns the tile under a point in metres.
func (m *Map) TileAt(p sim.Vec2) Tile {
	return m.At(int(math.Floor(p.X/TileSize)), int(math.Floor(p.Y/TileSize)))
}

// SizeMetres is the map extent.
func (m *Map) SizeMetres() (float64, float64) {
	return float64(m.W) * TileSize, float64(m.H) * TileSize
}

const (
	localEvery    = 8  // a 1-tile street every 8 tiles
	arterialEvery = 24 // arterials are 2 tiles wide
)

func isRoadLine(i int) (road, arterial bool) {
	arterial = i%arterialEvery == 0 || i%arterialEvery == 1
	return arterial || i%localEvery == 0, arterial
}

// Generate builds a simple grid city: streets every 8 tiles, two-lane wide
// arterials every 24, a winding river crossed only by arterial bridges,
// some parks, and buildings filling the remaining blocks.
func Generate(seed uint64) *Map {
	const w, h = 98, 61
	m := &Map{W: w, H: h, Tiles: make([]Tile, w*h), Seed: seed}
	rng := rand.New(rand.NewPCG(seed, 0x7261696c))

	riverCenter := func(x int) float64 {
		return 37.5 + 4.5*math.Sin(float64(x)/11+float64(seed%7))
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			water := math.Abs(float64(y)+0.5-riverCenter(x)) < 1.8
			roadX, artX := isRoadLine(x)
			roadY, artY := isRoadLine(y)
			switch {
			case water && (artX && !roadY || artY && !roadX):
				m.set(x, y, Tile{Kind: Bridge})
			case water:
				m.set(x, y, Tile{Kind: Water})
			case roadX || roadY:
				m.set(x, y, Tile{Kind: Road})
			}
		}
	}

	// Fill each block: a few become parks, the rest buildings with gaps.
	for by := 0; by*localEvery < h; by++ {
		for bx := 0; bx*localEvery < w; bx++ {
			park := rng.IntN(9) == 0
			for y := by * localEvery; y < (by+1)*localEvery && y < h; y++ {
				for x := bx * localEvery; x < (bx+1)*localEvery && x < w; x++ {
					if m.At(x, y).Kind != Grass {
						continue
					}
					switch {
					case park:
						m.set(x, y, Tile{Kind: Park, Variant: uint8(rng.IntN(4))})
					case rng.IntN(7) != 0:
						m.set(x, y, Tile{Kind: Building, Variant: uint8(rng.IntN(6))})
					}
				}
			}
		}
	}
	m.buildLots(rng)
	return m
}

// RoadCentre returns the centre of a straight street line given its tile
// index, for placing demo track along streets. Arterials (2 tiles wide)
// return the boundary between their two tiles.
func RoadCentre(i int) float64 {
	if _, art := isRoadLine(i); art && i%arterialEvery == 0 {
		return float64(i+1) * TileSize
	}
	return (float64(i) + 0.5) * TileSize
}

// StreetLine is one straight street corridor running across the map.
type StreetLine struct {
	Tile     int     // first tile index of the corridor
	Centre   float64 // centre line, metres
	Arterial bool    // 2-tile corridor
}

// StreetLines lists the street corridors along an axis with n tiles.
func StreetLines(n int) []StreetLine {
	var out []StreetLine
	for i := 0; i < n; i++ {
		road, art := isRoadLine(i)
		if !road || (art && i%arterialEvery == 1) {
			continue
		}
		if art && i+1 >= n {
			art = false // a half arterial at the map edge is just a street
		}
		out = append(out, StreetLine{Tile: i, Centre: RoadCentre(i), Arterial: art})
	}
	return out
}
