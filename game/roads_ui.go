package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"math"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// The Road tool (Build page, key 5) edits the city's streets: build a new
// street between two points on existing streets, demolish a street, or
// change its lanes. Motorways and their ramps cannot be edited yet, and
// streets carrying tram track must have the track removed first.

const (
	roadNew = iota
	roadRemove
	roadLanes
)

func roadModeNames() []string {
	return locale.L([]string{"新建", "拆除", "车道"}, []string{"Build", "Demolish", "Lanes"})
}

// roadLaneOpts are the lane layouts the tool builds or applies.
var roadLaneOpts = []struct {
	zh, en, short string
	fwd, back     int
}{
	{"单行 1 车道", "one-way, 1 lane", "1→", 1, 0}, {"单行 2 车道", "one-way, 2 lanes", "2→", 2, 0},
	{"双向 1+1", "two-way 1+1", "1+1", 1, 1}, {"双向 2+2", "two-way 2+2", "2+2", 2, 2}, {"双向 3+3", "two-way 3+3", "3+3", 3, 3},
}

func laneOptName(i int) string { return locale.T(roadLaneOpts[i].zh, roadLaneOpts[i].en) }

const defaultRoadLanes = 2

func highwayLocked() string {
	return locale.T("高速公路（含匝道）暂时不能修改", "Motorways and their ramps can't be changed yet")
}

// roadPoint is where a new street would join under the cursor: a ground street
// junction, or the nearest point on a street.
func (g *Game) roadPoint() (street.Vec, bool) {
	if j, ok := g.hoverJunction(); ok && g.hasStreetAt(j) {
		return g.net.Junctions[j].Pos, true
	}
	if r, t, ok := g.net.NearestStreet(g.cursor, g.pick(12)); ok {
		a, b := g.net.Ends(r)
		return a.Lerp(b, t), true
	}
	return street.Vec{}, false
}

func (g *Game) roadTool(left, right bool) {
	if right {
		g.roadStart = nil
	}
	if !left {
		return
	}
	if _, ok := g.onHighway(); ok {
		g.say("%s", highwayLocked())
		return
	}
	opt := roadLaneOpts[g.roadLanes]
	done := func(err error, msg string, args ...any) {
		if err != nil {
			g.say("%v", err)
			return
		}
		g.start = -1
		g.edited()
		g.say(msg, args...)
	}
	switch g.roadMode {
	case roadNew:
		p, ok := g.roadPoint()
		if !ok {
			g.say("%s", locale.T("新道路要从现有道路或路口开始，到现有道路或路口结束", "A new road must start and end on an existing road or junction"))
			return
		}
		if g.roadStart == nil {
			g.roadStart = &p
			g.say("%s", locale.T("已选起点：再点击终点（右键取消）", "Start chosen: now click the end (right-click cancels)"))
			return
		}
		a := *g.roadStart
		g.roadStart = nil
		done(g.net.AddRoad(a, p, opt.fwd, opt.back, 1), locale.T("已建成新道路（%s，%.0f m）", "New road built (%s, %.0f m)"), laneOptName(g.roadLanes), dist(a, p))
	case roadRemove:
		r, _, ok := g.net.NearestStreet(g.cursor, g.pick(12))
		if !ok {
			return
		}
		name := g.roadLabel(r)
		done(g.net.RemoveRoad(r), locale.T("已拆除 %s", "Demolished %s"), name)
	case roadLanes:
		r, _, ok := g.net.NearestStreet(g.cursor, g.pick(12))
		if !ok {
			return
		}
		rd := g.net.Roads[r]
		fwd, back := opt.fwd, opt.back
		if rd.OneWay() && back == 0 && rd.Lanes(true) == fwd {
			// Already like this: clicking again turns the one-way round.
			fwd, back = 0, fwd
		}
		name := g.roadLabel(r)
		done(g.net.SetRoadLanes(r, fwd, back), "%s：%s", name, lanesText(fwd, back))
	}
}

func lanesText(fwd, back int) string {
	if fwd == 0 || back == 0 {
		return fmt.Sprintf(locale.T("单行 %d 车道", "one-way, %d lanes"), max(fwd, back))
	}
	return fmt.Sprintf(locale.T("双向 %d+%d 车道", "two-way, %d+%d lanes"), fwd, back)
}

func (g *Game) roadLabel(r street.RoadID) string {
	if n := g.net.Roads[r].Name; n != "" {
		return n
	}
	return locale.T("这段道路", "this road")
}

// drawRoadOverlay previews the Road tool.
func (g *Game) drawRoadOverlay(c *canvas) {
	if h, ok := g.onHighway(); ok {
		g.markHighway(c, h)
		return
	}
	switch g.roadMode {
	case roadNew:
		p, ok := g.roadPoint()
		if ok {
			g.ring(c, p, 6, 2, colHover)
		}
		if g.roadStart != nil {
			g.ring(c, *g.roadStart, 7, 3, colHover)
			end := g.cursor
			col := colBad
			if ok {
				end, col = p, colOK
			}
			opt := roadLaneOpts[g.roadLanes]
			w := float64(opt.fwd+opt.back) * street.LaneWidth
			c.seg(*g.roadStart, end, math.Max(w, 3), 4, col)
		}
	case roadRemove, roadLanes:
		if r, _, ok := g.net.NearestStreet(g.cursor, g.pick(12)); ok {
			col := colAccent
			if g.roadMode == roadRemove {
				col = colBad
			}
			g.highlightRoads(c, []street.RoadID{r}, rgba(col.R, col.G, col.B, 170))
		}
	}
}
