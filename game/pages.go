package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
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
	g.page, g.start, g.anchor, g.pending, g.modelMenu = p, -1, nil, nil, nil
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
		case g.anchor != nil || g.start >= 0 || len(g.pending) > 0:
			g.anchor, g.start, g.pending = nil, -1, nil
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
		for i := range toolNames() {
			if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
				g.tool, g.start, g.anchor = tool(i), -1, nil
			}
		}
		switch {
		case g.tool == toolStop && inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
			g.platformLength(1)
		case g.tool == toolStop && inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
			g.platformLength(-1)
		case inpututil.IsKeyJustPressed(ebiten.KeyM):
			g.setLayout((g.layout + 1) % len(layouts))
		case inpututil.IsKeyJustPressed(ebiten.KeyL):
			// Cycle smoothly through the three buildable levels: Ground (0) -> Elevated (+1) -> Underground (-1).
			switch g.level {
			case groundLevel:
				g.setLevel(1) // Elevated (+1)
			case 1:
				g.setLevel(3) // Underground (-1)
			default:
				g.setLevel(groundLevel) // Ground (0)
			}
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
		case inpututil.IsKeyJustPressed(ebiten.KeyPeriod):
			g.configureLoad(1)
		case inpututil.IsKeyJustPressed(ebiten.KeyComma):
			g.configureLoad(-1)
		case inpututil.IsKeyJustPressed(ebiten.KeyEnter):
			g.commitLine()
		case inpututil.IsKeyJustPressed(ebiten.KeyN):
			if st, ok := g.hoverStop(); ok {
				g.openStopRename(st.ID)
			} else {
				g.say("%s", locale.T("把鼠标指向一个车站再按 N 改站名（或点击站名标签）", "Point at a stop and press N to rename it (or click its name label)"))
			}
		case inpututil.IsKeyJustPressed(ebiten.KeyBackspace), inpututil.IsKeyJustPressed(ebiten.KeyDelete):
			if len(g.pending) > 0 {
				g.pending = g.pending[:len(g.pending)-1]
				return
			}
			g.deleteLine()
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
			g.setDensity((g.densityIdx + 1) % len(densities))
		case inpututil.IsKeyJustPressed(ebiten.KeyF):
			g.followNext()
		case inpututil.IsKeyJustPressed(ebiten.KeyP):
			g.setPassengers((g.passIdx + 1) % len(traffic.PassengerRates))
		}
	}
}

// deleteLine deletes the selected line, or else the newest one.
func (g *Game) deleteLine() {
	id := g.selLine
	if id < 0 {
		if ids := g.net.LineIDs(); len(ids) > 0 {
			id = ids[len(ids)-1]
		}
	}
	if _, ok := g.net.Lines[id]; ok {
		g.say(locale.T("已删除 %d 路", "Deleted line %d"), g.lineIndex(id)+1)
		g.net.DeleteLine(id)
		g.selLine = -1
		g.edited()
	}
}

// hoverJunction is the junction under the cursor, if any.
func (g *Game) hoverJunction() (street.JunctionID, bool) {
	return g.net.NearestJunction(g.cursor, math.Max(g.pick(14), 12))
}

// hoverStop is the stop under the cursor, if any.
func (g *Game) hoverStop() (*street.Stop, bool) {
	if g.page == pageBuild {
		return g.net.StopNearAt(g.cursor, levelHeight(g.level), math.Max(g.pick(16), 20))
	}
	return g.net.StopNear(g.cursor, math.Max(g.pick(16), 20))
}

// platformLength lengthens or shortens the platform of the stop under the
// cursor, or else the platform new stops get.
func (g *Game) platformLength(d int) {
	if st, ok := g.hoverStop(); ok {
		k := min(max(st.Capacity()+d, street.MinSections), street.MaxSections)
		if k == st.Capacity() {
			return
		}
		if err := g.net.SetStopSections(st.ID, k); err != nil {
			g.say(locale.T("%s 站台无法改为 %d 节：%v", "%s: the platform can't hold %d sections: %v"), st.Name, k, err)
			return
		}
		g.edited()
		g.say(locale.T("%s：站台 %d 节（%.0f m）", "%s: platform %d sections (%.0f m)"), st.Name, k, street.SetLength(k))
		return
	}
	g.platform = min(max(g.platform+d, street.MinSections), street.MaxSections)
	g.say(locale.T("新车站的站台：%d 节（%.0f m）", "New stops get platforms for %d sections (%.0f m)"), g.platform, street.SetLength(g.platform))
}

// trackPreview is what a left click with the track tool would lay.
func (g *Game) trackPreview() []street.RoadID {
	if _, ok := g.onHighway(); ok {
		return nil
	}
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
		if g.landmarkClicked(mx, my) || g.stopLabelClicked(mx, my) {
			return
		}
		if g.page == pagePlan {
			for _, b := range g.cfgBtns {
				if b.r.has(mx, my) {
					b.act()
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
			g.say("%s", locale.T("依次点击车站，然后按回车建线", "Click stops in order, then press Enter to build the line"))
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
	if !levelOpen(g.level) {
		if left {
			g.say(locale.T("%s暂不能建设：%s（L 或右下角回到地面层）", "Nothing can be built on the %s yet: %s (L or the bottom right goes back to the ground)"), levelNames()[g.level], levelHints()[g.level])
		}
		return
	}
	if g.tool == toolRoad {
		if levelHeight(g.level) != 0 {
			if left {
				g.say("%s", locale.T("汽车道路只能在地面层建设（按 L 切回地面层）", "Car streets can only be built on the Ground level (press L for Ground)"))
			}
			return
		}
		g.roadTool(left, right)
		return
	}
	curLv := levelHeight(g.level)
	if curLv == 0 {
		if _, ok := g.onHighway(); ok && left && g.tool != toolRemove && g.tool != toolTrack {
			g.say("%s", highwayRefusal())
			return
		}
	}
	switch g.tool {
	case toolTrack:
		g.buildTrackTool(left, right)
	case toolStop:
		if !left {
			return
		}
		r, t, ok := g.net.NearestRoadAt(g.cursor, curLv, g.pick(14))
		if !ok {
			return
		}
		if st, ok := g.hoverStop(); ok {
			g.say(locale.T("%s：站台 %d 节（%.0f m），按 [ ] 调整", "%s: platform %d sections (%.0f m), [ ] to change"), st.Name, st.Capacity(), street.SetLength(st.Capacity()))
			return
		}
		if _, err := g.net.AddStop(r, t, g.platform); err != nil {
			g.say(locale.T("无法设站：%v", "Can't place a stop: %v"), err)
			return
		}
		g.edited()
	case toolCrossover:
		if !left {
			return
		}
		r, t, ok := g.net.NearestRoadAt(g.cursor, curLv, g.pick(12))
		if !ok {
			return
		}
		if err := g.net.AddCrossover(r, t); err != nil {
			g.say(locale.T("无法建渡线：%v", "Can't build a crossover: %v"), err)
			return
		}
		g.edited()
		g.say("%s", locale.T("已建渡线：电车可以在这里换到另一条轨道、折返", "Crossover built: trams can switch tracks and turn back here"))
	case toolRemove:
		if !left {
			return
		}
		if r, t, ok := g.net.CrossoverAtLevel(g.cursor, curLv, math.Max(g.pick(14), street.CrossoverLength/2)); ok {
			g.net.RemoveCrossover(r, t, 1)
			g.edited()
			g.say("%s", locale.T("已拆除渡线", "Crossover removed"))
			return
		}
		if r, _, ok := g.net.NearestRoadAt(g.cursor, curLv, g.pick(12)); ok && g.net.Roads[r].Tram != street.NoTram {
			if g.net.Roads[r].Tramway {
				if err := g.net.RemoveTramway(r); err != nil {
					g.say("%v", err)
					return
				}
				g.anchor, g.start = nil, -1
				g.edited()
				g.say("%s", locale.T("已拆除独立轨道段", "Removed tramway segment"))
				return
			}
			g.net.SetTram([]street.RoadID{r}, street.NoTram, false)
			g.edited()
		}
	}
}

func (g *Game) commitLine() {
	id, err := g.net.AddLine(g.pending)
	if err != nil {
		g.say(locale.T("无法建线：%v", "Can't build the line: %v"), err)
		return
	}
	g.pending = nil
	g.selLine = id
	g.edited()
	g.say(locale.T("已建立 %d 路，切换到「运行」开始运营", "Line %d built; switch to Run to start the service"), g.lineIndex(id)+1)
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
		g.say("%s", locale.T("还没有电车：先在「规划」页建线", "No trams yet: build a line on the Plan page first"))
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
	k := math.Min(0.65, 0.18*math.Max(1, g.zoom/8))
	g.camX += (mid.X - g.camX) * k
	g.camY += (mid.Y - g.camY) * k
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
			path, p, ok := n.TramRoute(cur.Link, cur.Head(n.TramLength(l)), calls[i%len(calls)], n.TramLength(l), calls[(i+1)%len(calls)])
			if !ok {
				break
			}
			var pts []street.Vec
			length := n.TramLength(l)
			start := cur.Pos
			if len(path) > 1 && n.Links[path[0]].Twin == path[1] && n.TurnsInPlace(path[0], cur.Head(length), length) {
				// Changes ends at the platform: draw from its tail onwards.
				start = n.Links[path[1]].Length - (cur.Head(length) - length)
				path = path[1:]
			}
			for k, lid := range path {
				lk := &n.Links[lid]
				from, to := 0.0, lk.Length
				if k == 0 {
					from = start
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
	if !levelOpen(g.level) {
		return
	}
	switch g.page {
	case pageBuild:
		curLv := levelHeight(g.level)
		// Built crossovers on this level, so they are easy to find.
		for _, rd := range g.net.Roads {
			la, lb := g.net.RoadLevels(rd.ID)
			if len(rd.Crossovers) == 0 || (la != curLv && lb != curLv) || !g.roadVisible(c, rd.ID) {
				continue
			}
			a, b := g.net.Ends(rd.ID)
			for _, t := range rd.Crossovers {
				g.crossoverMark(c, a.Lerp(b, t), unit(sub(b, a)), rd, colAccent)
			}
		}
		switch g.tool {
		case toolRoad:
			if curLv == 0 {
				g.drawRoadOverlay(c)
			}
			return
		case toolStop, toolCrossover:
			if curLv == 0 {
				if h, ok := g.onHighway(); ok {
					g.markHighway(c, h)
					return
				}
			}
		}
		switch g.tool {
		case toolTrack:
			g.drawTrackToolOverlay(c)
		case toolRemove:
			if r, t, ok := g.net.CrossoverAtLevel(g.cursor, curLv, math.Max(g.pick(14), street.CrossoverLength/2)); ok {
				a, b := g.net.Ends(r)
				g.ring(c, a.Lerp(b, t), street.CrossoverLength/2+3, 3, colBad)
			} else if r, _, ok := g.net.NearestRoadAt(g.cursor, curLv, g.pick(12)); ok && g.net.Roads[r].Tram != street.NoTram {
				g.highlightRoads(c, []street.RoadID{r}, colBad)
			}
		case toolCrossover:
			if r, t, ok := g.net.NearestRoadAt(g.cursor, curLv, g.pick(12)); ok {
				rd := g.net.Roads[r]
				col := rgb(47, 168, 90)
				if rd.Tram == street.NoTram || rd.Single || rd.OneWay() || g.net.IsRamp(r) {
					col = rgb(217, 59, 59)
				}
				a, b := g.net.Ends(r)
				g.crossoverMark(c, a.Lerp(b, t), unit(sub(b, a)), rd, col)
			}
		case toolStop:
			if st, ok := g.hoverStop(); ok {
				g.ring(c, g.net.StopPos(st), street.PlatformLength(st.Capacity())/2+6, 3, colHover)
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
	}
}

// text draws a label with the CJK-capable font.
func text(dst *ebiten.Image, s string, x, y, size float64, col color.Color) {
	textui.DrawText(dst, s, x, y, size, col)
}

// drawLabels writes stop names and line-pick order over the map.
func (g *Game) drawLabels(dst *ebiten.Image) {
	g.stopLabels = g.stopLabels[:0]
	g.drawLandmarks(dst)
	if g.zoom < 0.5 {
		return
	}
	for _, id := range g.net.StopIDs() {
		st := g.net.Stops[id]
		x, y := g.toScreen(g.net.StopPos(st))
		if x < -100 || y < -40 || int(x) > g.w+100 || int(y) > g.h+40 {
			continue
		}
		prefix := ""
		switch g.net.StopLevel(st) {
		case 1:
			prefix = locale.T("[高架] ", "[Elev] ")
		case -1:
			prefix = locale.T("[地下] ", "[Sub] ")
		}
		label := prefix + st.Name
		if g.page != pageRun {
			label = fmt.Sprintf(locale.T("%s%s · %d节", "%s%s · %d sections"), prefix, st.Name, st.Capacity())
		} else if n := g.sim.WaitingAt(id); n > 0 {
			label = fmt.Sprintf(locale.T("%s%s · 候车 %d", "%s%s · %d waiting"), prefix, st.Name, n)
		} else {
			label = prefix + st.Name + locale.T(" · 无人候车", " · nobody waiting")
		}
		w, _ := textui.MeasureText(label, 13)
		// Above the road, clear of the trams at the platforms when zoomed in.
		up := float32(math.Max(34, (g.net.Roads[st.Road].HalfWidth()+sidewalkWidth)*g.zoom+20))
		r := rect{x - float32(w)/2 - 6, y - up, x + float32(w)/2 + 6, y - up + 20}
		g.stopLabels = append(g.stopLabels, stopLabel{id, r})
		if g.page == pagePlan {
			if mx, my := ebiten.CursorPosition(); r.has(mx, my) {
				vector.FillRect(dst, r.x0-2, r.y0-2, r.x1-r.x0+4, r.y1-r.y0+4, colAccent, false) // clickable
			}
		}
		vector.FillRect(dst, x-float32(w)/2-6, y-up, float32(w)+12, 20, colHUD, false)
		vector.FillRect(dst, x-float32(w)/2-6, y-up, 3, 20, colAccent, false)
		text(dst, label, float64(x)-w/2, float64(y-up)+2, 13, color.White)
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
	for i, n := range pageNames() {
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
	g.hudBtns = g.hudBtns[:0]
	right := g.drawHUDRight(dst)
	x = g.drawToolbar(dst, x+12)
	var status string
	switch g.page {
	case pagePlan:
		status = fmt.Sprintf(locale.T("线路 %d 条", "lines: %d"), len(g.net.Lines))
	case pageRun:
		secs := int(g.sim.Seconds())
		status = fmt.Sprintf(locale.T("%02d:%02d:%02d   候车 %d · 已送达 %d", "%02d:%02d:%02d   %d waiting · %d delivered"), secs/3600, secs/60%60, secs%60, g.sim.TotalWaiting(), g.sim.Passengers.Delivered)
		if w, _ := textui.MeasureText(status, 13); x+w > right-8 {
			status = fmt.Sprintf("%02d:%02d:%02d", secs/3600, secs/60%60, secs%60)
		}
	}
	if w, _ := textui.MeasureText(status, 13); x+w <= right-8 {
		text(dst, status, x, 10, 13, rgb(190, 214, 226))
	}

	help := map[page]string{
		pageBuild: locale.T("轨道：点道路直接铺轨，或在空地/路口点击起点→终点直接建线；L 切换地面/高架/地下（选起点后按 L 再点终点即建高架坡道或隧道口，右键结束），M 切换单双线。车站：[ ] 调站台长度。", "Track: click a road, or click start -> end on open ground/junctions; L switches Ground/Elevated/Underground (press L after start to build a ramp/portal, right-click ends), M track form."),
		pagePlan:  locale.T("依次点击车站，回车建线；右键撤销一站。点击站名标签或指向车站按 N 改站名。选中左上线路后配置：+/- 电车数量，[ ] 编组节数，< > 每节载客，Delete 删除。", "Click stops in order, Enter builds the line, right-click undoes. Click a stop's label or press N on it to rename. Line at top left: +/- trams, [ ] sections, < > load."),
		pageRun:   locale.T("点击电车或左上线路开始跟踪（再点同一线路换下一辆），F 切换下一辆电车；空格暂停，+/- 倍速，C 汽车数量，P 乘客生成速度；T 线路管理查看运营数据。", "Click a tram or a line at top left to follow it, F next tram; Space pauses, +/- speed, C cars, P passengers, T line management."),
	}[g.page]
	lines := []string{help, locale.T("Tab 切换 建设 / 规划 / 运行   拖动平移，双指捏合 / 滚轮缩放，WASD 平移   T 线路管理   V 声音   F5 存档 / F9 读档   Esc 返回菜单", "Tab Build / Plan / Run   Drag to pan, pinch / wheel to zoom, WASD pans   T lines   V sound   F5 save / F9 load   Esc menu")}
	if g.msgTTL > 0 {
		lines = append(lines, "> "+g.message)
	}
	y := g.h - 22*len(lines) - 10
	vector.FillRect(dst, 0, float32(y-6), float32(g.w), float32(g.h-y+6), colHUD, false)
	for i, l := range lines {
		col := color.Color(color.White)
		if strings.HasPrefix(l, "> ") {
			col = colHover
		}
		text(dst, l, 12, float64(y+22*i), 14, col)
	}
	g.drawLevelPicker(dst, float32(y-6))
	g.drawLegend(dst)
	g.drawLineConfig(dst)
	g.drawFollowPanel(dst, float32(y-6))
	g.drawDropMenu(dst)
	g.drawSoundPanel(dst)
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
		// Only the two ends fit: where the line's trams turn back.
		ends := []street.StopID{l.Stops[0], l.Stops[len(l.Stops)-1]}
		if e := l.Ends(); len(e) >= 2 {
			ends = []street.StopID{e[0], e[len(e)-1]}
		}
		names := []string{g.net.Stops[ends[0]].Name, g.net.Stops[ends[1]].Name}
		info := fmt.Sprintf(locale.T("  %d 辆 × %d 节", "  trams %d × sections %d"), g.fleet(id), g.cars(id))
		if g.page == pageRun {
			switch n := running[id]; {
			case n == 0 && g.sim.Waiting(id) == 0:
				info = locale.T("  ! 无电车", "  ! no trams")
			case g.sim.Waiting(id) > 0:
				info = fmt.Sprintf(locale.T("  %d/%d 辆已上线", "  in service %d/%d"), n, n+g.sim.Waiting(id))
			default:
				info = fmt.Sprintf(locale.T("  %d 辆 · %d 运行中", "  trams %d · moving %d"), n, moving[id])
			}
		}
		label := fmt.Sprintf(locale.T("%d 路  %s%s", "Line %d  %s%s"), i+1, strings.Join(names, " – "), info)
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

// drawFollowPanel shows the followed tram's line, model, speed and next stop,
// with close-up zoom controls up to maxFollowZoom.
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
	model := street.DefaultTramModel()
	opMax := model.MaxSpeedKmh
	if l := g.net.Lines[v.Line]; l != nil {
		model = l.TramModel()
		opMax = l.OperatingSpeedKmh()
	}
	state := fmt.Sprintf(locale.T("%.0f / %d km/h · 前往 %s", "%.0f / %d km/h · to %s"), v.V*3.6, opMax, stop)
	switch {
	case v.Reversing():
		state = locale.T("终点换端中", "Changing ends at the terminus")
	case v.Dwelling:
		state = fmt.Sprintf(locale.T("停靠 %s · %s", "At %s · %s"), stop, g.doorState(v))
		if g.sim.AtTurnBack(v) {
			state += locale.T(" · 终点，停站时换端", " · terminus, changing ends during the stop")
		}
	}
	cap := g.sim.Capacity(v)
	state += fmt.Sprintf(locale.T("   载客 %d/%d", "   load %d/%d"), v.Load(), cap)
	if v.Load() > cap {
		state += locale.T("（只下不上）", " (alighting only)")
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
	title := fmt.Sprintf(locale.T("正在跟踪  %d 路 · %s · 第 %d/%d 辆 · %d 节", "Following  line %d · %s · tram %d of %d · %d sections"),
		i+1, model.Name(), nth, of, v.Cars)
	w1, _ := textui.MeasureText(title, 15)
	w2, _ := textui.MeasureText(state, 14)
	const zoomBoxW = 128.0
	w := float32(math.Max(w1+zoomBoxW, w2)) + 36
	y := bottom - 64
	vector.FillRect(dst, 8, y, w, 56, colHUD, false)
	vector.FillRect(dst, 8, y, 5, 56, lineColors[i%len(lineColors)], false)
	text(dst, title, 22, float64(y)+7, 15, color.White)
	text(dst, state, 22, float64(y)+32, 14, rgb(190, 214, 226))

	// Close-up zoom controls on the top right of the follow card.
	zx := float64(8+w) - zoomBoxW + 6
	zy := float64(y) + 5
	zx = buttonAt(dst, &g.hudBtns, zx, zy, 22, "-", btnPlain, func() {
		g.zoom = math.Max(minZoom, g.zoom/1.35)
	}) + 5
	zlbl := fmt.Sprintf("%.1f×", g.zoom)
	if g.zoom >= 10 {
		zlbl = fmt.Sprintf("%.0f×", g.zoom)
	}
	zw, _ := textui.MeasureText(zlbl, 13)
	text(dst, zlbl, zx+(44-zw)/2, zy+3, 13, colHeading)
	buttonAt(dst, &g.hudBtns, zx+48, zy, 22, "+", btnPlain, func() {
		g.zoom = math.Min(maxFollowZoom, g.zoom*1.35)
	})
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
