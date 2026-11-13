package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// lineCycle caches what the Plan page shows about a line's timetable.
type lineCycle struct {
	seconds float64 // estimated round trip
	cap     int     // most trams the track lets the line run
}

type cfgButton struct {
	r                 rect
	dTrams, dSections int
}

func (g *Game) cycleInfo(id street.LineID) lineCycle {
	if c, ok := g.cycles[id]; ok {
		return c
	}
	if g.cycles == nil {
		g.cycles = map[street.LineID]lineCycle{}
	}
	l := g.net.Lines[id]
	c := lineCycle{traffic.CycleTime(g.net, l), traffic.MaxFleet(g.net, l)}
	g.cycles[id] = c
	return c
}

// fleet is the number of trams the line will actually run.
func (g *Game) fleet(id street.LineID) int {
	return max(min(g.net.Lines[id].Fleet(), g.cycleInfo(id).cap), 0)
}

// configure changes the selected line's fleet size and set length.
func (g *Game) configure(dTrams, dSections int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("先点击左上的线路选中它，再配置车辆")
		return
	}
	cap := g.cycleInfo(l.ID).cap
	trams := min(max(l.Fleet()+dTrams, 1), max(cap, 1))
	if dTrams > 0 && trams == l.Fleet() {
		if cap == 1 {
			g.say("全线都是单线、没有会车的地方，只能运行 1 辆电车")
		} else {
			g.say("每条线最多 %d 辆电车", street.MaxTrams)
		}
	}
	sections := min(max(l.Cars()+dSections, street.MinSections), street.MaxSections)
	if dSections > 0 && sections == l.Cars() {
		g.say("站台最长容纳 %d 节编组（%.0f m）", street.MaxSections, street.TramLength)
	}
	if trams == l.Fleet() && sections == l.Cars() {
		return
	}
	l.Trams, l.Sections = trams, sections
	g.dirty = true // the timetable restarts on the Run page
	g.say("%d 路：%d 辆电车 × %d 节编组", g.lineIndex(l.ID)+1, trams, sections)
}

func fmtDuration(s float64) string {
	sec := int(math.Round(s))
	if sec < 60 {
		return fmt.Sprintf("%d 秒", sec)
	}
	if sec%60 == 0 {
		return fmt.Sprintf("%d 分", sec/60)
	}
	return fmt.Sprintf("%d 分 %d 秒", sec/60, sec%60)
}

// drawLineConfig is the Plan page's panel for the selected line's vehicles.
func (g *Game) drawLineConfig(dst *ebiten.Image) {
	g.cfgBtns, g.cfgPanel = g.cfgBtns[:0], rect{-1, -1, -1, -1}
	l := g.net.Lines[g.selLine]
	if g.page != pagePlan || l == nil {
		return
	}
	idx := g.lineIndex(l.ID)
	livery := lineColors[idx%len(lineColors)]
	info := g.cycleInfo(l.ID)
	trams := g.fleet(l.ID)

	const w, pad = 316, 14
	x := float32(g.w) - w - 10
	y := float32(hudHeight + 10)
	h := float32(214)
	if info.cap == 1 {
		h += 22
	}
	g.cfgPanel = rect{x, y, x + w, y + h}
	vector.FillRect(dst, x, y, w, h, colHUD, false)
	vector.FillRect(dst, x, y, w, 4, livery, false)
	tx := float64(x) + pad
	text(dst, fmt.Sprintf("%d 路 · 车辆配置", idx+1), tx, float64(y)+12, 16, color.White)

	// Each row: label, [−] value [+].
	row := func(ry float32, label, value string, dt, ds int) {
		text(dst, label, tx, float64(ry)+4, 14, rgb(190, 214, 226))
		bx := x + 118
		for _, b := range []struct {
			at   float32
			sign int
		}{{bx, -1}, {x + w - pad - 26, 1}} {
			r := rect{b.at, ry, b.at + 26, ry + 24}
			g.cfgBtns = append(g.cfgBtns, cfgButton{r, dt * b.sign, ds * b.sign})
			mx, my := ebiten.CursorPosition()
			bg := rgb(52, 64, 80)
			if r.has(mx, my) {
				bg = rgb(70, 88, 108)
			}
			vector.FillRect(dst, r.x0, r.y0, 26, 24, bg, false)
			vector.FillRect(dst, r.x0+7, r.y0+11, 12, 2, color.White, false)
			if b.sign > 0 {
				vector.FillRect(dst, r.x0+12, r.y0+6, 2, 12, color.White, false)
			}
		}
		vw, _ := textui.MeasureText(value, 15)
		mid := (float64(bx+26) + float64(x+w-pad-26)) / 2
		text(dst, value, mid-vw/2, float64(ry)+3, 15, color.White)
	}
	ry := y + 44
	row(ry, "电车数量  +/-", fmt.Sprintf("%d 辆", trams), 1, 0)
	ry += 34
	row(ry, "列车编组  [ ]", fmt.Sprintf("%d 节 · %.0f m", l.Cars(), street.SetLength(l.Cars())), 0, 1)
	ry += 36

	// A top-down sketch of one set: modules, joints and the cabs.
	n := l.Cars()
	unit := float32(w-2*pad) / float32(street.MaxSections)
	sx := x + pad
	for k := 0; k < n; k++ {
		mx := sx + float32(k)*unit
		vector.FillRect(dst, mx+1, ry, unit-4, 14, livery, false)
		vector.FillRect(dst, mx+3, ry+3, unit-8, 8, rgb(226, 230, 234), false)
		if k+1 < n {
			vector.FillRect(dst, mx+unit-3, ry+2, 3, 10, rgb(46, 50, 56), false)
		}
	}
	vector.FillRect(dst, sx+float32(n)*unit-6, ry+2, 3, 10, rgb(26, 40, 52), false) // head cab
	ry += 26

	headway := 0.0
	if trams > 0 {
		headway = info.seconds / float64(trams)
	}
	text(dst, fmt.Sprintf("周转时间 ≈ %s", fmtDuration(info.seconds)), tx, float64(ry), 14, color.White)
	ry += 22
	text(dst, fmt.Sprintf("发车间隔 ≈ %s", fmtDuration(headway)), tx, float64(ry), 14, color.White)
	ry += 22
	text(dst, "修改后在「运行」页重新开始运营", tx, float64(ry), 12, rgb(150, 164, 176))
	if info.cap == 1 {
		ry += 22
		text(dst, "! 全线单线、无会车点：只能运行 1 辆", tx, float64(ry), 13, colHover)
	}
}

// SetFleet gives every line the same vehicles (used by command-line flags).
func (g *Game) SetFleet(trams, sections int) {
	for _, id := range g.net.LineIDs() {
		l := g.net.Lines[id]
		l.Trams = min(max(trams, 1), max(g.cycleInfo(id).cap, 1))
		l.Sections = min(max(sections, street.MinSections), street.MaxSections)
	}
	g.rebuild()
}

// SelectLine selects the i-th line (0-based).
func (g *Game) SelectLine(i int) {
	if ids := g.net.LineIDs(); i >= 0 && i < len(ids) {
		g.selLine = ids[i]
	}
}
