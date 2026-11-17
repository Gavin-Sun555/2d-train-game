package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// Landmarks: well-known places (the Space Needle, King Street Station...)
// are labelled on the map. Clicking a label renames it; the names are kept
// in the save, keyed by the place's OpenStreetMap name.

type landmark struct {
	name string // default name
	key  string // stable id: the OpenStreetMap name
	p    street.Vec
	rank int
}

func newLandmarks(m *mapdata.Map) []landmark {
	var out []landmark
	for _, l := range m.Landmarks {
		key := l.OSM
		if key == "" {
			key = l.Name
		}
		out = append(out, landmark{name: l.Name, key: key, p: vec(l.P), rank: max(l.Rank, 1)})
	}
	return out
}

const MaxLandmarkName = 24 // runes

// LandmarkName is the landmark's shown name: the player's, or the default.
func (g *Game) LandmarkName(i int) string {
	l := g.scene.landmarks[i]
	if n, ok := g.landmarkNames[l.key]; ok {
		return n
	}
	return g.landmarkDefault(i)
}

// landmarkDefault is the landmark's own name: Chinese, or in English its
// OpenStreetMap name.
func (g *Game) landmarkDefault(i int) string {
	l := g.scene.landmarks[i]
	return locale.T(l.name, l.key)
}

// RenameLandmark gives a landmark a new name; an empty name restores the
// default.
func (g *Game) RenameLandmark(i int, name string) error {
	name, err := cleanSlotName(name)
	if err != nil {
		return err
	}
	l := g.scene.landmarks[i]
	if name == "" || name == l.name || name == l.key {
		delete(g.landmarkNames, l.key)
		return nil
	}
	if g.landmarkNames == nil {
		g.landmarkNames = map[string]string{}
	}
	g.landmarkNames[l.key] = name
	return nil
}

type landmarkLabel struct {
	i int
	r rect
}

// renameBox is the small window for renaming a landmark.
type renameBox struct {
	i                     int
	stop                  street.StopID // renaming a stop (-1: a landmark)
	name                  []rune
	msg                   string
	panel, ok, reset, esc rect
}

// drawLandmarks labels the landmarks in view; major ones from further out.
func (g *Game) drawLandmarks(dst *ebiten.Image) {
	g.lmLabels = g.lmLabels[:0]
	if g.scene == nil {
		return
	}
	// Major landmarks are placed first; a label that would cover one
	// already placed is left out at this zoom.
	order := make([]int, len(g.scene.landmarks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return g.scene.landmarks[order[a]].rank < g.scene.landmarks[order[b]].rank })
	overlaps := func(r rect) bool {
		for _, o := range g.lmLabels {
			if r.x0 < o.r.x1+4 && o.r.x0 < r.x1+4 && r.y0 < o.r.y1+2 && o.r.y0 < r.y1+2 {
				return true
			}
		}
		return false
	}
	for _, i := range order {
		l := g.scene.landmarks[i]
		if g.zoom < 0.22 || (l.rank > 1 && g.zoom < 0.6) {
			continue
		}
		x, y := g.toScreen(l.p)
		if x < -150 || y < -40 || int(x) > g.w+150 || int(y) > g.h+40 {
			continue
		}
		size := 13.0
		if l.rank == 1 {
			size = 14
		}
		w, _ := textui.MeasureText(g.LandmarkName(i), size)
		r := rect{x - float32(w)/2 - 18, y - 11, x + float32(w)/2 + 8, y + 11}
		if !overlaps(r) {
			g.lmLabels = append(g.lmLabels, landmarkLabel{i, r})
		}
	}
	mx, my := ebiten.CursorPosition()
	for _, lb := range g.lmLabels {
		r, i := lb.r, lb.i
		size := 13.0
		if g.scene.landmarks[i].rank == 1 {
			size = 14
		}
		y := (r.y0 + r.y1) / 2
		bg := rgba(62, 40, 78, 215)
		if r.has(mx, my) {
			bg = rgba(96, 62, 120, 235)
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		// A small star-like marker.
		cx, cy := r.x0+10, y
		vector.FillRect(dst, cx-4, cy-1, 8, 2, rgb(255, 214, 120), false)
		vector.FillRect(dst, cx-1, cy-4, 2, 8, rgb(255, 214, 120), false)
		vector.FillRect(dst, cx-2, cy-2, 4, 4, rgb(255, 214, 120), false)
		text(dst, g.LandmarkName(i), float64(r.x0)+18, float64(y)-size/2-2, size, rgb(255, 236, 200))
	}
}

// OpenRename opens the rename window of a stop (by its name) or a landmark
// (by its OpenStreetMap name); used by command-line flags.
func (g *Game) OpenRename(key string) {
	for _, id := range g.net.StopIDs() {
		if g.net.Stops[id].Name == key {
			g.openStopRename(id)
			return
		}
	}
	if g.scene == nil {
		return
	}
	for i, l := range g.scene.landmarks {
		if l.key == key {
			g.rename = &renameBox{i: i, stop: -1, name: []rune(g.LandmarkName(i))}
		}
	}
}

// landmarkClicked opens the rename window if a click hit a landmark label.
func (g *Game) landmarkClicked(mx, my int) bool {
	for k := len(g.lmLabels) - 1; k >= 0; k-- {
		if l := g.lmLabels[k]; l.r.has(mx, my) {
			g.rename = &renameBox{i: l.i, stop: -1, name: []rune(g.LandmarkName(l.i))}
			return true
		}
	}
	return false
}

func (g *Game) updateRename() {
	b := g.rename
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 0x20 && r != 0x7f && len(b.name) < MaxLandmarkName {
			b.name = append(b.name, r)
		}
	}
	bs := inpututil.KeyPressDuration(ebiten.KeyBackspace)
	if len(b.name) > 0 && (bs == 1 || bs > 20 && bs%3 == 0) {
		b.name = b.name[:len(b.name)-1]
	}
	apply := func(name string) {
		if b.stop >= 0 {
			if err := g.RenameStop(b.stop, name); err != nil {
				b.msg = err.Error()
				return
			}
			g.rename = nil
			g.say(locale.T("车站已改名：%s", "Stop renamed: %s"), g.net.Stops[b.stop].Name)
			return
		}
		if err := g.RenameLandmark(b.i, name); err != nil {
			b.msg = err.Error()
			return
		}
		g.rename = nil
		g.say(locale.T("已改名：%s", "Renamed: %s"), g.LandmarkName(b.i))
	}
	click := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	mx, my := ebiten.CursorPosition()
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape), click && (b.esc.has(mx, my) || !b.panel.has(mx, my)):
		g.rename = nil
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter), click && b.ok.has(mx, my):
		apply(string(b.name))
	case click && b.stop < 0 && b.reset.has(mx, my):
		apply("")
	}
}

func (g *Game) drawRename(dst *ebiten.Image) {
	b := g.rename
	if b == nil {
		return
	}
	vector.FillRect(dst, 0, 0, float32(g.w), float32(g.h), rgba(0, 0, 0, 110), false)
	const pw, ph = 440, 170
	x0, y0 := float32(g.w-pw)/2, float32(g.h-ph)/2
	b.panel = rect{x0, y0, x0 + pw, y0 + ph}
	vector.FillRect(dst, x0, y0, pw, ph, colPanel, false)
	vector.StrokeRect(dst, x0, y0, pw, ph, 2, rgb(186, 140, 220), false)
	if b.stop >= 0 {
		g.drawStopRename(dst, b, x0, y0, pw, ph)
		return
	}
	l := g.scene.landmarks[b.i]
	text(dst, locale.T("地标改名", "Rename landmark"), float64(x0+20), float64(y0+16), 18, color.White)
	orig := locale.T("原名：", "Original name: ") + g.landmarkDefault(b.i)
	if other := locale.T(l.key, l.name); other != g.landmarkDefault(b.i) {
		orig += locale.T("（", " (") + other + locale.T("）", ")")
	}
	text(dst, clip(orig, 12, pw-40), float64(x0+20), float64(y0+44), 12, colDim)
	// Text field with a blinking caret.
	vector.FillRect(dst, x0+20, y0+66, pw-40, 32, rgb(14, 20, 28), false)
	vector.StrokeRect(dst, x0+20, y0+66, pw-40, 32, 1, colAccent, false)
	s := string(b.name)
	if (ebiten.Tick()/30)%2 == 0 {
		s += "|"
	}
	text(dst, s, float64(x0+30), float64(y0+73), 15, color.White)
	if b.msg != "" {
		text(dst, b.msg, float64(x0+20), float64(y0+104), 12, colHover)
	} else {
		text(dst, fmt.Sprintf(locale.T("回车确定，Esc 取消；最多 %d 个字，清空即恢复原名", "Enter to confirm, Esc to cancel; up to %d characters, empty restores the original"), MaxLandmarkName), float64(x0+20), float64(y0+104), 12, colDim)
	}
	var btns []hudButton
	bx := float64(x0 + pw - 20)
	for _, it := range []struct {
		label string
		r     *rect
	}{{locale.T("取消", "Cancel"), &b.esc}, {locale.T("恢复原名", "Original"), &b.reset}, {locale.T("确定", "OK"), &b.ok}} {
		w, _ := textui.MeasureText(it.label, 13)
		bx -= w + 14
		style := btnPlain
		if it.label == locale.T("确定", "OK") {
			style = btnOn
		}
		buttonAt(dst, &btns, bx, float64(y0+ph-44), 28, it.label, style, nil)
		*it.r = btns[len(btns)-1].r
		bx -= 8
	}
}

// landmarkSave is what the save keeps: the renamed landmarks.
func (g *Game) landmarkSave() map[string]string {
	if len(g.landmarkNames) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range g.landmarkNames {
		out[k] = strings.TrimSpace(v)
	}
	return out
}
