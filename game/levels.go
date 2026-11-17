package game

import (
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// The game is drawn in 2D, but the city is five levels deep, as real cities
// are. The ground is the main level, where trams run. Above it is the level
// of viaducts and elevated roads, and above that the rare very high
// structure (stacked interchanges, the tallest towers). Below the ground are
// ordinary tunnels, and below those the rare deep bore (the SR 99 tunnel).
// Every level can be looked at; only the ground can be built on for now.
// The others are ready for metro, elevated light rail and high-speed rail.

func levelHints() []string {
	return locale.L([]string{
		"超高的立交和高楼（≥ 100 m）所在的层，将来用于高铁等超高高架",
		"有轨电车高架段与跨河/跨路高架桥（点地面起点后按 L 切本层可建高架坡道）",
		"主图层：街道铺轨、空地直接铺轨，以及高架/地下出入坡道口",
		"有轨电车地下隧道与地下站（点地面起点后按 L 切本层可建隧道出入口）",
		"极少数超深隧道所在的层，将来用于深层地铁 / 高铁",
	}, []string{
		"the rare stacked interchange and towers of 100 m or more; later for very high viaducts such as high-speed rail",
		"elevated tram viaducts over streets and water (pick a ground start, press L for Elevated, and click to build a ramp)",
		"the main level: street track, direct off-road track on open ground, and ramps/portals",
		"underground tram tunnels and stations (pick a ground start, press L for Underground, and click to build a portal)",
		"the rare very deep tunnel; later for deep metro and high-speed rail",
	})
}

// levelHeight is the level's place in the stack, -2 (deep) to 2 (high).
func levelHeight(i int) int { return groundLevel - i }

// Buildings reaching each upper level: taller than this many metres.
var levelBuildingHeight = map[int]float64{1: 20, 2: 100}

// drawLevelView draws a level other than the ground over the finished
// ground picture: the ground fades out and the level's roads, tunnels and
// tall buildings stand out.
func (g *Game) drawLevelView(dst *ebiten.Image) {
	h := levelHeight(g.level)
	if h == 0 {
		return
	}
	w, ht := float32(g.w), float32(g.h)
	if h > 0 {
		vector.FillRect(dst, 0, 0, w, ht, rgba(236, 240, 245, uint8(140+40*h)), false)
	} else {
		vector.FillRect(dst, 0, 0, w, ht, rgba(10, 14, 22, uint8(185+25*(-h))), false)
	}
	c := newCanvas(dst, g)
	// The street grid, faintly, for orientation.
	faint := rgba(120, 130, 140, 90)
	if h < 0 {
		faint = rgba(150, 170, 190, 55)
	}
	for _, rd := range g.net.Roads {
		if g.roadVisible(c, rd.ID) {
			a, b := g.net.Ends(rd.ID)
			c.seg(a, b, 2*rd.HalfWidth(), 1, faint)
		}
	}
	if g.scene != nil {
		if h > 0 {
			// Buildings tall enough to reach this level.
			for i := range g.scene.buildings {
				b := &g.scene.buildings[i]
				if b.height >= levelBuildingHeight[h] && c.meshVisible(&b.m, 2) {
					c.mesh(&b.m, tint(b.roof, rgb(90, 96, 110), 0.35))
				}
			}
		}
		// Streets on bridges at this level.
		for _, rd := range g.net.Roads {
			if rd.Level == h && g.roadVisible(c, rd.ID) {
				l, r := rd.Sides()
				g.band(c, rd.ID, l+1, r+1, colViaductSide)
				g.band(c, rd.ID, l, r, colRoad)
			}
		}
		for _, lv := range []int{-2, -1, 1, 2} {
			if lv != h {
				continue
			}
			if lv < 0 {
				// Tunnels: the other underground level faint, this one full.
				for i := range g.scene.highways {
					hw := &g.scene.highways[i]
					if hw.level < 0 && hw.level != h && c.highwayVisible(hw, 30) {
						hw.band(c, 0, rgba(90, 110, 130, 90), street.Vec{})
					}
				}
			}
			c.flush()
			g.drawHighways(c, lv)
		}
	}
	if h == 1 || h == -1 {
		g.drawActiveLevelTramways(c, h)
	}
	c.flush()
	if g.lines == nil && g.rename == nil {
		name := levelNames()[g.level] + " · " + levelHints()[g.level]
		tw, _ := textui.MeasureText(name, 14)
		x := float32(g.w)/2 - float32(tw)/2
		vector.FillRect(dst, x-12, hudHeight+40, float32(tw)+24, 26, colHUD, false)
		text(dst, name, float64(x), hudHeight+45, 14, color.White)
	}
}

// drawLevelPicker draws the five levels as a stack of buttons above the
// bottom bar, highest first.
func (g *Game) drawLevelPicker(dst *ebiten.Image, bottom float32) {
	const bh = 26.0
	bw := 92.0
	for _, n := range levelNames() {
		w, _ := textui.MeasureText(n, 13)
		bw = max(bw, w+40)
	}
	x := float64(g.w) - bw - 10
	if g.cfgPanel.x0 >= 0 && g.page == pagePlan {
		x = 16 // the line's settings fill the right side
	}
	y := float64(bottom) - float64(len(levels))*(bh+3) - 24
	vector.FillRect(dst, float32(x-6), float32(y-24), float32(bw+12), float32(len(levels))*(bh+3)+28, colHUD, false)
	text(dst, locale.T("层级 L", "Level L"), x, y-20, 12, colDim)
	for i, name := range levelNames() {
		style := btnPlain
		switch {
		case i == g.level:
			style = btnOn
		case !levelOpen(i):
			style = btnOff
		}
		i := i
		r := rect{float32(x), float32(y), float32(x + bw), float32(y + bh)}
		g.hudBtns = append(g.hudBtns, hudButton{r, func() { g.setLevel(i) }})
		bg, fg := rgb(44, 54, 68), color.Color(color.White)
		switch style {
		case btnOn:
			bg, fg = colAccent, rgb(14, 22, 30)
		case btnOff:
			fg = rgb(150, 160, 172)
		}
		vector.FillRect(dst, r.x0, r.y0, float32(bw), bh, bg, false)
		// A small marker of the height: bars above or below a ground line.
		mx := r.x0 + 10
		vector.FillRect(dst, mx, r.y0+13, 12, 1, fg, false)
		hgt := levelHeight(i)
		for k := 0; k < abs(hgt); k++ {
			dy := float32(-4 * (k + 1))
			if hgt < 0 {
				dy = float32(3 * (k + 1))
			}
			vector.FillRect(dst, mx+2, r.y0+13+dy, 8, 2, fg, false)
		}
		text(dst, name, float64(r.x0)+28, float64(r.y0)+5, 13, fg)
		y += bh + 3
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ShowLevel shows the level at a height from -2 to 2 (used by command-line
// flags).
func (g *Game) ShowLevel(h int) { g.setLevel(groundLevel - min(max(h, -2), 2)) }

// SelectTool selects a Build tool (used by command-line flags).
func (g *Game) SelectTool(i int) {
	if i >= 0 && i < len(toolNames()) {
		g.tool = tool(i)
	}
}
