package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
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
	maxCars int     // longest tram the platforms and turnbacks hold
	limit   string  // what sets maxCars
}

type cfgButton struct {
	r   rect
	act func()
}

type modelItem struct {
	r   rect
	idx int
}

// modelPicker is an open list of the available tram models for a line.
type modelPicker struct {
	line   street.LineID
	anchor rect
	panel  rect
	filter int // 0: all, 1: modern, 2: vintage (<=1950s)
	top    int // scroll offset
	tabs   [3]rect
	items  []modelItem
}

func (g *Game) cycleInfo(id street.LineID) lineCycle {
	if c, ok := g.cycles[id]; ok {
		return c
	}
	if g.cycles == nil {
		g.cycles = map[street.LineID]lineCycle{}
	}
	l := g.net.Lines[id]
	c := lineCycle{seconds: traffic.CycleTime(g.net, l), cap: traffic.MaxFleet(g.net, l)}
	c.maxCars, c.limit = g.net.LineCapacity(l)
	g.cycles[id] = c
	return c
}

// fleet is the number of trams the line will actually run.
func (g *Game) fleet(id street.LineID) int {
	return max(min(g.net.Lines[id].Fleet(), g.cycleInfo(id).cap), 0)
}

// cars is the number of sections the line's trams will run with.
func (g *Game) cars(id street.LineID) int {
	return max(min(g.net.Lines[id].Cars(), g.cycleInfo(id).maxCars), street.MinSections)
}

// configure changes the selected line's fleet size and set length.
func (g *Game) configure(dTrams, dSections int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	cap := g.cycleInfo(l.ID).cap
	trams := min(max(l.Fleet()+dTrams, 1), max(cap, 1))
	if dTrams > 0 && trams == l.Fleet() {
		if cap == 1 {
			g.say("%s", locale.T("全线都是单线、没有会车的地方，只能运行 1 辆电车", "The whole line is single track with nowhere to pass: it can run only 1 tram"))
		} else {
			g.say(locale.T("每条线最多 %d 辆电车", "At most %d trams per line"), street.MaxTrams)
		}
	}
	info := g.cycleInfo(l.ID)
	cur := g.cars(l.ID)
	sections := min(max(cur+dSections, street.MinSections), info.maxCars)
	if dSections > 0 && sections == cur {
		if info.maxCars >= street.MaxSections {
			g.say(locale.T("电车最长 %d 节", "Trams have at most %d sections"), street.MaxSections)
		} else {
			g.say(locale.T("最多 %d 节：受%s限制（在「建设」页加长站台）", "At most %d sections: limited by %s (lengthen platforms on the Build page)"), info.maxCars, info.limit)
		}
	}
	if trams == l.Fleet() && sections == l.Cars() {
		return
	}
	l.Trams, l.Sections = trams, sections
	g.dirty = true // the timetable restarts on the Run page
	g.say(locale.T("%d 路：%d 辆电车 × %d 节编组", "Line %d: trams %d × sections %d"), g.lineIndex(l.ID)+1, trams, sections)
}

// setLineModel assigns tram model idx to the selected line, clamping any
// configured operating speed that exceeds the new vehicle's supported maximum.
func (g *Game) setLineModel(idx int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	n := len(street.TramModels)
	idx = ((idx % n) + n) % n
	m := street.TramModels[idx]
	if l.ModelIndex() == idx {
		return
	}
	old := l.TramModel()
	l.Model = m.Key
	if l.MaxSpeed >= old.MaxSpeedKmh || l.MaxSpeed >= m.MaxSpeedKmh {
		l.MaxSpeed = 0 // run at the new vehicle's supported top speed
	}
	g.dirty = true
	g.cycles = nil
	g.say(locale.T("%d 路：%s（%s）· 车型极速 %d km/h · 加速度 %.2f m/s^2", "Line %d: %s (%s) · tram max %d km/h · accel %.2f m/s^2"),
		g.lineIndex(l.ID)+1, m.Name(), m.Region(), m.MaxSpeedKmh, m.Accel)
}

// configureModel steps the selected line's tram model forward or backward.
func (g *Game) configureModel(d int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	g.setLineModel(l.ModelIndex() + d)
}

// configureSpeed adjusts the selected line's operating maximum speed, bounded
// above by the chosen tram model's own supported top speed. It applies at once
// without restarting the running service.
func (g *Game) configureSpeed(d int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	m := l.TramModel()
	cur := l.OperatingSpeedKmh()
	next := min(max(cur+d*street.OperatingSpeedStepKmh, street.MinOperatingSpeedKmh), m.MaxSpeedKmh)
	if next == cur {
		if d > 0 {
			g.say(locale.T("已达该车型最高速度 %d km/h（%s）", "Already at this tram's top speed of %d km/h (%s)"), m.MaxSpeedKmh, m.Name())
		} else {
			g.say(locale.T("运营最高速度最低 %d km/h", "Operating top speed is at least %d km/h"), street.MinOperatingSpeedKmh)
		}
		return
	}
	if next >= m.MaxSpeedKmh {
		l.MaxSpeed = 0
	} else {
		l.MaxSpeed = next
	}
	g.cycles = nil
	g.say(locale.T("%d 路：运营最高速度 %d km/h（车型上限 %d km/h，立即生效）", "Line %d: operating top speed %d km/h (tram max %d km/h, applies at once)"),
		g.lineIndex(l.ID)+1, next, m.MaxSpeedKmh)
}

// configureService changes how long each passenger takes to board or get
// off, and how many one section serves at once. It applies at once, without
// restarting the service.
func (g *Game) configureService(dBoard, dAlight, dDoors int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	step := func(cur float64, d int) int {
		return min(max(int(cur*10+0.5)+d*street.ServiceStepTenths, street.MinServiceTenths), street.MaxServiceTenths)
	}
	if dBoard != 0 {
		l.BoardTenths = step(l.BoardSeconds(), dBoard)
	}
	if dAlight != 0 {
		l.AlightTenths = step(l.AlightSeconds(), dAlight)
	}
	if dDoors != 0 {
		l.Doors = min(max(l.DoorsPerSection()+dDoors, street.MinDoors), street.MaxDoors)
	}
	g.say(locale.T("%d 路：每人上车 %.1f 秒、下车 %.1f 秒，每节同时 %d 人", "Line %d: %.1f s to board, %.1f s to alight, %d at once per section"),
		g.lineIndex(l.ID)+1, l.BoardSeconds(), l.AlightSeconds(), l.DoorsPerSection())
}

// configureDwell changes the least time trams stand at stops on the way
// and where they turn back. It applies from the next stop on.
func (g *Game) configureDwell(dMid, dEnd int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	if dMid != 0 {
		l.DwellSecs = min(max(l.Dwell()+dMid*street.DwellStep, street.MinDwell), street.MaxDwell)
	}
	if dEnd != 0 {
		l.EndDwellSecs = min(max(l.EndDwell()+dEnd*street.EndDwellStep, street.MinEndDwell), street.MaxEndDwell)
	}
	g.cycles = nil // the planned round trip changes
	g.say(locale.T("%d 路：中途站最少停 %d 秒，终点站最少停 %d 秒（司机换端）", "Line %d: stops last at least %d s, %d s at the ends (the driver changes ends)"),
		g.lineIndex(l.ID)+1, l.Dwell(), l.EndDwell())
}

// configureLoad changes how many passengers each section of the selected
// line carries. It applies immediately without resetting the service; trams
// already carrying more than the new limit let passengers off without boarding
// new ones until their load drops below the new limit.
func (g *Game) configureLoad(d int) {
	l := g.net.Lines[g.selLine]
	if l == nil {
		g.say("%s", locale.T("先点击左上的线路选中它，再配置车辆", "First click a line at the top left to select it, then set its vehicles"))
		return
	}
	n := min(max(l.SectionLoad()+d*street.SectionLoadStep, street.MinSectionLoad), street.MaxSectionLoad)
	if n == l.SectionLoad() {
		g.say(locale.T("每节载客 %d – %d 人", "Each section carries %d–%d passengers"), street.MinSectionLoad, street.MaxSectionLoad)
		return
	}
	l.PerSection = n
	g.say(locale.T("%d 路：每节最多 %d 人，每辆 %d 人（立即生效，超员车辆只下不上）", "Line %d: up to %d per section, %d per tram (applies at once; fuller trams let riders off first)"),
		g.lineIndex(l.ID)+1, n, n*g.cars(l.ID))
}

func fmtDuration(s float64) string {
	sec := int(math.Round(s))
	if sec < 60 {
		return fmt.Sprintf(locale.T("%d 秒", "%d s"), sec)
	}
	if sec%60 == 0 {
		return fmt.Sprintf(locale.T("%d 分", "%d min"), sec/60)
	}
	return fmt.Sprintf(locale.T("%d 分 %d 秒", "%d min %d s"), sec/60, sec%60)
}

func (g *Game) toggleModelMenu(line street.LineID, anchor rect) {
	if g.modelMenu != nil && g.modelMenu.line == line {
		g.modelMenu = nil
		return
	}
	g.modelMenu = &modelPicker{line: line, anchor: anchor}
}

func (g *Game) scrollModelMenu(wy float64) bool {
	if g.modelMenu == nil || wy == 0 {
		return false
	}
	mx, my := ebiten.CursorPosition()
	if !g.modelMenu.panel.has(mx, my) {
		return false
	}
	d := -int(math.Copysign(math.Max(1, math.Abs(wy)), wy))
	g.modelMenu.top = max(g.modelMenu.top+d, 0)
	return true
}

// clickModelMenu handles a left click while the tram model selector is open,
// reporting whether the click was consumed by the menu.
func (g *Game) clickModelMenu(mx, my int) bool {
	mp := g.modelMenu
	if mp == nil {
		return false
	}
	for i, tr := range mp.tabs {
		if tr.has(mx, my) {
			mp.filter, mp.top = i, 0
			return true
		}
	}
	for _, it := range mp.items {
		if it.r.has(mx, my) {
			g.modelMenu = nil
			g.selLine = mp.line
			g.setLineModel(it.idx)
			return true
		}
	}
	if mp.panel.has(mx, my) {
		return true
	}
	g.modelMenu = nil
	return mp.anchor.has(mx, my)
}

// drawModelMenu renders the pop-up list of modern and classic tram models.
func (g *Game) drawModelMenu(dst *ebiten.Image) {
	mp := g.modelMenu
	if mp == nil || g.net.Lines[mp.line] == nil {
		g.modelMenu = nil
		return
	}
	l := g.net.Lines[mp.line]
	chosen := l.ModelIndex()
	var indices []int
	for i, m := range street.TramModels {
		if mp.filter == 1 && m.Vintage || mp.filter == 2 && !m.Vintage {
			continue
		}
		indices = append(indices, i)
	}
	const rowH, maxVis = 26, 12
	vis := min(len(indices), maxVis)
	mp.top = min(mp.top, max(len(indices)-vis, 0))
	pw := float32(516)
	ph := float32(66 + vis*rowH + 10)
	if len(indices) > maxVis {
		ph += 18
	}
	x := mp.anchor.x0
	if x+pw > float32(g.w)-10 {
		x = float32(g.w) - pw - 10
	}
	if x < 10 {
		x = 10
	}
	y := mp.anchor.y1 + 4
	if y+ph > float32(g.h)-10 {
		y = max(float32(hudHeight+6), float32(g.h)-ph-10)
	}
	mp.panel = rect{x, y, x + pw, y + ph}
	vector.FillRect(dst, x, y, pw, ph, rgb(22, 29, 39), false)
	vector.StrokeRect(dst, x, y, pw, ph, 1.5, colAccent, false)
	text(dst, fmt.Sprintf(locale.T("选择电车车型（共 %d 款）", "Choose tram model (%d models)"), len(street.TramModels)),
		float64(x+12), float64(y+8), 14, colHeading)

	mx, my := ebiten.CursorPosition()
	tabLabels := locale.L([]string{"全部", "现代电车", "老式电车 (≤1950)"}, []string{"All", "Modern", "Classic (<=1950)"})
	tx := x + pw - 8
	for i := len(tabLabels) - 1; i >= 0; i-- {
		tw, _ := textui.MeasureText(tabLabels[i], 12)
		bw := float32(tw) + 14
		tx -= bw
		r := rect{tx, y + 6, tx + bw, y + 28}
		mp.tabs[i] = r
		bg, fg := rgb(40, 50, 64), color.Color(color.White)
		if mp.filter == i {
			bg, fg = colAccent, rgb(14, 22, 30)
		} else if r.has(mx, my) {
			bg = rgb(56, 70, 88)
		}
		vector.FillRect(dst, r.x0, r.y0, bw, 22, bg, false)
		text(dst, tabLabels[i], float64(r.x0+7), float64(r.y0+4), 12, fg)
		tx -= 4
	}

	cols := []column{
		{locale.T("车型", "Model"), 228, false},
		{locale.T("地区/年代", "Region / era"), 106, false},
		{locale.T("最高速度", "Max speed"), 84, true},
		{locale.T("加速度", "Accel"), 86, true},
	}
	vector.FillRect(dst, x+6, y+34, pw-12, 22, rgb(38, 48, 62), false)
	cells(dst, cols, x+10, y+38, 12, []string{cols[0].title, cols[1].title, cols[2].title, cols[3].title}, colDim)

	mp.items = mp.items[:0]
	ry := y + 58
	for k := mp.top; k < len(indices) && k < mp.top+vis; k++ {
		i := indices[k]
		m := street.TramModels[i]
		r := rect{x + 6, ry, x + pw - 6, ry + rowH - 2}
		mp.items = append(mp.items, modelItem{r, i})
		bg := colRowA
		if k%2 == 1 {
			bg = colRowB
		}
		if r.has(mx, my) {
			bg = tint(bg, rgb(255, 255, 255), 0.10)
		}
		if i == chosen {
			bg = colRowSel
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		if i == chosen {
			vector.FillRect(dst, r.x0, r.y0, 4, r.y1-r.y0, colAccent, false)
		}
		cells(dst, cols, r.x0+4, r.y0+4, 13, []string{
			m.Name(),
			m.Region(),
			fmt.Sprintf("%d km/h", m.MaxSpeedKmh),
			fmt.Sprintf("%.2f m/s^2", m.Accel),
		}, color.White)
		ry += rowH
	}
	if len(indices) > maxVis {
		text(dst, fmt.Sprintf(locale.T("滚轮查看更多车型（%d–%d / %d）", "Scroll for more models (%d–%d of %d)"),
			mp.top+1, min(mp.top+vis, len(indices)), len(indices)), float64(x+12), float64(ry+2), 11, colDim)
	}
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
	model := l.TramModel()

	const w, pad = 358, 14
	x := float32(g.w) - w - 10
	y := float32(hudHeight + 8)
	h := float32(492)
	if info.cap == 1 {
		h += 20
	}
	g.cfgPanel = rect{x, y, x + w, y + h}
	vector.FillRect(dst, x, y, w, h, colHUD, false)
	vector.FillRect(dst, x, y, w, 4, livery, false)
	tx := float64(x) + pad
	text(dst, fmt.Sprintf(locale.T("%d 路 · 车辆配置", "Line %d · vehicles"), idx+1), tx, float64(y)+10, 16, color.White)

	mx, my := ebiten.CursorPosition()

	// Model selector row: [<] [Model Name ▾] [>] plus spec line underneath.
	ry := y + 36
	text(dst, locale.T("电车车型", "Tram model"), tx, float64(ry)+4, 13, rgb(190, 214, 226))
	bx := x + 98
	prevR := rect{bx, ry, bx + 24, ry + 24}
	nextR := rect{x + w - pad - 24, ry, x + w - pad, ry + 24}
	pickR := rect{bx + 26, ry, nextR.x0 - 2, ry + 24}
	g.cfgBtns = append(g.cfgBtns,
		cfgButton{prevR, func() { g.configureModel(-1) }},
		cfgButton{nextR, func() { g.configureModel(1) }},
		cfgButton{pickR, func() { g.toggleModelMenu(l.ID, pickR) }},
	)
	for _, b := range []struct {
		r   rect
		lbl string
	}{{prevR, "<"}, {nextR, ">"}} {
		bg := rgb(52, 64, 80)
		if b.r.has(mx, my) {
			bg = rgb(70, 88, 108)
		}
		vector.FillRect(dst, b.r.x0, b.r.y0, b.r.x1-b.r.x0, b.r.y1-b.r.y0, bg, false)
		lw, _ := textui.MeasureText(b.lbl, 14)
		text(dst, b.lbl, float64(b.r.x0)+(24-lw)/2, float64(b.r.y0)+3, 14, color.White)
	}
	pbg := rgb(44, 56, 72)
	if g.modelMenu != nil && g.modelMenu.line == l.ID {
		pbg = rgb(44, 70, 88)
	} else if pickR.has(mx, my) {
		pbg = rgb(62, 78, 98)
	}
	vector.FillRect(dst, pickR.x0, pickR.y0, pickR.x1-pickR.x0, pickR.y1-pickR.y0, pbg, false)
	mName := clip(model.ShortName(), 13, pickR.x1-pickR.x0-22)
	mw, _ := textui.MeasureText(mName, 13)
	text(dst, mName, float64(pickR.x0)+float64(pickR.x1-pickR.x0-14-float32(mw))/2, float64(pickR.y0)+4, 13, color.White)
	for k := float32(0); k < 4; k++ {
		vector.FillRect(dst, pickR.x1-13+k, pickR.y0+10+k, 8-2*k, 1, rgb(190, 214, 226), false)
	}
	ry += 26
	spec := fmt.Sprintf(locale.T("%s · 极速 %d km/h · 加速 %.2f m/s^2", "%s · max %d km/h · accel %.2f m/s^2"),
		model.Region(), model.MaxSpeedKmh, model.Accel)
	text(dst, spec, tx, float64(ry), 12, colDim)
	ry += 20

	// Numeric rows: label, [−] value [+].
	row := func(ry float32, label, value string, change func(sign int)) {
		text(dst, label, tx, float64(ry)+3, 13, rgb(190, 214, 226))
		bx := x + 126
		for _, b := range []struct {
			at   float32
			sign int
		}{{bx, -1}, {x + w - pad - 24, 1}} {
			r := rect{b.at, ry, b.at + 24, ry + 22}
			sign := b.sign
			g.cfgBtns = append(g.cfgBtns, cfgButton{r, func() { change(sign) }})
			bg := rgb(52, 64, 80)
			if r.has(mx, my) {
				bg = rgb(70, 88, 108)
			}
			vector.FillRect(dst, r.x0, r.y0, 24, 22, bg, false)
			vector.FillRect(dst, r.x0+6, r.y0+10, 12, 2, color.White, false)
			if b.sign > 0 {
				vector.FillRect(dst, r.x0+11, r.y0+5, 2, 12, color.White, false)
			}
		}
		vw, _ := textui.MeasureText(value, 14)
		mid := (float64(bx+24) + float64(x+w-pad-24)) / 2
		text(dst, value, mid-vw/2, float64(ry)+2, 14, color.White)
	}
	const stepY = 28
	row(ry, locale.T("电车数量  +/-", "Trams  +/-"), fmt.Sprintf(locale.T("%d 辆", "%d"), trams), func(d int) { g.configure(d, 0) })
	ry += stepY
	cars := g.cars(l.ID)
	row(ry, locale.T("列车编组  [ ]", "Sections  [ ]"), fmt.Sprintf(locale.T("%d 节 · %.0f m", "%d sections · %.0f m"), cars, street.SetLength(cars)), func(d int) { g.configure(0, d) })
	ry += stepY
	row(ry, locale.T("运营限速", "Max speed"), fmt.Sprintf("%d / %d km/h", l.OperatingSpeedKmh(), model.MaxSpeedKmh), g.configureSpeed)
	ry += stepY
	row(ry, locale.T("每节载客  < >", "Per section  < >"), fmt.Sprintf(locale.T("%d 人", "%d"), l.SectionLoad()), g.configureLoad)
	ry += stepY
	row(ry, locale.T("上车 每人", "Board each"), fmt.Sprintf(locale.T("%.1f 秒", "%.1f s"), l.BoardSeconds()), func(d int) { g.configureService(d, 0, 0) })
	ry += stepY
	row(ry, locale.T("下车 每人", "Alight each"), fmt.Sprintf(locale.T("%.1f 秒", "%.1f s"), l.AlightSeconds()), func(d int) { g.configureService(0, d, 0) })
	ry += stepY
	row(ry, locale.T("每节同时", "At once/section"), fmt.Sprintf(locale.T("%d 人", "%d"), l.DoorsPerSection()), func(d int) { g.configureService(0, 0, d) })
	ry += stepY
	row(ry, locale.T("中途最少停", "Least stop"), fmt.Sprintf(locale.T("%d 秒", "%d s"), l.Dwell()), func(d int) { g.configureDwell(d, 0) })
	ry += stepY
	row(ry, locale.T("终点最少停", "Least at ends"), fmt.Sprintf(locale.T("%d 秒", "%d s"), l.EndDwell()), func(d int) { g.configureDwell(0, d) })
	ry += 30

	// A top-down sketch of one set: modules, joints, roof style and cabs.
	n := cars
	unitW := float32(w-2*pad) / float32(street.MaxSections)
	sx := x + pad
	roofSketch := rgb(226, 230, 234)
	if model.DarkRoof {
		roofSketch = rgb(58, 64, 72)
	}
	for k := 0; k < n; k++ {
		mx := sx + float32(k)*unitW
		vector.FillRect(dst, mx+1, ry, unitW-4, 14, livery, false)
		vector.FillRect(dst, mx+3, ry+3, unitW-8, 8, roofSketch, false)
		if k+1 < n {
			vector.FillRect(dst, mx+unitW-3, ry+2, 3, 10, rgb(46, 50, 56), false)
		}
	}
	vector.FillRect(dst, sx+float32(n)*unitW-6, ry+2, 3, 10, rgb(26, 40, 52), false) // head cab
	vector.FillRect(dst, sx+1, ry+2, 3, 10, rgb(26, 40, 52), false)                  // tail cab
	ry += 22

	headway := 0.0
	if trams > 0 {
		headway = info.seconds / float64(trams)
	}
	text(dst, fmt.Sprintf(locale.T("周转 ≈ %s · 发车间隔 ≈ %s", "Round trip ≈ %s · headway ≈ %s"), fmtDuration(info.seconds), fmtDuration(headway)), tx, float64(ry), 13, color.White)
	ry += 20
	text(dst, fmt.Sprintf(locale.T("每辆最多 %d 人 · 全线运力 %d 人", "Up to %d per tram · %d on the whole line"), cars*l.SectionLoad(), trams*cars*l.SectionLoad()), tx, float64(ry), 13, color.White)
	ry += 20
	full := float64(cars*l.SectionLoad()) * l.BoardSeconds() / float64(cars*l.DoorsPerSection())
	text(dst, fmt.Sprintf(locale.T("空车坐满约需 %s 上车", "An empty tram fills in about %s"), fmtDuration(full)), tx, float64(ry), 13, color.White)
	ry += 20
	limit := fmt.Sprintf(locale.T("最长可编 %d 节", "Longest set: %d sections"), info.maxCars)
	if info.limit != "" && info.maxCars < street.MaxSections {
		limit += locale.T("（受", " (limited by ") + info.limit + locale.T("限制）", ")")
	}
	text(dst, limit, tx, float64(ry), 12, rgb(190, 214, 226))
	ry += 19
	text(dst, locale.T("限速/载客立即生效；改车型/数量/编组重开运营", "Speed & load apply at once; model/fleet restart on Run"), tx, float64(ry), 11, rgb(150, 164, 176))
	if info.cap == 1 {
		ry += 19
		text(dst, locale.T("! 全线单线、无会车点：只能运行 1 辆", "! Single track, nowhere to pass: 1 tram only"), tx, float64(ry), 12, colHover)
	}
}

// SetFleet gives every line the same vehicles (used by command-line flags).
func (g *Game) SetFleet(trams, sections int) {
	for _, id := range g.net.LineIDs() {
		l := g.net.Lines[id]
		l.Trams = min(max(trams, 1), max(g.cycleInfo(id).cap, 1))
		l.Sections = min(max(sections, street.MinSections), g.cycleInfo(id).maxCars)
	}
	g.rebuild()
}

// SetModel sets every line's tram model by key or 1-based index (used by
// command-line flags and tests).
func (g *Game) SetModel(key string) {
	idx := street.TramModelIndex(key)
	for _, id := range g.net.LineIDs() {
		l := g.net.Lines[id]
		l.Model = street.TramModels[idx].Key
		if l.MaxSpeed > street.TramModels[idx].MaxSpeedKmh {
			l.MaxSpeed = street.TramModels[idx].MaxSpeedKmh
		}
	}
	g.rebuild()
}

// SelectLine selects the i-th line (0-based).
func (g *Game) SelectLine(i int) {
	if ids := g.net.LineIDs(); i >= 0 && i < len(ids) {
		g.selLine = ids[i]
	}
}

// SetPlatforms lengthens every stop's platforms to hold up to the given
// sections where they fit (used by command-line flags).
func (g *Game) SetPlatforms(sections int) {
	for _, id := range g.net.StopIDs() {
		for k := sections; k >= street.MinSections; k-- {
			if g.net.SetStopSections(id, k) == nil {
				break
			}
		}
	}
	g.edited()
	g.rebuild()
}
