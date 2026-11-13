package city

import "math/rand/v2"

// LotStyle is what stands on a lot; the renderer draws each style.
type LotStyle uint8

const (
	LotTower LotStyle = iota // office tower with podium
	LotShops                 // row of shops / low commercial
	LotHomes                 // detached houses with gardens
	LotCourt                 // perimeter block around a courtyard
	LotPlaza                 // paved square with a pavilion
	LotPark                  // lawn, paths and trees
)

// Lot is one plot inside a city block, in metres.
type Lot struct {
	X, Y, W, H float64
	Style      LotStyle
	Variant    int
}

// runs returns the [start, end) tile ranges between street corridors.
func runs(n int) [][2]int {
	var out [][2]int
	start := -1
	for i := 0; i <= n; i++ {
		road := i == n
		if i < n {
			road, _ = isRoadLine(i)
		}
		switch {
		case !road && start < 0:
			start = i
		case road && start >= 0:
			out = append(out, [2]int{start, i})
			start = -1
		}
	}
	return out
}

// buildLots splits every block into up to 2×2 lots, skipping any lot
// touched by water. Park blocks become park lots.
func (m *Map) buildLots(rng *rand.Rand) {
	for _, ry := range runs(m.H) {
		for _, rx := range runs(m.W) {
			cols, rows := splitRun(rx), splitRun(ry)
			for _, cy := range rows {
				for _, cx := range cols {
					water, park := false, true
					for y := cy[0]; y < cy[1]; y++ {
						for x := cx[0]; x < cx[1]; x++ {
							k := m.At(x, y).Kind
							water = water || k == Water || k == Bridge
							park = park && k == Park
						}
					}
					if water {
						continue
					}
					const inset = 2.0
					lot := Lot{
						X: float64(cx[0])*TileSize + inset, Y: float64(cy[0])*TileSize + inset,
						W: float64(cx[1]-cx[0])*TileSize - 2*inset, H: float64(cy[1]-cy[0])*TileSize - 2*inset,
						Variant: rng.IntN(1 << 16),
					}
					switch {
					case park:
						lot.Style = LotPark
					default:
						lot.Style = []LotStyle{LotTower, LotShops, LotShops, LotHomes, LotHomes, LotCourt, LotCourt, LotPlaza}[rng.IntN(8)]
					}
					m.Lots = append(m.Lots, lot)
				}
			}
		}
	}
}

func splitRun(r [2]int) [][2]int {
	n := r[1] - r[0]
	if n < 6 {
		return [][2]int{r}
	}
	mid := r[0] + n/2
	return [][2]int{{r[0], mid}, {mid, r[1]}}
}
