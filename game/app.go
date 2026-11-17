package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// mapInfo describes a map on the start screen.
type mapInfo struct {
	key                              string
	titleZH, titleEN, descZH, descEN string
}

func (m mapInfo) title() string { return locale.T(m.titleZH, m.titleEN) }
func (m mapInfo) desc() string  { return locale.T(m.descZH, m.descEN) }

var maps = []mapInfo{
	{"grid", "01  网格新城", "01  Grid City", "规整的生成城市：支路、主干道、大道与河流，适合练手",
		"A generated city of streets, avenues and a river: good for practice"},
	{"seattle", "02  西雅图 · 中心城区", "02  Seattle · central city",
		"真实路网与建筑（OpenStreetMap），约 43 平方公里，含 SLU 有轨电车示例",
		"Real OpenStreetMap streets and buildings, 43 sq km, SLU Streetcar"},
}

// App is the whole program: a map selection screen in front of one game
// per map. Leaving a game with Esc keeps its state for coming back.
type App struct {
	games  []*Game
	cur    int // index of the open game, -1 on the start screen
	sel    int
	w, h   int
	thumbs []*ebiten.Image
	start  []rect
	demo   []rect
	reset  []rect
	cards  []rect // whole map cards, for click-to-select
	slotsB []rect // "saves" buttons on the cards
	langB  []rect // the language switch: 中文, English
	// slotMap is the map whose save window is open over the menu (-1).
	slotMap int
	// confirm is the map whose "start over" button was clicked once (-1).
	confirm int
	notice  string // load errors to show on the start screen
	// NoSave turns saving off (demos and screenshots never overwrite the
	// player's progress).
	NoSave bool
}

// NewApp opens every map with its saved progress; with noSave nothing is
// ever written.
func NewApp(noSave bool) *App {
	a := &App{cur: -1, w: 1280, h: 720, confirm: -1, slotMap: -1, NoSave: noSave}
	for _, m := range maps {
		g := New(m.key)
		if _, err := g.Load(); err != nil {
			a.notice = fmt.Sprintf(locale.T("%s 存档无法读取：%v", "%s: the save can't be read: %v"), m.title(), err)
		}
		a.games = append(a.games, g)
	}
	return a
}

// Open enters a map directly (optionally with a fresh sample network).
func (a *App) Open(key string, demo bool) error {
	for i, m := range maps {
		if m.key == key {
			a.cur = i
			if demo {
				a.games[i] = New(key)
				a.games[i].Layout(a.w, a.h)
				a.thumbs = nil
				return a.games[i].LoadDemo()
			}
			return nil
		}
	}
	return fmt.Errorf("unknown map %q", key)
}

// SaveAll writes every map's progress.
func (a *App) SaveAll() {
	if a.NoSave {
		return
	}
	for _, g := range a.games {
		if g.autoAt.IsZero() { // not opened this run: nothing new to keep
			if err := g.Save(false); err != nil {
				a.notice = fmt.Sprintf(locale.T("保存失败：%v", "Saving failed: %v"), err)
			}
			continue
		}
		if err := g.exitSave(); err != nil {
			a.notice = fmt.Sprintf(locale.T("自动存档失败：%v", "Autosave failed: %v"), err)
		}
	}
}

// openSlots shows a map's save window over the menu, to load a save.
func (a *App) openSlots(i int) {
	a.games[i].noSave = a.NoSave
	a.games[i].openSlots(false)
	a.slotMap, a.confirm = i, -1
}

// restart throws a map's progress away.
func (a *App) restart(i int) {
	if !a.NoSave {
		if err := DeleteSave(maps[i].key); err != nil {
			a.notice = fmt.Sprintf(locale.T("无法删除存档：%v", "Couldn't delete the save: %v"), err)
			return
		}
	}
	a.games[i] = New(maps[i].key)
	a.games[i].Layout(a.w, a.h)
	if a.thumbs != nil && i < len(a.thumbs) {
		a.thumbs[i] = nil
	}
}

// Current is the open game, or nil on the start screen.
func (a *App) Current() *Game {
	if a.cur < 0 {
		return nil
	}
	return a.games[a.cur]
}

func (a *App) Layout(w, h int) (int, int) {
	a.w, a.h = w, h
	for _, g := range a.games {
		g.Layout(w, h)
	}
	return w, h
}

func (a *App) Update() error {
	if ebiten.IsWindowBeingClosed() {
		a.SaveAll()
		return ebiten.Termination
	}
	if g := a.Current(); g != nil {
		g.noSave = a.NoSave
		if err := g.Update(); err != nil {
			return err
		}
		g.updateSave(!a.NoSave)
		if g.toMenu {
			g.toMenu = false
			silence()
			if !a.NoSave {
				if err := g.exitSave(); err != nil {
					a.notice = fmt.Sprintf(locale.T("自动存档失败：%v", "Autosave failed: %v"), err)
				} else if g.hasProgress() {
					a.notice = fmt.Sprintf(locale.T("已自动存档：%s", "Autosaved: %s"), mapTitle(g.key))
				}
			}
			if a.cur < len(a.thumbs) {
				a.thumbs[a.cur] = nil // show the new track on the card
			}
			a.cur = -1
		}
		return nil
	}
	if a.slotMap >= 0 {
		i := a.slotMap
		switch a.games[i].updateSlots() {
		case slotsLoaded:
			a.slotMap, a.cur, a.sel = -1, i, i
			if i < len(a.thumbs) {
				a.thumbs[i] = nil
			}
		case slotsClosed:
			a.slotMap = -1
			if i < len(a.thumbs) {
				a.thumbs[i] = nil // a deleted or renamed save changes nothing, a save may
			}
		}
		return nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyF9):
		a.openSlots(a.sel)
		return nil
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft), inpututil.IsKeyJustPressed(ebiten.KeyA):
		a.sel = (a.sel + len(maps) - 1) % len(maps)
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowRight), inpututil.IsKeyJustPressed(ebiten.KeyD):
		a.sel = (a.sel + 1) % len(maps)
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeySpace):
		a.cur, a.notice = a.sel, ""
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		for i, r := range a.langB {
			if r.has(mx, my) {
				a.setLanguage([]locale.Language{locale.Chinese, locale.English}[i])
				return nil
			}
		}
		confirm := -1
		for i := range maps {
			if i < len(a.cards) && a.cards[i].has(mx, my) {
				a.sel = i // clicking anywhere on a card selects it
			}
			switch {
			case i < len(a.start) && a.start[i].has(mx, my):
				a.sel, a.cur, a.notice = i, i, ""
			case i < len(a.demo) && a.demo[i].has(mx, my) && !a.games[i].hasProgress():
				a.sel, a.cur, a.notice = i, i, ""
				if err := a.games[i].LoadDemo(); err != nil {
					a.games[i].say(locale.T("示例载入失败：%v", "The sample line couldn't be loaded: %v"), err)
				}
			case i < len(a.slotsB) && a.slotsB[i].has(mx, my):
				a.sel = i
				a.openSlots(i)
			case i < len(a.reset) && a.reset[i].has(mx, my):
				a.sel = i
				if a.confirm == i {
					a.restart(i)
				} else {
					confirm = i // ask once more before throwing progress away
				}
			}
		}
		a.confirm = confirm
	}
	return nil
}

const thumbW, thumbH = 460, 290

// thumbnail renders a map overview once for its card.
func (a *App) thumbnail(i int) *ebiten.Image {
	for len(a.thumbs) <= i {
		a.thumbs = append(a.thumbs, nil)
	}
	if a.thumbs[i] != nil {
		return a.thumbs[i]
	}
	g := a.games[i]
	img := ebiten.NewImage(thumbW, thumbH)
	saved := *g
	g.w, g.h = thumbW, thumbH
	g.camX, g.camY = g.mapW/2, g.mapH/2
	g.zoom = min(float64(thumbW)/g.mapW, float64(thumbH)/g.mapH) * 1.05
	g.page, g.cursor = pageBuild, street.Vec{X: -1e9, Y: -1e9}
	g.drawWorld(img)
	g.w, g.h, g.camX, g.camY, g.zoom, g.page, g.cursor = saved.w, saved.h, saved.camX, saved.camY, saved.zoom, saved.page, saved.cursor
	a.thumbs[i] = img
	return img
}

func (a *App) Draw(dst *ebiten.Image) {
	if g := a.Current(); g != nil {
		g.Draw(dst)
		return
	}
	dst.Fill(rgb(18, 24, 32))
	title := locale.T("2D 有轨电车", "2D Tram")
	tw, _ := textui.MeasureText(title, 34)
	text(dst, title, float64(a.w)/2-tw/2, 40, 34, color.White)
	sub := locale.T("选择地图开始：建设轨道 → 规划线路 → 运营", "Choose a map: build track → plan lines → run the service")
	sw, _ := textui.MeasureText(sub, 16)
	text(dst, sub, float64(a.w)/2-sw/2, 92, 16, rgb(150, 176, 190))
	a.drawLanguageSwitch(dst)

	cardW, cardH := float32(thumbW+40), float32(thumbH+170)
	gap := float32(40)
	x0 := (float32(a.w) - (cardW*float32(len(maps)) + gap*float32(len(maps)-1))) / 2
	y0 := float32(140)
	a.start, a.demo, a.reset, a.cards, a.slotsB = a.start[:0], a.demo[:0], a.reset[:0], a.cards[:0], a.slotsB[:0]
	mx, my := ebiten.CursorPosition()
	for i, m := range maps {
		x := x0 + float32(i)*(cardW+gap)
		card := rect{x, y0, x + cardW, y0 + cardH}
		a.cards = append(a.cards, card)
		border := rgb(60, 72, 88)
		if card.has(mx, my) {
			border = rgb(96, 116, 138)
		}
		if i == a.sel {
			border = colAccent
		}
		vector.FillRect(dst, x, y0, cardW, cardH, rgb(30, 38, 50), false)
		vector.StrokeRect(dst, x, y0, cardW, cardH, 2, border, false)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x+20), float64(y0+20))
		dst.DrawImage(a.thumbnail(i), op)
		text(dst, m.title(), float64(x+20), float64(y0+thumbH+34), 19, color.White)
		text(dst, m.desc(), float64(x+20), float64(y0+thumbH+64), 13, rgb(150, 176, 190))
		g := a.games[i]
		progress := g.hasProgress()
		if progress {
			info := fmt.Sprintf(locale.T("已建 %d 座车站、%d 条线路", "Built: stops %d · lines %d"), len(g.net.Stops), len(g.net.Lines))
			if t, ok := SavedAt(m.key); ok {
				info += locale.T("  ·  存档 ", "  ·  saved ") + t.Format("01-02 15:04")
			}
			text(dst, info, float64(x+20), float64(y0+thumbH+86), 12, colHover)
		}
		by := y0 + cardH - 50
		bw := (cardW - 80) / 3
		b1 := rect{x + 20, by, x + 20 + bw, by + 34}
		b3 := rect{b1.x1 + 20, by, b1.x1 + 20 + bw, by + 34}
		b2 := rect{x + cardW - 20 - bw, by, x + cardW - 20, by + 34}
		a.start, a.slotsB = append(a.start, b1), append(a.slotsB, b3)
		label2 := locale.T("载入示例线路", "Load sample line")
		if progress {
			a.demo, a.reset = append(a.demo, rect{}), append(a.reset, b2)
			label2 = locale.T("重新开始", "Start over")
			if a.confirm == i {
				label2 = locale.T("确定清空进度？", "Clear progress?")
			}
		} else {
			a.demo, a.reset = append(a.demo, b2), append(a.reset, rect{})
		}
		label1 := locale.T("开始建设", "Start building")
		if progress {
			label1 = locale.T("继续", "Continue")
		}
		for k, b := range []rect{b1, b2, b3} {
			bg := colAccent
			label := label1
			if k == 2 {
				bg, label = rgb(52, 64, 80), locale.T("读取存档", "Load save")
			}
			if k == 1 {
				bg, label = rgb(52, 64, 80), label2
				if progress && a.confirm == i {
					bg = rgb(150, 52, 48)
				}
			}
			if b.has(mx, my) {
				bg = tint(bg, rgb(255, 255, 255), 0.18)
			}
			vector.FillRect(dst, b.x0, b.y0, b.x1-b.x0, b.y1-b.y0, bg, false)
			lw, _ := textui.MeasureText(label, 15)
			col := color.Color(color.White)
			if k == 0 {
				col = rgb(14, 22, 30)
			}
			text(dst, label, float64(b.x0+b.x1)/2-lw/2, float64(b.y0)+8, 15, col)
		}
	}
	if a.notice != "" {
		nw, _ := textui.MeasureText(a.notice, 13)
		text(dst, a.notice, float64(a.w)/2-nw/2, float64(a.h)-66, 13, colHover)
	}
	hint := locale.T("← → 选择   回车开始   F9 读取存档   进度自动保存，游戏中 F5 存档到槽位   Esc 返回本页", "← → choose   Enter start   F9 load a save   Progress saves itself; F5 in a map saves to a slot   Esc comes back here")
	hw, _ := textui.MeasureText(hint, 13)
	text(dst, hint, float64(a.w)/2-hw/2, float64(a.h)-40, 13, rgb(120, 140, 152))
	if a.slotMap >= 0 {
		a.games[a.slotMap].drawSlots(dst, a.w, a.h)
	}
}

// drawLanguageSwitch draws "中文 | English" at the top right of the menu.
func (a *App) drawLanguageSwitch(dst *ebiten.Image) {
	a.langB = a.langB[:0]
	x := float64(a.w) - 20
	langs := []locale.Language{locale.Chinese, locale.English}
	rs := make([]rect, len(langs))
	for i := len(langs) - 1; i >= 0; i-- {
		w, _ := textui.MeasureText(langs[i].Name(), 14)
		x -= w + 24
		rs[i] = rect{float32(x), 20, float32(x + w + 24), 50}
		x -= 4
	}
	label := locale.T("语言", "Language")
	lw, _ := textui.MeasureText(label, 13)
	text(dst, label, float64(rs[0].x0)-lw-10, 27, 13, rgb(150, 176, 190))
	mx, my := ebiten.CursorPosition()
	for i, l := range langs {
		r := rs[i]
		a.langB = append(a.langB, r)
		bg, fg := rgb(36, 46, 60), color.Color(color.White)
		switch {
		case l == locale.Current:
			bg, fg = colAccent, rgb(14, 22, 30)
		case r.has(mx, my):
			bg = rgb(52, 66, 84)
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		text(dst, l.Name(), float64(r.x0)+12, float64(r.y0)+7, 14, fg)
	}
}
