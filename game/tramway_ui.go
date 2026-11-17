package game

import (
	"errors"
	"math"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// trackAnchor remembers the starting point and vertical level (-1, 0, +1) of
// an in-progress track segment so the player can build across open ground or
// switch levels (L) before clicking the end point to form a ramp/portal.
type trackAnchor struct {
	pos      street.Vec
	level    int               // -1 underground, 0 ground, 1 elevated
	junction street.JunctionID // >= 0 when snapped to an existing junction at level
	onStreet bool              // true when snapped to a car-street junction at level 0
}

func (g *Game) attachCorridorCheck() {
	if g.net != nil {
		g.net.CheckCorridor = g.checkCorridor
	}
}

func (g *Game) hasStreetAt(j street.JunctionID) bool {
	if j < 0 || int(j) >= len(g.net.Junctions) {
		return false
	}
	for _, r := range g.net.RoadsAt(j) {
		if !g.net.Roads[r].Tramway {
			return true
		}
	}
	return false
}

func (g *Game) onAnyStreetCarriageway(p street.Vec) bool {
	for _, rd := range g.net.Roads {
		if rd.Tramway {
			continue
		}
		if onCarriageway(g.net, rd.ID, p, 0.8) {
			return true
		}
	}
	return false
}

func (g *Game) waterAt(p street.Vec) bool {
	if g.city != nil {
		return g.city.TileAt(p).Kind == city.Water
	}
	if g.scene != nil {
		// Existing road bridges carry over water.
		if g.onAnyStreetCarriageway(p) {
			return false
		}
		for i := range g.scene.areas {
			a := &g.scene.areas[i]
			if a.kind == "water" && p.X >= a.m.box[0] && p.X <= a.m.box[2] && p.Y >= a.m.box[1] && p.Y <= a.m.box[3] {
				if polyContains(a.m.pts, p) {
					return true
				}
			}
		}
	}
	return false
}

// buildingAt reports whether p hits a building, and its approximate height (m).
func (g *Game) buildingAt(p street.Vec) (bool, float64) {
	if g.city != nil {
		for _, lot := range g.city.Lots {
			if lot.Style == city.LotPark || lot.Style == city.LotPlaza {
				continue
			}
			const margin = 4.0
			if p.X > lot.X+margin && p.X < lot.X+lot.W-margin && p.Y > lot.Y+margin && p.Y < lot.Y+lot.H-margin {
				switch lot.Style {
				case city.LotTower:
					return true, 40
				case city.LotCourt:
					return true, 18
				default:
					return true, 8
				}
			}
		}
		return false, 0
	}
	if g.scene != nil {
		for i := range g.scene.buildings {
			b := &g.scene.buildings[i]
			if p.X >= b.m.box[0] && p.X <= b.m.box[2] && p.Y >= b.m.box[1] && p.Y <= b.m.box[3] {
				if polyContains(b.m.pts, p) {
					h := b.height
					if h <= 0 {
						h = 10
					}
					return true, h
				}
			}
		}
	}
	return false, 0
}

func (g *Game) highwayConflict(p street.Vec, lvA, lvB int) bool {
	if g.scene == nil {
		return false
	}
	for i := range g.scene.highways {
		h := &g.scene.highways[i]
		sameLv := h.level == lvA || h.level == lvB || (lvA != lvB && h.level == 0)
		if !sameLv {
			continue
		}
		if p.X < h.box[0]-h.width || p.X > h.box[2]+h.width || p.Y < h.box[1]-h.width || p.Y > h.box[3]+h.width {
			continue
		}
		for k := 0; k+1 < len(h.pts); k++ {
			if segDist(p, h.pts[k], h.pts[k+1]) < h.width/2+2.0 {
				return true
			}
		}
	}
	return false
}

// checkCorridor validates map terrain, water, buildings and motorways along a
// proposed tramway segment from (a, lvA) to (b, lvB).
func (g *Game) checkCorridor(a, b street.Vec, lvA, lvB int) error {
	for _, p := range []street.Vec{a, b} {
		if p.X < 8 || p.Y < 8 || p.X > g.mapW-8 || p.Y > g.mapH-8 {
			return errors.New(locale.T("超出地图可建设边界", "Outside the map's buildable area"))
		}
	}
	l := dist(a, b)
	steps := max(int(math.Ceil(l/2.0)), 1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		p := a.Lerp(b, t)
		if g.highwayConflict(p, lvA, lvB) {
			return errors.New(locale.T("与同层高速公路冲突（请改用高架跨越或地下下穿）", "Conflicts with a motorway at this level (go over on a viaduct or under in a tunnel)"))
		}
		switch {
		case lvA == -1 && lvB == -1:
			// Underground tunnels pass freely beneath water, streets and buildings.
		case lvA == 1 && lvB == 1:
			// Elevated viaducts cross water and streets, but cannot cut through tall buildings.
			if hit, h := g.buildingAt(p); hit && h >= 12 {
				return errors.New(locale.T("高架轨道不能穿过高层建筑（请绕行或改走地下隧道）", "Elevated viaducts can't pass through tall buildings (route around or go underground)"))
			}
		default:
			// Ground off-road track or portal ramp (0 <-> +-1).
			if g.waterAt(p) {
				return errors.New(locale.T("地面轨道与出入坡道不能穿过水域，请按 L 切换高架或地下层跨越", "Ground track and portal ramps can't cross water: press L for Elevated or Underground"))
			}
			// Endpoints on an existing ground street are valid junctions; interior points cannot hit buildings.
			if (i > 0 && i < steps) || !g.onAnyStreetCarriageway(p) {
				if hit, _ := g.buildingAt(p); hit && !g.onAnyStreetCarriageway(p) {
					return errors.New(locale.T("地面轨道与出入坡道不能穿过建筑物（可在空地铺设或按 L 改走地下）", "Ground track and portal ramps can't pass through buildings (build on open ground or press L for Underground)"))
				}
			}
		}
	}
	return nil
}

// snapTrackPoint resolves the cursor at vertical level lv to a nearby junction,
// a flat road/tramway at lv, or the open-ground/air/underground cursor position.
func (g *Game) snapTrackPoint(lv int) (pos street.Vec, jid street.JunctionID, onRoad bool) {
	snapJ := math.Max(g.pick(14), 12)
	if j, ok := g.net.NearestJunctionAt(g.cursor, lv, snapJ); ok {
		return g.net.Junctions[j].Pos, j, true
	}
	snapR := math.Max(g.pick(12), 8)
	for _, r := range g.net.Roads {
		la, lb := g.net.RoadLevels(r.ID)
		if la != lv || lb != lv {
			continue
		}
		a, b := g.net.Ends(r.ID)
		if t, d := projectOn(g.cursor, a, b); d <= snapR {
			snapR = d
			pos = a.Lerp(b, t)
			onRoad = true
		}
	}
	if onRoad {
		return pos, -1, true
	}
	return g.cursor, -1, false
}

func levelLabel(lv int) string {
	switch {
	case lv > 0:
		return locale.T("高架", "elevated")
	case lv < 0:
		return locale.T("地下", "underground")
	default:
		return locale.T("地面", "ground")
	}
}

// buildTrackTool handles clicks for toolTrack across streets, open ground,
// elevated viaducts, underground tunnels and transition portals.
func (g *Game) buildTrackTool(left, right bool) {
	if right {
		g.anchor, g.start = nil, -1
		return
	}
	if !left {
		return
	}
	curLv := levelHeight(g.level)

	// No start anchor yet: on ground level, single-clicking a street (away from
	// junctions) lays track on that street immediately, just as before.
	if g.anchor == nil {
		if curLv == 0 {
			if j, ok := g.net.NearestJunctionAt(g.cursor, 0, math.Max(g.pick(14), 12)); ok {
				g.anchor = &trackAnchor{pos: g.net.Junctions[j].Pos, level: 0, junction: j, onStreet: g.hasStreetAt(j)}
				g.start = j
				g.say("%s", locale.T("已选起点：点击目标点铺轨，或按 L 切换高架/地下建出入坡道（右键取消）", "Start chosen: click destination, or press L for Elevated/Underground to build a ramp/portal (right-click cancels)"))
				return
			}
			if _, ok := g.onHighway(); ok {
				g.say("%s", highwayRefusal())
				return
			}
			if r, _, ok := g.net.NearestStreet(g.cursor, g.pick(12)); ok {
				g.net.SetTram([]street.RoadID{r}, layouts[g.layout].mode, layouts[g.layout].single)
				g.edited()
				return
			}
		}
		pos, jid, _ := g.snapTrackPoint(curLv)
		if err := g.checkCorridor(pos, pos, curLv, curLv); err != nil {
			g.say("%v", err)
			return
		}
		g.anchor = &trackAnchor{pos: pos, level: curLv, junction: jid, onStreet: curLv == 0 && jid >= 0 && g.hasStreetAt(jid)}
		if curLv == 0 {
			g.start = jid
		}
		g.say(locale.T("已选%s起点：点击终点直接铺设，或按 L 切换层级建坡道（右键结束）", "Start set (%s): click end point to build, or press L to change level via a ramp (right-click ends)"), levelLabel(curLv))
		return
	}

	// Start anchor IS active: check classic street-junction -> street-junction path first.
	start := *g.anchor
	if start.level == 0 && curLv == 0 && start.onStreet && start.junction >= 0 {
		if jb, ok := g.net.NearestJunctionAt(g.cursor, 0, math.Max(g.pick(14), 12)); ok && jb != start.junction && g.hasStreetAt(jb) {
			if roads, ok := g.net.RoadPath(start.junction, jb); ok && len(roads) > 0 {
				g.net.SetTram(roads, layouts[g.layout].mode, layouts[g.layout].single)
				g.edited()
				g.anchor = &trackAnchor{pos: g.net.Junctions[jb].Pos, level: 0, junction: jb, onStreet: true}
				g.start = jb
				return
			}
		}
	}

	endPos, _, _ := g.snapTrackPoint(curLv)
	snap := math.Max(g.pick(14), 12)
	if err := g.net.AddTramway(start.pos, endPos, start.level, curLv, layouts[g.layout].single, snap); err != nil {
		g.say("%v", err)
		return
	}
	g.edited()
	// Advance anchor to the newly created/joined endpoint so the player can keep chaining.
	jid, _ := g.net.NearestJunctionAt(endPos, curLv, snap+2)
	realEnd := endPos
	if jid >= 0 {
		realEnd = g.net.Junctions[jid].Pos
	}
	g.anchor = &trackAnchor{pos: realEnd, level: curLv, junction: jid, onStreet: curLv == 0 && jid >= 0 && g.hasStreetAt(jid)}
	if curLv == 0 {
		g.start = jid
	} else {
		g.start = -1
	}
	if start.level != curLv {
		g.say(locale.T("已建成出入坡道（%s → %s，%.0f m），信号灯将保护路口通行", "Built portal ramp (%s -> %s, %.0f m); signals protect any road crossing"),
			levelLabel(start.level), levelLabel(curLv), dist(start.pos, realEnd))
	} else {
		g.say(locale.T("已建成%s独立轨道（%.0f m），继续点击可延伸，右键结束", "Built %s tramway (%.0f m); click on to extend, right-click to finish"),
			levelLabel(curLv), dist(start.pos, realEnd))
	}
}

// previewCorridorValid checks whether building from start to (endPos, curLv) looks valid for preview colouring.
func (g *Game) previewCorridorValid(start trackAnchor, endPos street.Vec, curLv int) bool {
	diff := start.level - curLv
	if diff < -1 || diff > 1 {
		return false
	}
	minL := 20.0
	if diff != 0 {
		minL = 36.0
	}
	if dist(start.pos, endPos) < minL {
		return false
	}
	if g.checkCorridor(start.pos, endPos, start.level, curLv) != nil {
		return false
	}
	if diff != 0 {
		// Ramps cannot cross ground roads midway.
		for _, rd := range g.net.Roads {
			if rd.A == start.junction || rd.B == start.junction {
				if !rd.Tramway {
					ra, rb := g.net.Ends(rd.ID)
					if _, d := projectOn(start.pos.Lerp(endPos, 0.5), ra, rb); d < rd.HalfWidth()+1.5 {
						return false
					}
				}
				continue
			}
			la, lb := g.net.RoadLevels(rd.ID)
			if la != 0 && lb != 0 && la != start.level && lb != start.level && la != curLv && lb != curLv {
				continue
			}
			ra, rb := g.net.Ends(rd.ID)
			if segCrossStrict(start.pos, endPos, ra, rb) {
				return false
			}
		}
	}
	return true
}

func segCrossStrict(p, q, a, b street.Vec) bool {
	r, d := sub(q, p), sub(b, a)
	den := r.X*d.Y - r.Y*d.X
	if math.Abs(den) < 1e-9 {
		return false
	}
	w := sub(a, p)
	s := (w.X*d.Y - w.Y*d.X) / den
	u := (w.X*r.Y - w.Y*r.X) / den
	return s > 0.02 && s < 0.98 && u > 0.02 && u < 0.98
}

// drawTrackToolOverlay previews street track, off-road track, viaducts,
// tunnels and level-transition ramps.
func (g *Game) drawTrackToolOverlay(c *canvas) {
	curLv := levelHeight(g.level)
	endPos, endJ, snapped := g.snapTrackPoint(curLv)

	if g.anchor == nil {
		if curLv == 0 {
			if h, ok := g.onHighway(); ok {
				g.markHighway(c, h)
				return
			}
			if j, ok := g.net.NearestJunctionAt(g.cursor, 0, math.Max(g.pick(14), 12)); ok {
				g.ring(c, g.net.Junctions[j].Pos, g.net.Radius(j), 2.5, colHover)
				return
			}
			if r, _, ok := g.net.NearestStreet(g.cursor, g.pick(12)); ok {
				g.highlightRoads(c, []street.RoadID{r}, colOK)
				return
			}
		}
		col := colAccent
		if g.checkCorridor(endPos, endPos, curLv, curLv) != nil {
			col = colBad
		} else if snapped {
			col = colHover
		}
		r := 5.0
		if endJ >= 0 {
			r = g.net.Radius(endJ)
		}
		g.ring(c, endPos, r, 2.5, col)
		return
	}

	start := *g.anchor
	g.ring(c, start.pos, 6.5, 3, colHover)

	// Street-to-street path preview on ground level.
	if start.level == 0 && curLv == 0 && start.onStreet && start.junction >= 0 && endJ >= 0 && endJ != start.junction && g.hasStreetAt(endJ) {
		if roads, ok := g.net.RoadPath(start.junction, endJ); ok && len(roads) > 0 {
			g.highlightRoads(c, roads, colOK)
			g.ring(c, g.net.Junctions[endJ].Pos, g.net.Radius(endJ), 2.5, colHover)
			return
		}
	}

	w := 2 * street.LaneWidth
	if layouts[g.layout].single {
		w = street.CentreTrackWidth
	}
	col := colBad
	if g.previewCorridorValid(start, endPos, curLv) {
		col = colOK
	}
	c.seg(start.pos, endPos, w, 4, col)
	if start.level != curLv {
		// Draw ramp direction chevrons along the previewed portal ramp.
		l := dist(start.pos, endPos)
		u := unit(sub(endPos, start.pos))
		rn := street.Vec{X: -u.Y, Y: u.X}
		for s := 8.0; s < l-6; s += 10 {
			p := add(start.pos, scale(u, s))
			c.seg(add(add(p, scale(u, -2.2)), scale(rn, -w*0.35)), add(p, scale(u, 1.5)), 0.35, 1.5, rgba(255, 255, 255, 200))
			c.seg(add(add(p, scale(u, -2.2)), scale(rn, w*0.35)), add(p, scale(u, 1.5)), 0.35, 1.5, rgba(255, 255, 255, 200))
		}
	}
	g.ring(c, endPos, 5.5, 2.5, colHover)
}

// --- Multi-level tramway infrastructure rendering ---------------------------

var (
	colBallast       = rgb(116, 120, 124)
	colBallastEdge   = rgb(164, 170, 174)
	colViaductDeck   = rgb(196, 200, 206)
	colViaductParap  = rgb(224, 226, 230)
	colViaductTrack  = rgb(132, 138, 144)
	colViaductPier   = rgb(142, 146, 152)
	colTunnelTrace   = rgba(68, 132, 162, 72)
	colTunnelCutaway = rgba(18, 26, 36, 228)
	colTunnelWall    = rgb(88, 106, 124)
	colTunnelFloor   = rgb(38, 46, 56)
)

// drawUndergroundOnGround shows underground subway corridors faintly on the
// surface view, or opens an illuminated subterranean cutaway around the
// underground network while the player follows a tram inside a tunnel.
func (g *Game) drawUndergroundOnGround(c *canvas) {
	cutaway := g.followedTramUnderground()
	hasUnder := false
	for _, rd := range g.net.Roads {
		la, lb := g.net.RoadLevels(rd.ID)
		if la < 0 || lb < 0 {
			hasUnder = true
			break
		}
	}
	if !hasUnder {
		return
	}
	if !cutaway {
		for _, rd := range g.net.Roads {
			la, lb := g.net.RoadLevels(rd.ID)
			if la >= 0 || lb >= 0 || !g.roadVisible(c, rd.ID) {
				continue
			}
			a, b := g.net.Ends(rd.ID)
			w := rd.HalfWidth()*2 + 1.2
			dashed(c, a, b, 6, 4, w, colTunnelTrace)
		}
		for _, id := range g.net.StopIDs() {
			st := g.net.Stops[id]
			if g.net.StopLevel(st) >= 0 || !c.visibleAround(g.net.StopPos(st), 50) {
				continue
			}
			for _, p := range g.net.Platforms(st) {
				l := &g.net.Links[p.Link]
				if l.Twin >= 0 && l.Twin < l.ID {
					continue
				}
				if isl, _, ok := g.platformLine(p); ok {
					c.polyline(isl, 2.0, 2, rgba(84, 186, 204, 110))
				}
			}
		}
		return
	}
}

// drawUndergroundCutaway renders an illuminated x-ray cutaway of the
// underground tunnels, stations, signals and trams over the surface map
// whenever the followed tram is running underground.
func (g *Game) drawUndergroundCutaway(c *canvas) {
	if !g.followedTramUnderground() {
		return
	}
	g.drawTunnelNetwork(c, true)
	g.drawTrackAtLevel(c, -1)
	g.drawStopsAtLevel(c, -1)
	g.drawTramSignalsAt(c, -1)
	g.drawVehiclesBand(c, -1.5, -0.55, false)
}

// drawTunnelNetwork paints concrete tunnel bores and underground station boxes.
func (g *Game) drawTunnelNetwork(c *canvas, cutaway bool) {
	n := g.net
	bg := colTunnelFloor
	if cutaway {
		bg = colTunnelCutaway
	}
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if (la >= 0 && lb >= 0) || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		if la < 0 && lb == 0 {
			a, b = b, a // a is ground (0), b is tunnel (-1)
		}
		hw := rd.HalfWidth()
		if la != lb {
			// Portal mouth join into tunnel bore.
			c.circle(b, hw+1.4, colTunnelWall)
			c.circle(b, hw+0.8, bg)
			continue
		}
		c.seg(a, b, 2*hw+2.8, 3, colTunnelWall)
		c.circle(a, hw+1.4, colTunnelWall)
		c.circle(b, hw+1.4, colTunnelWall)
	}
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if la >= 0 || lb >= 0 || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		hw := rd.HalfWidth()
		c.seg(a, b, 2*hw+1.4, 2, bg)
		c.circle(a, hw+0.7, bg)
		c.circle(b, hw+0.7, bg)
	}
	// Station cavern widenings for underground stops.
	for _, id := range n.StopIDs() {
		st := n.Stops[id]
		if n.StopLevel(st) >= 0 || !c.visibleAround(n.StopPos(st), 60) {
			continue
		}
		rd := n.Roads[st.Road]
		a, b := n.Ends(st.Road)
		u := unit(sub(b, a))
		pos := n.StopPos(st)
		plen := street.PlatformLength(st.Capacity()) + 4
		cw := rd.HalfWidth()*2 + 6.4
		c.bevelBox(pos, u, plen+1.6, cw+1.2, 0.8, colTunnelWall)
		c.bevelBox(pos, u, plen, cw, 0.6, rgb(48, 58, 70))
	}
}

// drawGroundTramways draws off-road ground trackbeds, street level-crossing
// aprons and the open U-trough ramps of underground tunnel portals.
func (g *Game) drawGroundTramways(c *canvas) {
	n := g.net
	for _, rd := range n.Roads {
		if !rd.Tramway || !g.roadVisible(c, rd.ID) {
			continue
		}
		la, lb := n.RoadLevels(rd.ID)
		a, b := n.Ends(rd.ID)
		hw := rd.HalfWidth()
		switch {
		case la == 0 && lb == 0:
			c.seg(a, b, 2*hw+1.2, 2, colBallastEdge)
			c.circle(a, hw+0.6, colBallastEdge)
			c.circle(b, hw+0.6, colBallastEdge)
			c.seg(a, b, 2*hw+0.4, 1.5, colTrackBed)
			c.circle(a, hw+0.2, colTrackBed)
			c.circle(b, hw+0.2, colTrackBed)
		case (la == 0 && lb < 0) || (la < 0 && lb == 0):
			// Open U-trough tunnel portal descending from g0 (ground) to g1 (tunnel).
			g0, g1 := a, b
			if la < 0 {
				g0, g1 = b, a
			}
			u := unit(sub(g1, g0))
			rn := street.Vec{X: -u.Y, Y: u.X}
			// Retaining walls along both sides.
			c.seg(g0, g1, 2*hw+1.8, 2.5, rgb(184, 188, 194))
			// Graded ramp floor darkening towards the tunnel mouth.
			const steps = 8
			for k := 0; k < steps; k++ {
				t0, t1 := float64(k)/steps, float64(k+1)/steps
				col := tint(rgb(136, 142, 148), rgb(18, 22, 28), (t0+t1)*0.5)
				c.seg(g0.Lerp(g1, t0), g0.Lerp(g1, t1), 2*hw+0.3, 1.5, col)
			}
			// Inner wall shadow lines.
			for _, sd := range []float64{-1, 1} {
				w0 := add(g0, scale(rn, sd*(hw+0.25)))
				w1 := add(g1, scale(rn, sd*(hw+0.25)))
				c.seg(w0, w1, 0.28, 0.9, rgba(10, 14, 20, 140))
			}
		}
	}
	// Paved crossing aprons where an off-road tramway meets a car street at level 0.
	for _, j := range n.Junctions {
		if j.Level != 0 || !c.visibleAround(j.Pos, n.Radius(j.ID)+15) {
			continue
		}
		hasStr, hasTw := false, false
		for _, r := range n.RoadsAt(j.ID) {
			if n.Roads[r].Tramway {
				hasTw = true
			} else {
				hasStr = true
			}
		}
		if hasStr && hasTw {
			for _, r := range n.RoadsAt(j.ID) {
				rd := n.Roads[r]
				if !rd.Tramway {
					continue
				}
				a, b := n.Ends(r)
				other := b
				if rd.B == j.ID {
					other = a
				}
				d := unit(sub(other, j.Pos))
				c.seg(j.Pos, add(j.Pos, scale(d, n.Radius(j.ID)+1.2)), rd.HalfWidth()*2+0.8, 2, rgb(124, 130, 136))
			}
		}
	}
}

// drawPortalHeadwalls paints the concrete portal arch beam across the deep end
// of every tunnel portal, drawn AFTER ground/portal vehicles so trams pass
// visually underneath the portal headwall as they enter or leave the tunnel.
func (g *Game) drawPortalHeadwalls(c *canvas) {
	n := g.net
	for _, rd := range n.Roads {
		if !rd.Tramway || !g.roadVisible(c, rd.ID) {
			continue
		}
		la, lb := n.RoadLevels(rd.ID)
		if !((la == 0 && lb < 0) || (la < 0 && lb == 0)) {
			continue
		}
		a, b := n.Ends(rd.ID)
		g0, g1 := a, b
		if la < 0 {
			g0, g1 = b, a
		}
		u := unit(sub(g1, g0))
		hw := rd.HalfWidth()
		portalPos := g0.Lerp(g1, 0.88)
		c.box(add(portalPos, street.Vec{X: 0.4, Y: 0.5}), u, 3.6, 2*hw+2.4, rgba(8, 12, 18, 130))
		c.bevelBox(portalPos, u, 3.4, 2*hw+2.2, 0.35, rgb(196, 200, 206))
		c.box(portalPos, u, 2.2, 2*hw+1.4, rgb(156, 162, 168))
	}
}

// drawElevatedLayer renders elevated approach ramps (0 <-> +1), elevated
// viaducts (+1) with piers and drop shadows, elevated stations, signals and
// elevated trams above the ground scene.
func (g *Game) drawElevatedLayer(c *canvas) {
	n := g.net
	hasElev := false
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if la > 0 || lb > 0 {
			hasElev = true
			break
		}
	}
	if !hasElev {
		return
	}
	shadOff := scale(sunShadow, 9.5)

	// 1. Viaduct and ramp shadows on the ground below.
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if (la <= 0 && lb <= 0) || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		hw := rd.HalfWidth() + 0.9
		if la > 0 && lb > 0 {
			sa, sb := add(a, shadOff), add(b, shadOff)
			c.seg(sa, sb, 2*hw, 2, colViaductShad)
			c.circle(sa, hw, colViaductShad)
			c.circle(sb, hw, colViaductShad)
		} else {
			// Tapered shadow rising from 0 at ground end to shadOff at viaduct end.
			g0, g1 := a, b
			if la > 0 {
				g0, g1 = b, a
			}
			l := dist(g0, g1)
			rn := street.Vec{X: -(g1.Y - g0.Y) / math.Max(l, 1e-6), Y: (g1.X - g0.X) / math.Max(l, 1e-6)}
			s1 := add(g1, shadOff)
			c.fan([]street.Vec{
				add(g0, scale(rn, hw)), add(s1, scale(rn, hw)),
				sub(s1, scale(rn, hw)), sub(g0, scale(rn, hw)),
			}, colViaductShad)
		}
	}

	// 2. Concrete piers under flat +1 viaducts (skipping water & street carriageways).
	if c.px() >= 0.7 {
		for _, rd := range n.Roads {
			la, lb := n.RoadLevels(rd.ID)
			if la <= 0 || lb <= 0 || !g.roadVisible(c, rd.ID) {
				continue
			}
			a, b := n.Ends(rd.ID)
			l := dist(a, b)
			for s := 14.0; s < l-10; s += 28.0 {
				p := a.Lerp(b, s/l)
				if g.waterAt(p) || g.onAnyStreetCarriageway(p) {
					continue
				}
				c.circle(add(p, street.Vec{X: 0.7, Y: 0.9}), 1.05, rgba(0, 0, 0, 80))
				c.circle(p, 1.05, colViaductPier)
			}
		}
	}

	// 3. Elevated approach ramps (0 <-> +1) & viaduct decks (+1).
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if (la <= 0 && lb <= 0) || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		hw := rd.HalfWidth()
		if la != lb {
			g0, g1 := a, b
			if la > 0 {
				g0, g1 = b, a
			}
			c.seg(g0, g1, 2*hw+1.8, 2.5, colViaductParap)
			const steps = 6
			for k := 0; k < steps; k++ {
				t0, t1 := float64(k)/steps, float64(k+1)/steps
				col := tint(rgb(154, 160, 166), colViaductDeck, (t0+t1)*0.5)
				c.seg(g0.Lerp(g1, t0), g0.Lerp(g1, t1), 2*hw+0.6, 1.5, col)
			}
			c.seg(g0, g1, 2*hw-0.2, 1.2, colViaductTrack)
			continue
		}
		c.seg(a, b, 2*hw+1.8, 2.5, colViaductParap)
		c.circle(a, hw+0.9, colViaductParap)
		c.circle(b, hw+0.9, colViaductParap)
	}
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if la <= 0 || lb <= 0 || !g.roadVisible(c, rd.ID) {
			continue
		}
		a, b := n.Ends(rd.ID)
		hw := rd.HalfWidth()
		c.seg(a, b, 2*hw+0.8, 2, colViaductDeck)
		c.circle(a, hw+0.4, colViaductDeck)
		c.circle(b, hw+0.4, colViaductDeck)
		c.seg(a, b, 2*hw-0.2, 1.2, colViaductTrack)
		c.circle(a, hw-0.1, colViaductTrack)
		c.circle(b, hw-0.1, colViaductTrack)
	}

	// Widened station decks for elevated stops.
	for _, id := range n.StopIDs() {
		st := n.Stops[id]
		if n.StopLevel(st) <= 0 || !c.visibleAround(n.StopPos(st), 60) {
			continue
		}
		rd := n.Roads[st.Road]
		a, b := n.Ends(st.Road)
		u := unit(sub(b, a))
		pos := n.StopPos(st)
		plen := street.PlatformLength(st.Capacity()) + 4
		cw := rd.HalfWidth()*2 + 5.8
		c.bevelBox(add(pos, shadOff), u, plen, cw, 0.8, colViaductShad)
		c.bevelBox(pos, u, plen+0.8, cw+0.8, 0.7, colViaductParap)
		c.bevelBox(pos, u, plen, cw, 0.6, colViaductDeck)
	}

	// 4. Rails, sleepers, stops, signals and trams on the elevated layer.
	g.drawTrackAtLevel(c, 1)
	g.drawStopsAtLevel(c, 1)
	g.drawTramSignalsAt(c, 1)
	g.drawVehiclesBand(c, 0.201, 1.5, false)
}

// drawActiveLevelTramways renders full-detail tramways, portals, stops,
// signals, vehicles and build overlays when viewing Elevated (+1) or
// Underground (-1) explicitly via L.
func (g *Game) drawActiveLevelTramways(c *canvas, h int) {
	// Faint ground-level tram tracks for easy portal alignment.
	for i := range g.net.Links {
		l := &g.net.Links[i]
		if l.Modes&street.Tram == 0 || (l.Kind == street.LaneLink && !l.Track) || (l.Twin >= 0 && l.Twin < l.ID) {
			continue
		}
		if g.linkLevelBand(l) != 0 || !c.visibleAround(l.Pts[0], l.Length+5) {
			continue
		}
		c.polyline(l.Pts, 1.2, 1.5, rgba(84, 186, 204, 95))
	}
	for _, j := range g.net.Junctions {
		if j.Level == 0 && c.visibleAround(j.Pos, 15) {
			for _, r := range g.net.RoadsAt(j.ID) {
				if g.net.Roads[r].Tram != street.NoTram {
					c.circle(j.Pos, 2.2, rgba(84, 186, 204, 120))
					break
				}
			}
		}
	}

	switch {
	case h > 0:
		g.drawElevatedLayer(c)
	case h < 0:
		g.drawGroundTramways(c) // shows the tunnel portals connecting to ground
		g.drawTunnelNetwork(c, false)
		g.drawTrackAtLevel(c, -1)
		g.drawStopsAtLevel(c, -1)
		g.drawTramSignalsAt(c, -1)
		g.drawVehiclesBand(c, -1.5, -0.01, false)
		g.drawPortalHeadwalls(c)
	}
	g.drawToolOverlay(c)
}

// linkLevelBand classifies a tram link into -1 (underground), 0 (ground +
// underground portals) or +1 (elevated + elevated ramps).
func (g *Game) linkLevelBand(l *street.Link) int {
	if l.Kind == street.ConnectorLink {
		return g.net.Junctions[l.Junction].Level
	}
	la, lb := g.net.RoadLevels(l.Road)
	if la > 0 || lb > 0 {
		return 1
	}
	if la < 0 && lb < 0 {
		return -1
	}
	return 0 // flat ground or 0 <-> -1 open tunnel portal
}

func (g *Game) drawTrackAtLevel(c *canvas, band int) {
	n := g.net
	sleepers := c.px() > 3
	for i := range n.Links {
		l := &n.Links[i]
		if l.Modes&street.Tram == 0 || (l.Kind == street.LaneLink && !l.Track) {
			continue
		}
		if l.Twin >= 0 && l.Twin < l.ID {
			continue
		}
		if g.linkLevelBand(l) != band || !c.visibleAround(l.Pts[0], l.Length+5) {
			continue
		}
		own := l.Kind == street.LaneLink && l.Modes == street.Tram || g.bendTrack(l)
		if sleepers && own {
			slCol := colSleeper
			if band != 0 {
				slCol = rgb(92, 98, 106)
			}
			for s := 0.5; s < l.Length; s += 0.9 {
				p, d := l.PointAt(s)
				c.box(p, d, 0.22, 2.3, slCol)
			}
		}
		rail := colRail
		if l.Kind == street.ConnectorLink && !own {
			rail = rgb(120, 128, 136)
		}
		for _, side := range []float64{-0.72, 0.72} {
			c.polyline(offset(l.Pts, side), 0.11, 0.8, rail)
		}
	}
	if c.px() < 1.2 || band < 0 {
		return // underground tunnels use ceiling conductor rails
	}
	for _, rd := range n.Roads {
		if rd.Tram == street.NoTram || !g.roadVisible(c, rd.ID) {
			continue
		}
		la, lb := n.RoadLevels(rd.ID)
		rdBand := 0
		if la > 0 || lb > 0 {
			rdBand = 1
		} else if la < 0 && lb < 0 {
			rdBand = -1
		}
		if rdBand != band {
			continue
		}
		a, b := n.Ends(rd.ID)
		d := unit(sub(b, a))
		right := street.Vec{X: -d.Y, Y: d.X}
		l := dist(a, b)
		hw := rd.HalfWidth()
		r0, r1 := n.Radius(rd.A), l-n.Radius(rd.B)
		for s := r0 + 6; s < r1-4; s += 32 {
			p := a.Lerp(b, s/l)
			if rd.Class.Median() > 0 && !rd.Single && !rd.Tramway {
				c.circle(p, 0.3, rgb(60, 66, 72))
				c.seg(add(p, scale(right, -hw+0.5)), add(p, scale(right, hw-0.5)), 0.06, 0.6, rgba(20, 24, 28, 140))
				continue
			}
			c.seg(add(p, scale(right, -hw-0.6)), add(p, scale(right, hw+0.6)), 0.06, 0.6, rgba(20, 24, 28, 140))
			c.circle(add(p, scale(right, -hw-0.6)), 0.3, rgb(60, 66, 72))
			c.circle(add(p, scale(right, hw+0.6)), 0.3, rgb(60, 66, 72))
		}
	}
}

func (g *Game) drawStopsAtLevel(c *canvas, level int) {
	n := g.net
	for _, id := range n.StopIDs() {
		st := n.Stops[id]
		if n.StopLevel(st) != level || !c.visibleAround(n.StopPos(st), 60) {
			continue
		}
		for _, p := range n.Platforms(st) {
			l := &n.Links[p.Link]
			if l.Twin >= 0 && l.Twin < l.ID {
				continue
			}
			isl, side, ok := g.platformLine(p)
			if !ok {
				continue
			}
			edge := offset(isl, -math.Copysign(0.75, side))
			platCol := rgb(196, 198, 190)
			if level < 0 {
				platCol = rgb(168, 178, 188)
			}
			c.polyline(isl, 1.8, 2, platCol)
			c.polyline(edge, 0.3, 0.8, colYellow)
			mid := isl[0].Lerp(isl[len(isl)-1], 0.5)
			dir := unit(sub(isl[len(isl)-1], isl[0]))
			c.box(add(mid, street.Vec{X: 0.4, Y: 0.5}), dir, 8, 1.4, colShadow)
			c.box(mid, dir, 8, 1.4, rgb(46, 66, 80))
			c.box(mid, dir, 7.4, 0.9, rgb(98, 140, 160))
		}
	}
}
