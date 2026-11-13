package game

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/build"
	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

type rect struct{ x0, y0, x1, y1 float32 }

func (r rect) has(x, y int) bool {
	return float32(x) >= r.x0 && float32(x) <= r.x1 && float32(y) >= r.y0 && float32(y) <= r.y1
}

type legendItem struct {
	line street.LineID
	r    rect
}

var tabRects [3]rect

// setPage switches screens; the service restarts on the Run page after edits.
func (g *Game) setPage(p page) {
	if p == g.page {
		return
	}
	g.page, g.start, g.pending = p, -1, nil
	if p == pageRun && g.dirty {
		g.rebuild()
	}
	if p != pageRun {
		g.follow = 0
	}
}

// --- input ------------------------------------------------------------------

func (g *Game) updateKeys() {
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		if shift {
			g.setPage((g.page + 2) % 3)
		} else {
			g.setPage((g.page + 1) % 3)
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		for i, r := range tabRects {
			if r.has(mx, my) {
				g.setPage(page(i))
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		switch {
		case g.start >= 0 || len(g.pending) > 0:
			g.start, g.pending = -1, nil
		case g.follow != 0:
			g.follow = 0
		case g.selLine >= 0:
			g.selLine = -1
		default:
			g.toMenu = true
		}
	}

	switch g.page {
	case pageBuild:
		for i := range toolNames {
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.tool, g.start = tool(i), -1
			}
		}
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyM):
			g.layout = (g.layout + 1) % len(layouts)
			g.say("轨道形式：%s", layouts[g.layout].name)
		case inpututil.IsKeyJustPressed(ebiten.KeyL):
			next := (g.level + 1) % len(levels)
			if !build.Rules[levels[next]].Buildable || levels[next] != sim.Ground {
				g.say("%s线路（轻轨 / 地铁）将在以后的版本开放", levelNames[next])
				return
			}
			g.level = next
		}
	case pagePlan:
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyEqual), inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
			g.configure(1, 0)
		case inpututil.IsKeyJustPressed(ebiten.KeyMinus), inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
			g.configure(-1, 0)
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
			g.configure(0, 1)
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
			g.configure(0, -1)
		case inpututil.IsKeyJustPressed(ebiten.KeyEnter):
			g.commitLine()
		case inpututil.IsKeyJustPressed(ebiten.KeyBackspace), inpututil.IsKeyJustPressed(ebiten.KeyDelete):
			if len(g.pending) > 0 {
				g.pending = g.pending[:len(g.pending)-1]
				return
			}
			id := g.selLine
			if id < 0 {
				if ids := g.net.LineIDs(); len(ids) > 0 {
					id = ids[len(ids)-1]
				}
			}
			if _, ok := g.net.Lines[id]; ok {
				g.say("已删除 %d 路", g.lineIndex(id)+1)
				g.net.DeleteLine(id)
				g.selLine = -1
				g.edited()
			}
		}
	case pageRun:
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeySpace):
			g.paused = !g.paused
		case inpututil.IsKeyJustPressed(ebiten.KeyEqual), inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
			g.speedIdx = min(g.speedIdx+1, len(speeds)-1)
		case inpututil.IsKeyJustPressed(ebiten.KeyMinus), inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
			g.speedIdx = max(g.speedIdx-1, 0)
		case inpututil.IsKeyJustPressed(ebiten.KeyC):
			g.densityIdx = (g.densityIdx + 1) % len(densities)
			g.sim.TargetCars = densities[g.densityIdx]
			g.say("汽车数量：%d", densities[g.densityIdx])
		case inpututil.IsKeyJustPressed(ebiten.KeyF):
			g.followNext()
		}
	}
}

// hoverJunction is the junction under the cursor, if any.
func (g *Game) hoverJunction() (street.JunctionID, bool) {
	return g.net.NearestJunction(g.cursor, math.Max(g.pick(14), 12))
}

// trackPreview is what a left click with the track tool would lay.
func (g *Game) trackPreview() []street.RoadID {
	if j, ok := g.hoverJunction(); ok {
		if g.start >= 0 && j != g.start {
			p, _ := g.net.RoadPath(g.start, j)
			return p
		}
		return nil
	}
	if r, _, ok := g.net.NearestRoad(g.cursor, g.pick(12)); ok {
		return []street.RoadID{r}
	}
	return nil
}

func (g *Game) updateTool() {
	left := g.click
	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if left {
		mx, my := ebiten.CursorPosition()
		if g.page == pagePlan {
			for _, b := range g.cfgBtns {
				if b.r.has(mx, my) {
					g.configure(b.dTrams, b.dSections)
					return
				}
			}
			if g.cfgPanel.has(mx, my) {
				return
			}
		}
		for _, it := range g.legend {
			if it.r.has(mx, my) {
				g.legendClick(it.line)
				return
			}
		}
	}
	switch g.page {
	case pageBuild:
		g.buildTool(left, right)
	case pagePlan:
		if right && len(g.pending) > 0 {
			g.pending = g.pending[:len(g.pending)-1]
		}
		if !left {
			return
		}
		st, ok := g.net.StopNear(g.cursor, math.Max(g.pick(16), 20))
		if !ok {
			g.say("依次点击车站，然后按回车建线")
			return
		}
		if n := len(g.pending); n == 0 || g.pending[n-1] != st.ID {
			g.pending = append(g.pending, st.ID)
		}
	case pageRun:
		if left {
			if v := g.tramAt(g.cursor); v != nil {
				g.follow = v.ID
			}
		}
	}
}

func (g *Game) buildTool(left, right bool) {
	switch g.tool {
	case toolTrack:
		if right {
			g.start = -1
		}
		if !left {
			return
		}
		roads := g.trackPreview()
		if j, ok := g.hoverJunction(); ok {
			g.start = j
		}
		if len(roads) > 0 {
			g.net.SetTram(roads, layouts[g.layout].mode, layouts[g.layout].single)
			g.edited()
		}
	case toolStop:
		if !left {
			return
		}
		r, t, ok := g.net.NearestRoad(g.cursor, g.pick(14))
		if !ok {
			return
		}
		if _, err := g.net.AddStop(r, t); err != nil {
			g.say("无法设站：%v", err)
			return
		}
		g.edited()
	case toolRemove:
		if !left {
			return
		}
		if r, _, ok := g.net.NearestRoad(g.cursor, g.pick(12)); ok && g.net.Roads[r].Tram != street.NoTram {
			g.net.SetTram([]street.RoadID{r}, street.NoTram, false)
			g.edited()
		}
	}
}

func (g *Game) commitLine() {
	id, err := g.net.AddLine(g.pending)
	if err != nil {
		g.say("无法建线：%v", err)
		return
	}
	g.pending = nil
	g.selLine = id
	g.edited()
	g.say("已建立 %d 路，切换到「运行」开始运营", g.lineIndex(id)+1)
}

func (g *Game) legendClick(id street.LineID) {
	switch g.page {
	case pageRun:
		var trams []*traffic.Vehicle
		for _, v := range g.sim.Vehicles {
			if v.Kind == traffic.TramKind && v.Line == id {
				trams = append(trams, v)
			}
		}
		if len(trams) == 0 {
			return
		}
		next := trams[0]
		for i, v := range trams {
			if v.ID == g.follow {
				next = trams[(i+1)%len(trams)]
			}
		}
		g.follow = next.ID
	default:
		if g.selLine == id {
			g.selLine = -1
		} else {
			g.selLine = id
		}
	}
}

// --- following --------------------------------------------------------------

func (g *Game) tramAt(p street.Vec) *traffic.Vehicle {
	var best *traffic.Vehicle
	bestD := math.Max(g.pick(18), 6)
	for _, v := range g.sim.Vehicles {
		if v.Kind != traffic.TramKind {
			continue
		}
		pts := g.sim.Body(v)
		for i := 0; i+1 < len(pts); i++ {
			if d := segDist(p, pts[i], pts[i+1]); d < bestD {
				best, bestD = v, d
			}
		}
	}
	return best
}

// followNext cycles the camera through the trams.
func (g *Game) followNext() {
	var trams []*traffic.Vehicle
	for _, v := range g.sim.Vehicles {
		if v.Kind == traffic.TramKind {
			trams = append(trams, v)
		}
	}
	if len(trams) == 0 {
		g.say("还没有电车：先在「规划」页建线")
		return
	}
	next := trams[0]
	for i, v := range trams {
		if v.ID == g.follow {
			next = trams[(i+1)%len(trams)]
		}
	}
	g.follow = next.ID
	if g.zoom < 1.5 {
		g.zoom = 2.5
	}
}

// updateFollow keeps the followed tram in the middle of the view.
func (g *Game) updateFollow() {
	if g.follow == 0 {
		return
	}
	v := g.sim.Find(g.follow)
	if v == nil {
		g.follow = 0
		return
	}
	pts := g.sim.Body(v)
	if len(pts) == 0 {
		return
	}
	mid, _ := along(pts, v.Length/2)
	g.camX += (mid.X - g.camX) * 0.18
	g.camY += (mid.Y - g.camY) * 0.18
}

// --- planning ---------------------------------------------------------------

// lineRoutes traces each line's track for the Plan page.
func (g *Game) lineRoutes() map[street.LineID][][]street.Vec {
	if g.routes != nil {
		return g.routes
	}
	g.routes = map[street.LineID][][]street.Vec{}
	n := g.net
	for _, id := range n.LineIDs() {
		l := n.Lines[id]
		cur, err := n.LineCycle(l)
		if err != nil {
			continue
		}
		calls := l.Calls()
		for i := 1; i <= len(calls); i++ {
			path, p, ok := n.TramRoute(cur.Link, cur.Pos, calls[i%len(calls)])
			if !ok {
				break
			}
			var pts []street.Vec
			for k, lid := range path {
				lk := &n.Links[lid]
				from, to := 0.0, lk.Length
				if k == 0 {
					from = cur.Pos
				}
				if k == len(path)-1 {
					to = p.Pos
				}
				pts = append(pts, lk.Slice(from, to)...)
			}
			g.routes[id] = append(g.routes[id], pts)
			cur = p
		}
	}
	return g.routes
}

// --- overlays ---------------------------------------------------------------

func (g *Game) drawToolOverlay(c *canvas) {
	switch g.page {
	case pageBuild:
		switch g.tool {
		case toolTrack:
			g.highlightRoads(c, g.trackPreview(), colOK)
			if g.start >= 0 {
				g.ring(c, g.net.Junctions[g.start].Pos, g.net.Radius(g.start), 3, colHover)
			}
			if j, ok := g.hoverJunction(); ok {
				g.ring(c, g.net.Junctions[j].Pos, g.net.Radius(j), 2, colHover)
			}
		case toolRemove:
			if r, _, ok := g.net.NearestRoad(g.cursor, g.pick(12)); ok && g.net.Roads[r].Tram != street.NoTram {
				g.highlightRoads(c, []street.RoadID{r}, colBad)
			}
		}
	case pagePlan:
		for i, id := range g.net.LineIDs() {
			col := lineColors[i%len(lineColors)]
			w, minPx := 3.0, 4.0
			if g.selLine >= 0 && g.selLine != id {
				col = tint(col, rgb(120, 120, 120), 0.6)
				w, minPx = 2.0, 2.5
			} else if g.selLine == id {
				w, minPx = 4.5, 6
			}
			for _, pts := range g.lineRoutes()[id] {
				if len(pts) > 1 {
					c.polyline(pts, w, minPx, col)
				}
			}
		}
		for _, id := range g.pending {
			g.ring(c, g.net.StopPos(g.net.Stops[id]), math.Max(10, 12/c.px()), 3, colHover)
		}
	case pageRun:
		if v := g.sim.Find(g.follow); v != nil {
			if pts := g.sim.Body(v); len(pts) > 0 {
				mid, _ := along(pts, v.Length/2)
				g.ring(c, mid, v.Length/2+4, 3, colHover)
			}
		}
	}
}

// text draws a label with the CJK-capable font.
func text(dst *ebiten.Image, s string, x, y, size float64, col color.Color) {
	textui.DrawText(dst, s, x, y, size, col)
}

// drawLabels writes stop names and line-pick order over the map.
func (g *Game) drawLabels(dst *ebiten.Image) {
	if g.zoom < 0.5 {
		return
	}
	for _, id := range g.net.StopIDs() {
		st := g.net.Stops[id]
		x, y := g.toScreen(g.net.StopPos(st))
		if x < -100 || y < -40 || int(x) > g.w+100 || int(y) > g.h+40 {
			continue
		}
		w, _ := textui.MeasureText(st.Name, 13)
		vector.FillRect(dst, x-float32(w)/2-6, y-34, float32(w)+12, 20, colHUD, false)
		vector.FillRect(dst, x-float32(w)/2-6, y-34, 3, 20, colAccent, false)
		text(dst, st.Name, float64(x)-w/2, float64(y)-32, 13, color.White)
	}
	for i, id := range g.pending {
		x, y := g.toScreen(g.net.StopPos(g.net.Stops[id]))
		text(dst, fmt.Sprint(i+1), float64(x)-4, float64(y)+14, 16, colHover)
	}
}

// --- HUD --------------------------------------------------------------------

func (g *Game) drawHUD(dst *ebiten.Image) {
	vector.FillRect(dst, 0, 0, float32(g.w), hudHeight, colHUD, false)
	x := 8.0
	for i, n := range pageNames {
		w, _ := textui.MeasureText(n, 15)
		r := rect{float32(x), 3, float32(x + w + 24), hudHeight - 3}
		tabRects[i] = r
		if page(i) == g.page {
			vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, colAccent, false)
			text(dst, n, x+12, 7, 15, colHUD)
		} else {
			vector.StrokeRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, 1, rgb(80, 92, 106), false)
			text(dst, n, x+12, 7, 15, color.White)
		}
		x += w + 30
	}
	x += 12
	var status string
	switch g.page {
	case pageBuild:
		for i, n := range toolNames {
			label := fmt.Sprintf("%d %s", i+1, n)
			w, _ := textui.MeasureText(label, 14)
			col := color.Color(color.White)
			if tool(i) == g.tool {
				vector.FillRect(dst, float32(x-5), 6, float32(w+10), hudHeight-12, rgb(52, 64, 80), false)
				col = colAccent
			}
			text(dst, label, x, 8, 14, col)
			x += w + 18
		}
		status = fmt.Sprintf("轨道形式：%s (M)   层级：%s (L)", layouts[g.layout].name, levelNames[g.level])
	case pagePlan:
		status = fmt.Sprintf("线路 %d 条   已选车站 %d 个", len(g.net.Lines), len(g.pending))
	case pageRun:
		secs := int(g.sim.Seconds())
		speed := fmt.Sprintf("×%d", speeds[g.speedIdx])
		if g.paused {
			speed = "暂停"
		}
		status = fmt.Sprintf("%02d:%02d:%02d %s   汽车 %d/%d (C)", secs/3600, secs/60%60, secs%60, speed, g.sim.CarCount(), g.sim.TargetCars)
	}
	text(dst, status, x+8, 8, 14, rgb(190, 214, 226))

	help := map[page]string{
		pageBuild: "点击道路铺轨，或点击路口 → 路口沿路线铺轨；M 切换 双线独立路权 / 双线混行 / 单线；右键结束。轨道尽头自动生成渡线折返。",
		pagePlan:  "依次点击车站，回车建线；右键撤销一站。点击左上线路选中后配置：+/- 电车数量，[ ] 编组节数，Delete 删除线路。",
		pageRun:   "点击电车或左上线路开始跟踪（再点同一线路换下一辆），F 切换下一辆电车；空格暂停，+/- 倍速。",
	}[g.page]
	lines := []string{help, "Tab 切换 建设 / 规划 / 运行   拖动平移，双指捏合 / 滚轮缩放，WASD 平移   Esc 返回菜单"}
	if g.msgTTL > 0 {
		lines = append(lines, "› "+g.message)
	}
	y := g.h - 22*len(lines) - 10
	vector.FillRect(dst, 0, float32(y-6), float32(g.w), float32(g.h-y+6), colHUD, false)
	for i, l := range lines {
		col := color.Color(color.White)
		if strings.HasPrefix(l, "›") {
			col = colHover
		}
		text(dst, l, 12, float64(y+22*i), 14, col)
	}
	g.drawLegend(dst)
	g.drawLineConfig(dst)
	g.drawFollowPanel(dst, float32(y-6))
}

func (g *Game) drawLegend(dst *ebiten.Image) {
	running := map[street.LineID]int{}
	moving := map[street.LineID]int{}
	if g.page == pageRun {
		for _, v := range g.sim.Vehicles {
			if v.Kind == traffic.TramKind {
				running[v.Line]++
				if !v.Dwelling {
					moving[v.Line]++
				}
			}
		}
	}
	g.legend = g.legend[:0]
	for i, id := range g.net.LineIDs() {
		l := g.net.Lines[id]
		names := make([]string, len(l.Stops))
		for k, s := range l.Stops {
			names[k] = g.net.Stops[s].Name
		}
		info := fmt.Sprintf("  %d 辆 × %d 节", g.fleet(id), l.Cars())
		if g.page == pageRun {
			switch n := running[id]; {
			case n == 0 && g.sim.Waiting(id) == 0:
				info = "  ! 无电车"
			case g.sim.Waiting(id) > 0:
				info = fmt.Sprintf("  %d/%d 辆已上线", n, n+g.sim.Waiting(id))
			default:
				info = fmt.Sprintf("  %d 辆 · %d 运行中", n, moving[id])
			}
		}
		label := fmt.Sprintf("%d 路  %s%s", i+1, strings.Join(names, " – "), info)
		w, _ := textui.MeasureText(label, 14)
		y := float32(hudHeight + 8 + 26*i)
		r := rect{8, y, float32(w) + 42, y + 22}
		g.legend = append(g.legend, legendItem{id, r})
		bg := colHUD
		if id == g.selLine && g.page == pagePlan {
			bg = rgba(60, 90, 110, 235)
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		vector.FillRect(dst, 14, y+5, 12, 12, lineColors[i%len(lineColors)], false)
		text(dst, label, 32, float64(y)+3, 14, color.White)
	}
}

// drawFollowPanel shows the followed tram's line, speed and next stop.
func (g *Game) drawFollowPanel(dst *ebiten.Image, bottom float32) {
	if g.page != pageRun || g.follow == 0 {
		return
	}
	v := g.sim.Find(g.follow)
	if v == nil {
		return
	}
	stop := "—"
	if id := g.sim.TramStop(v); id >= 0 {
		if st := g.net.Stops[id]; st != nil {
			stop = st.Name
		}
	}
	state := fmt.Sprintf("%.0f km/h · 前往 %s", v.V*3.6, stop)
	switch {
	case v.Reversing():
		state = "终点换端中"
	case v.Dwelling:
		state = fmt.Sprintf("停靠 %s · %.0f 秒后发车", stop, math.Ceil(v.DwellLeft))
	}
	i := g.lineIndex(v.Line)
	nth, of := 0, 0
	for _, o := range g.sim.Vehicles {
		if o.Kind == traffic.TramKind && o.Line == v.Line {
			of++
			if o.ID == v.ID {
				nth = of
			}
		}
	}
	title := fmt.Sprintf("正在跟踪  %d 路 · 第 %d/%d 辆 · %d 节编组", i+1, nth, of, v.Cars)
	w1, _ := textui.MeasureText(title, 16)
	w2, _ := textui.MeasureText(state, 14)
	w := float32(math.Max(w1, w2)) + 40
	y := bottom - 64
	vector.FillRect(dst, 8, y, w, 56, colHUD, false)
	vector.FillRect(dst, 8, y, 5, 56, lineColors[i%len(lineColors)], false)
	text(dst, title, 22, float64(y)+7, 16, color.White)
	text(dst, state, 22, float64(y)+32, 14, rgb(190, 214, 226))
}

// Run switches to the Run page (used by command-line flags).
func (g *Game) Run() { g.setPage(pageRun) }

// FollowFirst starts following the first tram.
func (g *Game) FollowFirst() { g.follow = 0; g.followNext() }

// ShowPage opens a page by name: build, plan or run.
func (g *Game) ShowPage(name string) {
	for i, n := range []string{"build", "plan", "run"} {
		if n == name {
			g.setPage(page(i))
		}
	}
}
