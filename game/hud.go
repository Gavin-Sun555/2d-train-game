package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/sound"
	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// The top bar's options are buttons: tools, track form and level on the
// Build page; pause, speed, traffic and passengers on the Run page; line
// management, save and load on every page. Their keys still work.

type hudButton struct {
	r   rect
	act func()
}

// Short names for the track forms on the buttons (layouts has the full ones).
func layoutShort() []string {
	return locale.L([]string{"双线独立", "双线混行", "单线往返"}, []string{"Own way", "Mixed", "Single"})
}

// clickHUD runs the top-bar button under a fresh left click. It reports
// whether one was hit.
func (g *Game) clickHUD() bool {
	if g.drop != nil && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.drop = nil
		return true
	}
	if g.lines == nil && g.modelMenu != nil && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.modelMenu = nil
		return true
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return false
	}
	mx, my := ebiten.CursorPosition()
	if g.lines == nil && g.modelMenu != nil {
		if g.clickModelMenu(mx, my) {
			return true
		}
	}
	if d := g.drop; d != nil {
		g.drop = nil
		for i, r := range d.items {
			if r.has(mx, my) {
				d.pick(i)
				return true
			}
		}
		if d.anchor.has(mx, my) {
			return true // the button that opened it: just close
		}
		// Elsewhere: closed; a click on another button still works.
		for _, b := range g.hudBtns {
			if b.r.has(mx, my) {
				b.act()
			}
		}
		return true
	}
	for _, b := range g.hudBtns {
		if b.r.has(mx, my) {
			b.act()
			return true
		}
	}
	if g.soundPanel {
		// A click elsewhere closes the sound panel (and does nothing else).
		if !g.soundRect.has(mx, my) {
			g.soundPanel = false
		}
		return true
	}
	return false
}

// hudButtonStyle: plain, selected (one of a group) or unavailable.
type hudButtonStyle int

const (
	btnPlain hudButtonStyle = iota
	btnOn
	btnOff
)

// button draws a top-bar button at x and returns its right edge.
func (g *Game) button(dst *ebiten.Image, x float64, label string, style hudButtonStyle, act func()) float64 {
	return buttonAt(dst, &g.hudBtns, x, 6, hudHeight-12, label, style, act)
}

// buttonAt draws a button with its top left at (x, y), adds it to list and
// returns its right edge.
func buttonAt(dst *ebiten.Image, list *[]hudButton, x, y, h float64, label string, style hudButtonStyle, act func()) float64 {
	const size, pad = 13.0, 7.0
	w, _ := textui.MeasureText(label, size)
	bw := math.Max(w+2*pad, h) // single signs get a square button
	r := rect{float32(x), float32(y), float32(x + bw), float32(y + h)}
	*list = append(*list, hudButton{r, act})
	mx, my := ebiten.CursorPosition()
	bg, fg := rgb(44, 54, 68), color.Color(color.White)
	switch style {
	case btnOn:
		bg, fg = colAccent, rgb(14, 22, 30)
	case btnOff:
		bg, fg = rgb(36, 42, 52), rgb(120, 130, 142)
	}
	if r.has(mx, my) && style != btnOn {
		bg = tint(bg, rgb(255, 255, 255), 0.16)
	}
	vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
	text(dst, label, x+(bw-w)/2, y+(h-size)/2-1, size, fg)
	return float64(r.x1)
}

// group draws a label and a row of buttons, the chosen one highlighted.
func (g *Game) group(dst *ebiten.Image, x float64, label string, names []string, chosen int, off func(int) bool, pick func(int)) float64 {
	if label != "" {
		w, _ := textui.MeasureText(label, 13)
		text(dst, label, x, 10, 13, rgb(150, 170, 186))
		x += w + 6
	}
	for i, n := range names {
		style := btnPlain
		switch {
		case i == chosen:
			style = btnOn
		case off != nil && off(i):
			style = btnOff
		}
		i := i
		x = g.button(dst, x, n, style, func() { pick(i) }) + 2
	}
	return x + 12
}

func (g *Game) setLayout(i int) {
	g.layout = i
	g.say(locale.T("轨道形式：%s", "Track: %s"), layoutName(i))
}

func levelOpen(i int) bool {
	h := levelHeight(i)
	return h >= -1 && h <= 1
}

// setLevel shows one of the five levels; ground (0), elevated (+1) and
// underground (-1) can all carry tramways, with an active track anchor
// kept across open levels so clicking builds a portal ramp between them.
func (g *Game) setLevel(i int) {
	g.level = min(max(i, 0), len(levels)-1)
	g.roadStart = nil
	if !levelOpen(g.level) {
		g.anchor, g.start = nil, -1
		g.say("%s：%s", levelNames()[g.level], levelHints()[g.level])
	} else if g.anchor != nil && g.anchor.level != levelHeight(g.level) {
		g.say(locale.T("已切至%s：点击终点即可建 %s → %s 出入坡道", "Switched to %s: click end point to build a %s -> %s portal ramp"),
			levelNames()[g.level], levelLabel(g.anchor.level), levelLabel(levelHeight(g.level)))
	}
}

func (g *Game) setDensity(i int) {
	g.densityIdx = i
	g.sim.TargetCars = densities[i]
	g.say(locale.T("汽车数量：%d", "Cars: %d"), densities[i])
}

func (g *Game) setPassengers(i int) {
	g.passIdx = i
	g.sim.PassengerRate = traffic.PassengerRates[i]
	g.say(locale.T("乘客生成速度：%s", "Passengers: %s"), passNames()[i])
}

// drawToolbar draws the page's options from x and returns where it ended.
func (g *Game) drawToolbar(dst *ebiten.Image, x float64) float64 {
	switch g.page {
	case pageBuild:
		names := make([]string, len(toolNames()))
		for i, n := range toolNames() {
			names[i] = fmt.Sprintf("%d %s", i+1, n)
		}
		x = g.group(dst, x, "", names, int(g.tool), nil, func(i int) { g.tool, g.start, g.anchor = tool(i), -1, nil })
		if g.tool != toolRoad {
			lvShort := locale.L([]string{"地下", "地面", "高架"}, []string{"Sub", "Ground", "Elev"})
			lvIdxs := []int{3, 2, 1} // Underground (-1), Ground (0), Elevated (+1)
			chosenLv := -1
			for k, idx := range lvIdxs {
				if g.level == idx {
					chosenLv = k
				}
			}
			x = g.group(dst, x, locale.T("层高 L", "Level L"), lvShort, chosenLv, nil, func(k int) { g.setLevel(lvIdxs[k]) })
		}
		switch g.tool {
		case toolTrack:
			x = g.group(dst, x, locale.T("形式 M", "Form M"), layoutShort(), g.layout, nil, g.setLayout)
		case toolRoad:
			x = g.group(dst, x, "", roadModeNames(), g.roadMode, nil, func(i int) { g.roadMode, g.roadStart = i, nil })
			if g.roadMode != roadRemove {
				names := make([]string, len(roadLaneOpts))
				for i, o := range roadLaneOpts {
					names[i] = o.short
				}
				x = g.group(dst, x, locale.T("车道", "Lanes"), names, g.roadLanes, nil, func(i int) { g.roadLanes = i })
			}
		case toolStop:
			w, _ := textui.MeasureText(locale.T("新站台 [ ]", "New platform [ ]"), 13)
			text(dst, locale.T("新站台 [ ]", "New platform [ ]"), x, 10, 13, rgb(150, 170, 186))
			x = g.button(dst, x+w+6, "-", btnPlain, func() { g.platformLength(-1) }) + 6
			v := fmt.Sprintf(locale.T("%d 节 · %.0f m", "%d sections · %.0f m"), g.platform, street.SetLength(g.platform))
			vw, _ := textui.MeasureText(v, 13)
			text(dst, v, x, 10, 13, color.White)
			x = g.button(dst, x+vw+6, "+", btnPlain, func() { g.platformLength(1) }) + 12
		}
	case pagePlan:
		style := btnOff
		if len(g.pending) >= 2 {
			style = btnPlain
		}
		x = g.button(dst, x, fmt.Sprintf(locale.T("建立线路 Enter（已选 %d 站）", "Build line Enter (%d stops picked)"), len(g.pending)), style, g.commitLine) + 4
		if len(g.pending) > 0 {
			x = g.button(dst, x, locale.T("撤销一站", "Undo stop"), btnPlain, func() { g.pending = g.pending[:len(g.pending)-1] }) + 4
		}
		if l := g.net.Lines[g.selLine]; l != nil {
			x = g.button(dst, x, fmt.Sprintf(locale.T("删除 %d 路", "Delete line %d"), g.lineIndex(l.ID)+1), btnPlain, g.deleteLine) + 4
		}
		x += 8
	case pageRun:
		label, style := locale.T("暂停 空格", "Pause Space"), btnPlain
		if g.paused {
			label, style = locale.T("继续 空格", "Resume Space"), btnOn
		}
		x = g.button(dst, x, label, style, func() { g.paused = !g.paused }) + 12
		sp := make([]string, len(speeds))
		for i, s := range speeds {
			sp[i] = fmt.Sprintf("×%d", s)
		}
		x = g.group(dst, x, locale.T("倍速", "Speed"), sp, g.speedIdx, nil, func(i int) { g.speedIdx = i })
		cars := make([]string, len(densities))
		for i, d := range densities {
			cars[i] = fmt.Sprintf(locale.T("%d 辆", "%d"), d)
		}
		x = g.dropButton(dst, x, locale.T("汽车 C", "Cars C"), cars, g.densityIdx, g.setDensity) + 12
		x = g.dropButton(dst, x, locale.T("乘客 P", "Passengers P"), passNames(), g.passIdx, g.setPassengers) + 12
	}
	return x
}

// drawHUDRight draws line management, save and load at the bar's right end
// and returns where they start.
func (g *Game) drawHUDRight(dst *ebiten.Image) float64 {
	labels := []string{locale.T("声音 V", "Sound V"), locale.T("线路管理 T", "Lines T"), locale.T("存档 F5", "Save F5"), locale.T("读档 F9", "Load F9")}
	acts := []func(){func() { g.soundPanel = !g.soundPanel }, g.toggleLinePanel, func() { g.openSlots(true) }, func() { g.openSlots(false) }}
	x := float64(g.w) - 8
	for i := len(labels) - 1; i >= 0; i-- {
		w, _ := textui.MeasureText(labels[i], 13)
		x -= w + 14
		style := btnPlain
		if i == 0 && g.soundPanel || i == 1 && g.lines != nil {
			style = btnOn
		}
		g.button(dst, x, labels[i], style, acts[i])
		if i == 0 {
			g.soundBtn = g.hudBtns[len(g.hudBtns)-1].r
		}
		x -= 6
	}
	return x
}

// dropMenu is an open drop-down list under a top-bar button.
type dropMenu struct {
	anchor  rect
	label   string
	options []string
	chosen  int
	pick    func(int)
	items   []rect
}

// dropButton draws "label  value ▾"; clicking it opens the options in a
// drop-down list, keeping the bar narrow.
func (g *Game) dropButton(dst *ebiten.Image, x float64, label string, options []string, chosen int, pick func(int)) float64 {
	w, _ := textui.MeasureText(label, 13)
	text(dst, label, x, 10, 13, rgb(150, 170, 186))
	x += w + 6
	var r rect
	open := g.drop != nil && g.drop.label == label
	style := btnPlain
	if open {
		style = btnOn
	}
	right := g.button(dst, x, options[chosen]+"     ", style, func() {
		g.drop = &dropMenu{anchor: r, label: label, options: options, chosen: chosen, pick: pick}
	})
	r = g.hudBtns[len(g.hudBtns)-1].r
	if open {
		g.drop.anchor = r
	}
	// A small arrow pointing down (the font has no ▾).
	col := color.Color(rgb(190, 214, 226))
	if open {
		col = rgb(14, 22, 30)
	}
	ax, ay := r.x1-15, (r.y0+r.y1)/2-2
	for k := float32(0); k < 4; k++ {
		vector.FillRect(dst, ax+k, ay+k, 8-2*k, 1, col, false)
	}
	return right
}

// drawDropMenu draws the open drop-down list over everything else.
func (g *Game) drawDropMenu(dst *ebiten.Image) {
	d := g.drop
	if d == nil {
		return
	}
	const rowH = 28
	w := float32(0)
	for _, o := range d.options {
		tw, _ := textui.MeasureText(o, 13)
		w = max(w, float32(tw)+36)
	}
	w = max(w, d.anchor.x1-d.anchor.x0)
	x, y := d.anchor.x0, d.anchor.y1+2
	vector.FillRect(dst, x, y, w, float32(len(d.options))*rowH+4, rgb(28, 35, 46), false)
	vector.StrokeRect(dst, x, y, w, float32(len(d.options))*rowH+4, 1, colAccent, false)
	mx, my := ebiten.CursorPosition()
	d.items = d.items[:0]
	for i, o := range d.options {
		r := rect{x + 2, y + 2 + float32(i)*rowH, x + w - 2, y + 2 + float32(i+1)*rowH}
		d.items = append(d.items, r)
		switch {
		case i == d.chosen:
			vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, rgb(44, 70, 88), false)
		case r.has(mx, my):
			vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, rgb(44, 54, 68), false)
		}
		if i == d.chosen {
			vector.FillRect(dst, r.x0+8, r.y0+8, 4, rowH-16, colAccent, false)
		}
		text(dst, o, float64(r.x0)+26, float64(r.y0)+6, 13, color.White)
	}
}

// OpenMenu opens the cars or passengers drop-down (used by command-line
// flags; it is placed when the bar is next drawn).
func (g *Game) OpenMenu(name string) {
	if name == "sound" {
		g.soundPanel = true
		return
	}
	label, opts, chosen, pick := locale.T("汽车 C", "Cars C"), []string(nil), g.densityIdx, g.setDensity
	for _, d := range densities {
		opts = append(opts, fmt.Sprintf(locale.T("%d 辆", "%d"), d))
	}
	if name == "passengers" {
		label, opts, chosen, pick = locale.T("乘客 P", "Passengers P"), passNames(), g.passIdx, g.setPassengers
	}
	g.drop = &dropMenu{label: label, options: opts, chosen: chosen, pick: pick}
}

// drawSoundPanel shows the three volumes under the Sound button.
func (g *Game) drawSoundPanel(dst *ebiten.Image) {
	if !g.soundPanel {
		return
	}
	rows := []struct {
		label string
		get   func() float64
		set   func(*sound.Volumes, float64)
	}{
		{locale.T("电车运行声", "Tram motors"), func() float64 { return volumes.Motor }, func(v *sound.Volumes, x float64) { v.Motor = x }},
		{locale.T("路口铃声", "Crossing gong"), func() float64 { return volumes.Bell }, func(v *sound.Volumes, x float64) { v.Bell = x }},
		{locale.T("报站", "Announcements"), func() float64 { return volumes.Announce }, func(v *sound.Volumes, x float64) { v.Announce = x }},
		{locale.T("车门声", "Doors"), func() float64 { return volumes.Doors }, func(v *sound.Volumes, x float64) { v.Doors = x }},
	}
	choices := []struct {
		label string
		names []string
		cur   int
		set   func(int)
	}{
		{locale.T("铃声", "Gong"), bellNames(), int(styles.Bell), func(i int) {
			st := styles
			st.Bell = sound.BellStyle(i)
			setStyles(st, true)
			if sink != nil {
				sink.PreviewBell()
			}
		}},
		{locale.T("引擎声", "Motor"), motorNames(), int(styles.Motor), func(i int) {
			st := styles
			st.Motor = sound.MotorStyle(i)
			setStyles(st, true)
		}},
		{locale.T("车门声", "Doors"), doorNames(), int(styles.Door), func(i int) {
			st := styles
			st.Door = sound.DoorStyle(i)
			setStyles(st, true)
			if sink != nil {
				sink.Door(false, 1, 0) // a preview: the warning and closing
			}
		}},
		{locale.T("播报声音", "Voice"), voiceNames(), int(styles.Voice), func(i int) {
			st := styles
			st.Voice = sound.VoiceStyle(i)
			setStyles(st, true)
			g.announce(locale.T("前方到站：示例站，下车的乘客请做好准备。", "Arriving at Sample Street. Please get ready to leave the tram."))
		}},
	}
	const pw, rowH = 330.0, 34.0
	x := math.Min(float64(g.soundBtn.x0), float64(g.w)-pw-8)
	y := float64(g.soundBtn.y1) + 4
	ph := rowH*float64(len(rows)+len(choices)) + 66
	g.soundRect = rect{float32(x), float32(y), float32(x + pw), float32(y + ph)}
	vector.FillRect(dst, float32(x), float32(y), pw, float32(ph), rgb(28, 35, 46), false)
	vector.StrokeRect(dst, float32(x), float32(y), pw, float32(ph), 1, colAccent, false)
	for i, r := range rows {
		ry := y + 10 + float64(i)*rowH
		text(dst, r.label, x+14, ry+5, 13, color.White)
		r := r
		change := func(d float64) func() {
			return func() {
				v := volumes
				r.set(&v, r.get()+d)
				setVolumes(v, true)
			}
		}
		bx := x + pw - 182
		buttonAt(dst, &g.hudBtns, bx, ry, 26, "-", btnPlain, change(-0.1))
		val := fmt.Sprintf("%.0f%%", r.get()*100)
		vw, _ := textui.MeasureText(val, 13)
		text(dst, val, bx+26+(116-vw)/2, ry+5, 13, color.White)
		buttonAt(dst, &g.hudBtns, bx+144, ry, 26, "+", btnPlain, change(0.1))
	}
	// The choices, each stepped with < >.
	for i, c := range choices {
		ry := y + 16 + float64(len(rows)+i)*rowH
		text(dst, c.label, x+14, ry+5, 13, color.White)
		c := c
		n := len(c.names)
		bx := x + pw - 182
		buttonAt(dst, &g.hudBtns, bx, ry, 26, "<", btnPlain, func() { c.set((c.cur + n - 1) % n) })
		name := clip(c.names[c.cur], 13, 112)
		nw, _ := textui.MeasureText(name, 13)
		text(dst, name, bx+26+(116-nw)/2, ry+5, 13, colHeading)
		buttonAt(dst, &g.hudBtns, bx+144, ry, 26, ">", btnPlain, func() { c.set((c.cur + 1) % n) })
	}
	vector.FillRect(dst, float32(x+12), float32(y+8+float64(len(rows))*rowH+4), float32(pw-24), 1, rgb(60, 72, 88), false)
	notes := []string{
		locale.T("运行声、铃声、车门声：屏幕上和附近的电车", "Motors, gongs, doors: trams on and near the screen"),
		locale.T("报站：只给正在跟踪的电车（F 或点击电车）", "Announcements: the followed tram only (F or click)"),
	}
	for i, n := range notes {
		text(dst, clip(n, 11, pw-24), x+12, y+ph-40+float64(i)*16, 11, colDim)
	}
}
