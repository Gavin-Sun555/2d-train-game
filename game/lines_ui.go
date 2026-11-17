package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/platforminput"
	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// Line management, as in the bus game: every line's operating figures in
// one table, and for the chosen line its vehicles (with −/+ to change
// them), each stop's passengers and each tram. Figures count from the
// moment the service last (re)started on the Run page. The service keeps
// running while the window is open.

type linePanel struct {
	sel              street.LineID
	lineTop          int // first row shown of each list
	stopTop, tramTop int
	btns             []hudButton
	panel            rect
	lineRows         []lineRow
	stopRows         []stopRow
	tramRows         []tramRow
	lineArea         rect
	stopArea         rect
	tramArea         rect
}

type lineRow struct {
	r  rect
	id street.LineID
}

type stopRow struct {
	r  rect
	id street.StopID
}

type tramRow struct {
	r  rect
	id int
}

func (g *Game) toggleLinePanel() {
	if g.lines != nil {
		g.closeLinePanel()
		return
	}
	p := &linePanel{sel: g.selLine}
	if g.net.Lines[p.sel] == nil {
		p.sel = -1
		if ids := g.net.LineIDs(); len(ids) > 0 {
			p.sel = ids[0]
		}
		if g.follow != 0 {
			if v := g.sim.Find(g.follow); v != nil {
				p.sel = v.Line
			}
		}
	}
	g.lines = p
}

// OpenLinePanel opens line management (used by command-line flags).
func (g *Game) OpenLinePanel() {
	if g.lines == nil {
		g.toggleLinePanel()
	}
}

// closeLinePanel closes the window; vehicle changes made in it restart the
// service on the Run page.
func (g *Game) closeLinePanel() {
	g.lines, g.modelMenu = nil, nil
	if g.page == pageRun && g.dirty {
		g.rebuild()
		g.say("%s", locale.T("已按新的车辆配置重新开始运营", "Service restarted with the new vehicles"))
	}
}

func (g *Game) updateLinePanel() {
	p := g.lines
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if g.modelMenu != nil {
			g.modelMenu = nil
			return
		}
		g.closeLinePanel()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) && g.page == pageRun {
		g.paused = !g.paused
	}
	// Keys for the chosen line, as on the Plan page.
	if g.net.Lines[p.sel] != nil {
		g.selLine = p.sel
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyEqual), inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
			g.configure(1, 0)
		case inpututil.IsKeyJustPressed(ebiten.KeyMinus), inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
			g.configure(-1, 0)
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
			g.configure(0, 1)
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
			g.configure(0, -1)
		case inpututil.IsKeyJustPressed(ebiten.KeyPeriod):
			g.configureLoad(1)
		case inpututil.IsKeyJustPressed(ebiten.KeyComma):
			g.configureLoad(-1)
		}
	}
	ids := g.net.LineIDs()
	for i, id := range ids {
		if inpututil.IsKeyJustPressed(ebiten.Key1+ebiten.Key(i)) && i < 9 {
			p.choose(id)
		}
	}
	mx, my := ebiten.CursorPosition()
	platforminput.TakePinchDelta()
	if wy := platforminput.TakeMouseWheelY(); wy != 0 {
		if g.scrollModelMenu(wy) {
			return
		}
		d := -int(math.Copysign(math.Max(1, math.Abs(wy)), wy))
		switch {
		case p.lineArea.has(mx, my):
			p.lineTop = max(p.lineTop+d, 0)
		case p.stopArea.has(mx, my):
			p.stopTop = max(p.stopTop+d, 0)
		case p.tramArea.has(mx, my):
			p.tramTop = max(p.tramTop+d, 0)
		}
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	if g.clickModelMenu(mx, my) {
		return
	}
	for _, b := range p.btns {
		if b.r.has(mx, my) {
			b.act()
			return
		}
	}
	for _, r := range p.lineRows {
		if r.r.has(mx, my) {
			p.choose(r.id)
			return
		}
	}
	for _, r := range p.stopRows {
		if r.r.has(mx, my) {
			pos := g.net.StopPos(g.net.Stops[r.id])
			g.follow, g.camX, g.camY = 0, pos.X, pos.Y
			g.zoom = math.Max(g.zoom, 1.5)
			g.closeLinePanel()
			return
		}
	}
	for _, r := range p.tramRows {
		if r.r.has(mx, my) {
			g.closeLinePanel()
			if g.sim.Find(r.id) != nil {
				g.follow = r.id
			}
			return
		}
	}
	if !p.panel.has(mx, my) {
		g.closeLinePanel()
	}
}

func (p *linePanel) choose(id street.LineID) {
	if id != p.sel {
		p.sel, p.stopTop, p.tramTop = id, 0, 0
	}
}

// --- drawing ----------------------------------------------------------------

var (
	colPanel   = rgb(24, 31, 41)
	colRowA    = rgb(30, 38, 50)
	colRowB    = rgb(34, 43, 56)
	colRowSel  = rgb(44, 70, 88)
	colDim     = rgb(150, 164, 176)
	colHeading = rgb(190, 214, 226)
)

type column struct {
	title string
	w     float32
	right bool // right-aligned (numbers)
}

// clip shortens s with an ellipsis to fit w pixels.
func clip(s string, size float64, w float32) string {
	if tw, _ := textui.MeasureText(s, size); float32(tw) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if tw, _ := textui.MeasureText(string(r)+"…", size); float32(tw) <= w {
			break
		}
	}
	return string(r) + "…"
}

// cells draws one row of a table.
func cells(dst *ebiten.Image, cols []column, x, y float32, size float64, vals []string, col color.Color) {
	for i, c := range cols {
		if i >= len(vals) {
			break
		}
		v := clip(vals[i], size, c.w-10)
		tx := float64(x) + 6
		if c.right {
			w, _ := textui.MeasureText(v, size)
			tx = float64(x+c.w) - 6 - w
		}
		text(dst, v, tx, float64(y), size, col)
		x += c.w
	}
}

// fit widens the flexible column (w 0) to fill the table width.
func fit(cols []column, width float32) []column {
	used := float32(0)
	for _, c := range cols {
		used += c.w
	}
	out := append([]column(nil), cols...)
	// Every column is at least as wide as its title.
	used = 0
	for i := range out {
		if out[i].w > 0 {
			tw, _ := textui.MeasureText(out[i].title, 13)
			out[i].w = max(out[i].w, float32(tw)+14)
		}
		used += out[i].w
	}
	for i := range out {
		if out[i].w == 0 {
			out[i].w = max(width-used, 80)
		}
	}
	return out
}

func clock(s float64) string {
	if s <= 0 {
		return "—"
	}
	n := int(math.Round(s))
	if n >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", n/3600, n/60%60, n%60)
	}
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}

func km(m float64) string {
	if m < 1000 {
		return fmt.Sprintf("%.0f m", m)
	}
	return fmt.Sprintf("%.2f km", m/1000)
}

func kmh(ms float64) string {
	if ms <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f km/h", ms*3.6)
}

func dash(n int, live bool) string {
	if !live {
		return "—"
	}
	return fmt.Sprint(n)
}

// live reports whether the service's figures belong to the lines as they
// are now.
func (g *Game) live() bool { return !g.dirty && g.sim.Seconds() > 0 }

func (g *Game) drawLinePanel(dst *ebiten.Image) {
	p := g.lines
	if p == nil {
		return
	}
	p.btns, p.lineRows, p.stopRows, p.tramRows = p.btns[:0], p.lineRows[:0], p.stopRows[:0], p.tramRows[:0]
	pw := float32(min(g.w-32, 1180))
	ph := float32(min(g.h-hudHeight-24, 700))
	x0, y0 := (float32(g.w)-pw)/2, float32(hudHeight+12)
	p.panel = rect{x0, y0, x0 + pw, y0 + ph}
	vector.FillRect(dst, x0, y0, pw, ph, colPanel, false)
	vector.StrokeRect(dst, x0, y0, pw, ph, 2, colAccent, false)
	tx := x0 + 20
	text(dst, locale.T("线路管理 · 运营数据", "Lines · operating data"), float64(tx), float64(y0+14), 20, color.White)
	buttonAt(dst, &p.btns, float64(x0+pw-56), float64(y0+12), 26, locale.T("关闭", "Close"), btnPlain, g.closeLinePanel)

	live := g.live()
	trams, onBoard := 0, 0
	for _, v := range g.sim.Vehicles {
		if v.Kind == traffic.TramKind {
			trams++
			onBoard += v.Load()
		}
	}
	var status string
	switch {
	case g.dirty:
		status = locale.T("线路或车辆已修改：在「运行」页重新开始运营后统计（关闭本窗口时生效）", "Lines or vehicles changed: figures start again when the service restarts on the Run page (on closing this window)")
	case !live:
		status = locale.T("运营数据在「运行」页统计；这里也可以调整各线路的车辆", "Figures are counted on the Run page; you can also set each line's vehicles here")
	default:
		status = fmt.Sprintf(locale.T("运营 %s · 电车 %d 辆在线 · 在车 %d 人 · 候车 %d 人 · 已送达 %d 人（自本次开始运营起）", "Running %s · trams in service %d · on board %d · waiting %d · delivered %d (since the service started)"),
			clock(g.sim.Seconds()), trams, onBoard, g.sim.TotalWaiting(), g.sim.Passengers.Delivered)
	}
	text(dst, status, float64(tx), float64(y0+44), 13, colHeading)

	ids := g.net.LineIDs()
	if len(ids) == 0 {
		text(dst, locale.T("还没有线路：在「规划」页依次点击车站，回车建立线路。", "No lines yet: on the Plan page click stops in order and press Enter."), float64(tx), float64(y0+90), 15, color.White)
		return
	}
	if g.net.Lines[p.sel] == nil {
		p.sel = ids[0]
	}

	// All lines.
	tw := pw - 40
	cols := fit([]column{
		{locale.T("线路", "Line"), 64, false}, {locale.T("起讫", "From – to"), 0, false}, {locale.T("站", "Stops"), 36, true}, {locale.T("长度", "Length"), 76, true}, {locale.T("电车", "Trams"), 76, true},
		{locale.T("每辆", "Places"), 56, true}, {locale.T("单程", "Trips"), 50, true}, {locale.T("平均单程", "Avg trip"), 76, true}, {locale.T("运营均速", "Avg speed"), 86, true}, {locale.T("发车间隔", "Headway"), 72, true},
		{locale.T("上车", "On"), 60, true}, {locale.T("送达", "Delivered"), 60, true}, {locale.T("在车", "Aboard"), 52, true}, {locale.T("满载率", "Load"), 62, true},
	}, tw)
	y := y0 + 72
	vector.FillRect(dst, tx, y, tw, 24, rgb(40, 50, 64), false)
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.title
	}
	cells(dst, cols, tx, y+5, 13, titles, colDim)
	y += 26
	const rowH = 26
	maxRows := 5
	p.lineTop = min(p.lineTop, max(len(ids)-maxRows, 0))
	p.lineArea = rect{tx, y, tx + tw, y + rowH*float32(maxRows)}
	mx, my := ebiten.CursorPosition()
	for i := p.lineTop; i < len(ids) && i < p.lineTop+maxRows; i++ {
		id := ids[i]
		l := g.net.Lines[id]
		r := rect{tx, y, tx + tw, y + rowH - 2}
		p.lineRows = append(p.lineRows, lineRow{r, id})
		bg := colRowA
		if i%2 == 1 {
			bg = colRowB
		}
		if r.has(mx, my) {
			bg = tint(bg, rgb(255, 255, 255), 0.06)
		}
		if id == p.sel {
			bg = colRowSel
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		vector.FillRect(dst, r.x0, r.y0, 4, r.y1-r.y0, lineColors[i%len(lineColors)], false)
		st := g.sim.LineStats(id)
		load := 0
		for _, v := range g.sim.Vehicles {
			if v.Kind == traffic.TramKind && v.Line == id {
				load += v.Load()
			}
		}
		// The two stops where trams turn back (for a line drawn there and
		// back again, not its first and last listed stops).
		first, last := g.net.Stops[l.Stops[0]].Name, g.net.Stops[l.Stops[len(l.Stops)-1]].Name
		if ends := l.Ends(); len(ends) >= 2 {
			first, last = g.net.Stops[ends[0]].Name, g.net.Stops[ends[len(ends)-1]].Name
		}
		vals := []string{fmt.Sprintf(locale.T("%d 路", "Line %d"), i+1), first + " – " + last, fmt.Sprint(len(l.Stops)),
			km(traffic.LineLength(g.net, l)), fmt.Sprintf(locale.T("%d × %d节", "%d × %d"), g.fleet(id), g.cars(id)),
			fmt.Sprint(g.cars(id) * l.SectionLoad()), "—", "—", "—", "—", "—", "—", "—", "—"}
		if live {
			vals[6] = fmt.Sprint(st.Trips)
			vals[7] = clock(st.AvgTrip())
			vals[8] = kmh(st.Speed())
			vals[9] = clock(st.Headway())
			vals[10] = fmt.Sprint(st.Boarded)
			vals[11] = fmt.Sprint(st.Delivered)
			vals[12] = fmt.Sprint(load)
			vals[13] = fmt.Sprintf("%.0f%%", 100*st.LoadFactor())
		}
		cells(dst, cols, tx+4, y+5, 13, vals, color.White)
		y += rowH
	}
	if len(ids) > maxRows {
		text(dst, fmt.Sprintf(locale.T("滚轮查看其余线路（%d–%d / %d）", "Scroll for more lines (%d–%d of %d)"), p.lineTop+1, min(p.lineTop+maxRows, len(ids)), len(ids)), float64(tx), float64(y+2), 12, colDim)
		y += 18
	}

	// The chosen line.
	y += 12
	l := g.net.Lines[p.sel]
	model := l.TramModel()
	idx := g.lineIndex(p.sel)
	livery := lineColors[idx%len(lineColors)]
	vector.FillRect(dst, tx, y, tw, 2, livery, false)
	y += 12
	text(dst, fmt.Sprintf(locale.T("%d 路", "Line %d"), idx+1), float64(tx), float64(y+3), 17, color.White)
	nw, _ := textui.MeasureText(fmt.Sprintf(locale.T("%d 路", "Line %d"), idx+1), 17)
	bx := float64(tx) + nw + 16
	step := func(label, value string, dec, inc func()) {
		w, _ := textui.MeasureText(label, 13)
		text(dst, label, bx, float64(y+6), 13, colDim)
		bx = buttonAt(dst, &p.btns, bx+w+5, float64(y+1), 24, "-", btnPlain, dec) + 5
		vw, _ := textui.MeasureText(value, 14)
		text(dst, value, bx, float64(y+5), 14, color.White)
		bx = buttonAt(dst, &p.btns, bx+vw+5, float64(y+1), 24, "+", btnPlain, inc) + 14
	}
	sel := func(f func()) func() { return func() { g.selLine = p.sel; f() } }
	// Model selector: label [<] [Model ▾] [>]
	mlbl := locale.T("车型", "Tram")
	mlw, _ := textui.MeasureText(mlbl, 13)
	text(dst, mlbl, bx, float64(y+6), 13, colDim)
	bx = buttonAt(dst, &p.btns, bx+mlw+5, float64(y+1), 24, "<", btnPlain, sel(func() { g.configureModel(-1) })) + 3
	var pickAnchor rect
	mStyle := btnPlain
	if g.modelMenu != nil && g.modelMenu.line == p.sel {
		mStyle = btnOn
	}
	bx = buttonAt(dst, &p.btns, bx, float64(y+1), 24, fmt.Sprintf("%s · %.2f m/s^2   ", model.ShortName(), model.Accel), mStyle, func() {
		g.toggleModelMenu(p.sel, pickAnchor)
	})
	pickAnchor = p.btns[len(p.btns)-1].r
	for k := float32(0); k < 4; k++ {
		vector.FillRect(dst, pickAnchor.x1-13+k, pickAnchor.y0+10+k, 8-2*k, 1, rgb(190, 214, 226), false)
	}
	bx = buttonAt(dst, &p.btns, bx+3, float64(y+1), 24, ">", btnPlain, sel(func() { g.configureModel(1) })) + 14

	step(locale.T("电车 +/-", "Trams +/-"), fmt.Sprintf(locale.T("%d 辆", "%d"), g.fleet(p.sel)), sel(func() { g.configure(-1, 0) }), sel(func() { g.configure(1, 0) }))
	step(locale.T("编组 [ ]", "Sections [ ]"), fmt.Sprintf(locale.T("%d 节", "%d"), g.cars(p.sel)), sel(func() { g.configure(0, -1) }), sel(func() { g.configure(0, 1) }))
	info := g.cycleInfo(p.sel)
	plan := fmt.Sprintf(locale.T("周转 ≈ %s · 间隔 ≈ %s", "Round trip ≈ %s · headway ≈ %s"), fmtDuration(info.seconds), fmtDuration(info.seconds/math.Max(1, float64(g.fleet(p.sel)))))
	text(dst, clip(plan, 13, x0+pw-20-float32(bx)), bx, float64(y+6), 13, colDim)
	// Second row: operating speed, load, boarding and alighting — all apply at once without resetting!
	y += 32
	bx = float64(tx) + nw + 16
	step(locale.T("运营限速", "Max speed"), fmt.Sprintf("%d/%d km/h", l.OperatingSpeedKmh(), model.MaxSpeedKmh), sel(func() { g.configureSpeed(-1) }), sel(func() { g.configureSpeed(1) }))
	step(locale.T("每节载客 < >", "Per section < >"), fmt.Sprintf(locale.T("%d 人", "%d"), l.SectionLoad()), sel(func() { g.configureLoad(-1) }), sel(func() { g.configureLoad(1) }))
	step(locale.T("上车", "Board"), fmt.Sprintf(locale.T("%.1f 秒", "%.1f s"), l.BoardSeconds()), sel(func() { g.configureService(-1, 0, 0) }), sel(func() { g.configureService(1, 0, 0) }))
	step(locale.T("下车", "Alight"), fmt.Sprintf(locale.T("%.1f 秒", "%.1f s"), l.AlightSeconds()), sel(func() { g.configureService(0, -1, 0) }), sel(func() { g.configureService(0, 1, 0) }))
	step(locale.T("每节同时", "At once"), fmt.Sprintf(locale.T("%d 人", "%d"), l.DoorsPerSection()), sel(func() { g.configureService(0, 0, -1) }), sel(func() { g.configureService(0, 0, 1) }))
	text(dst, clip(locale.T("（均立即生效）", "(all apply at once)"), 13, x0+pw-20-float32(bx)), bx, float64(y+6), 13, colDim)
	y += 32
	bx = float64(tx) + nw + 16
	step(locale.T("中途最少停", "Least stop"), fmt.Sprintf(locale.T("%d 秒", "%d s"), l.Dwell()), sel(func() { g.configureDwell(-1, 0) }), sel(func() { g.configureDwell(1, 0) }))
	step(locale.T("终点最少停", "Least at ends"), fmt.Sprintf(locale.T("%d 秒", "%d s"), l.EndDwell()), sel(func() { g.configureDwell(0, -1) }), sel(func() { g.configureDwell(0, 1) }))
	text(dst, locale.T("（从下一站起生效；载客下调后超员车辆只下不上）", "(from the next stop on; trams above a lowered load let riders off first)"), bx, float64(y+6), 13, colDim)
	y += 36

	// Stops and trams side by side.
	half := (tw - 16) / 2
	bottom := y0 + ph - 16
	g.drawStopTable(dst, p, l, tx, y, half, bottom, live)
	g.drawTramTable(dst, p, l, tx+half+16, y, half, bottom, live)
}

func (g *Game) drawStopTable(dst *ebiten.Image, p *linePanel, l *street.Line, x, y, w, bottom float32, live bool) {
	text(dst, locale.T("各站客流（点击车站在地图上查看）", "Passengers by stop (click a stop to see it on the map)"), float64(x), float64(y), 14, colHeading)
	y += 24
	cols := fit([]column{{"#", 32, true}, {locale.T("车站", "Stop"), 0, false}, {locale.T("上车", "On"), 60, true}, {locale.T("下车", "Off"), 60, true}, {locale.T("候车", "Waiting"), 60, true}, {"", 90, false}}, w)
	vector.FillRect(dst, x, y, w, 22, rgb(40, 50, 64), false)
	cells(dst, cols, x, y+4, 13, []string{"#", locale.T("车站", "Stop"), locale.T("上车", "On"), locale.T("下车", "Off"), locale.T("候车", "Waiting")}, colDim)
	y += 24
	const rowH = 24
	n := len(l.Stops)
	rows := max(int((bottom-y)/rowH), 1)
	p.stopTop = min(p.stopTop, max(n-rows, 0))
	p.stopArea = rect{x, y, x + w, bottom}
	st := g.sim.LineStats(l.ID)
	most := 1
	for _, s := range l.Stops {
		most = max(most, g.sim.WaitingAt(s))
	}
	mx, my := ebiten.CursorPosition()
	for i := p.stopTop; i < n && i < p.stopTop+rows; i++ {
		id := l.Stops[i]
		r := rect{x, y, x + w, y + rowH - 2}
		p.stopRows = append(p.stopRows, stopRow{r, id})
		bg := colRowA
		if i%2 == 1 {
			bg = colRowB
		}
		if r.has(mx, my) {
			bg = tint(bg, rgb(255, 255, 255), 0.06)
		}
		vector.FillRect(dst, r.x0, r.y0, w, rowH-2, bg, false)
		wait := g.sim.WaitingAt(id)
		cells(dst, cols, x, y+4, 13, []string{fmt.Sprint(i + 1), g.net.Stops[id].Name,
			dash(st.On[id], live), dash(st.Off[id], live), dash(wait, live)}, color.White)
		if live && wait > 0 {
			bw := cols[5].w - 12
			bx := x + w - cols[5].w + 4
			vector.FillRect(dst, bx, y+7, bw, 8, rgb(44, 54, 68), false)
			vector.FillRect(dst, bx, y+7, bw*float32(wait)/float32(most), 8, colHover, false)
		}
		y += rowH
	}
	if n > rows {
		text(dst, fmt.Sprintf(locale.T("滚轮查看更多（%d–%d / %d）", "Scroll for more (%d–%d of %d)"), p.stopTop+1, min(p.stopTop+rows, n), n), float64(x), float64(bottom-2), 11, colDim)
	}
}

func (g *Game) drawTramTable(dst *ebiten.Image, p *linePanel, l *street.Line, x, y, w, bottom float32, live bool) {
	var vs []*traffic.Vehicle
	if live {
		for _, v := range g.sim.Vehicles {
			if v.Kind == traffic.TramKind && v.Line == l.ID {
				vs = append(vs, v)
			}
		}
		sort.Slice(vs, func(i, j int) bool { return vs[i].ID < vs[j].ID })
	}
	head := locale.T("电车（点击开始跟踪）", "Trams (click to follow)")
	if wait := g.sim.Waiting(l.ID); live && wait > 0 {
		head = fmt.Sprintf(locale.T("电车 · 另有 %d 辆待上线（点击开始跟踪）", "Trams · waiting to enter service: %d (click to follow)"), wait)
	}
	text(dst, head, float64(x), float64(y), 14, colHeading)
	y += 24
	cols := fit([]column{{locale.T("车", "#"), 34, true}, {locale.T("状态", "Status"), 0, false}, {locale.T("载客", "Load"), 78, true}, {locale.T("单程", "Trips"), 46, true}, {locale.T("里程", "Distance"), 76, true}, {locale.T("均速", "Avg speed"), 82, true}}, w)
	vector.FillRect(dst, x, y, w, 22, rgb(40, 50, 64), false)
	cells(dst, cols, x, y+4, 13, []string{locale.T("车", "#"), locale.T("状态", "Status"), locale.T("载客", "Load"), locale.T("单程", "Trips"), locale.T("里程", "Distance"), locale.T("均速", "Avg speed")}, colDim)
	y += 24
	if !live || len(vs) == 0 {
		msg := locale.T("运营开始后显示", "Shown once the service runs")
		if live && g.page == pageRun {
			msg = locale.T("电车即将上线", "Trams are about to enter service")
		}
		text(dst, msg, float64(x+6), float64(y+4), 13, colDim)
		return
	}
	const rowH = 24
	rows := max(int((bottom-y)/rowH), 1)
	p.tramTop = min(p.tramTop, max(len(vs)-rows, 0))
	p.tramArea = rect{x, y, x + w, bottom}
	now := g.sim.Seconds()
	mx, my := ebiten.CursorPosition()
	for i := p.tramTop; i < len(vs) && i < p.tramTop+rows; i++ {
		v := vs[i]
		r := rect{x, y, x + w, y + rowH - 2}
		p.tramRows = append(p.tramRows, tramRow{r, v.ID})
		bg := colRowA
		if i%2 == 1 {
			bg = colRowB
		}
		if r.has(mx, my) {
			bg = tint(bg, rgb(255, 255, 255), 0.06)
		}
		if v.ID == g.follow {
			bg = colRowSel
		}
		vector.FillRect(dst, r.x0, r.y0, w, rowH-2, bg, false)
		ts := v.Stats()
		cells(dst, cols, x, y+4, 13, []string{fmt.Sprint(i + 1), g.tramState(v),
			fmt.Sprintf("%d/%d", v.Load(), g.sim.Capacity(v)), fmt.Sprint(ts.Trips), km(ts.Distance), kmh(ts.Speed(now))}, color.White)
		y += rowH
	}
	if len(vs) > rows {
		text(dst, fmt.Sprintf(locale.T("滚轮查看更多（%d–%d / %d）", "Scroll for more (%d–%d of %d)"), p.tramTop+1, min(p.tramTop+rows, len(vs)), len(vs)), float64(x), float64(bottom-2), 11, colDim)
	}
}

// tramState says briefly what a tram is doing.
func (g *Game) tramState(v *traffic.Vehicle) string {
	stop := "—"
	if id := g.sim.TramStop(v); id >= 0 {
		if st := g.net.Stops[id]; st != nil {
			stop = st.Name
		}
	}
	switch {
	case v.Reversing():
		return locale.T("换端", "Changing ends")
	case v.Dwelling:
		return locale.T("停靠 ", "At ") + stop + " · " + g.doorState(v)
	}
	return fmt.Sprintf("%.0f km/h → %s", v.V*3.6, stop)
}

// doorState says what is happening at a stopped tram's doors: passengers
// getting off, getting on, or how long until it leaves.
func (g *Game) doorState(v *traffic.Vehicle) string {
	var parts []string
	ds, _ := v.Doors()
	switch {
	case ds == traffic.DoorsOpening:
		parts = append(parts, locale.T("开门中", "doors opening"))
	case ds == traffic.DoorsClosing:
		parts = append(parts, locale.T("关门中", "doors closing"))
	case ds == traffic.DoorsClosed && v.DwellLeft <= 0:
		return locale.T("已关门，即将发车", "doors shut, about to leave")
	case v.Alighting() > 0:
		parts = append(parts, fmt.Sprintf(locale.T("下车中，还有 %d 人", "%d still getting off"), v.Alighting()))
	case v.Load() > g.sim.Capacity(v):
		parts = append(parts, locale.T("超员只下不上", "over limit, alighting only"))
	case g.sim.Boarding(v) > 0:
		parts = append(parts, fmt.Sprintf(locale.T("上车中，还有 %d 人", "%d still getting on"), g.sim.Boarding(v)))
	}
	// The least stop time runs alongside the doors.
	switch {
	case v.DwellLeft > 0 && len(parts) > 0:
		parts = append(parts, fmt.Sprintf(locale.T("至少再停 %.0f 秒", "at least %.0f s more"), math.Ceil(v.DwellLeft)))
	case v.DwellLeft > 0:
		parts = append(parts, fmt.Sprintf(locale.T("%.0f 秒后发车", "leaves in %.0f s"), math.Ceil(v.DwellLeft)))
	case len(parts) == 0:
		parts = append(parts, locale.T("即将发车", "about to leave"))
	}
	return strings.Join(parts, " · ")
}
