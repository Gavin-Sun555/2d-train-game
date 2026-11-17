package game

import (
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
)

// slotPanel is the save / load window over a map: manual slots, and in
// load mode a second tab with the autosaves.
type slotPanel struct {
	save    bool // saving (else loading)
	auto    bool // autosave tab
	sel     int
	list    []slotInfo
	confirm string // "overwrite" or "delete" after one click, until the selection changes
	naming  bool
	name    []rune
	msg     string
	tick    int
	clickAt int // tick and row of the last row click, for double clicks
	clickOn int

	panel, tabMan, tabAuto, act, rename, del, close rect
	rows                                            []rect
}

// slot panel results
const (
	slotsOpen = iota
	slotsClosed
	slotsLoaded
)

// openSlots shows the save (or load) window.
func (g *Game) openSlots(save bool) {
	p := &slotPanel{save: save, clickOn: -1}
	p.list = g.Slots(false)
	if !save {
		// Start on the newest save, or on the autosaves if there is no
		// manual one.
		p.sel = newest(p.list)
		if p.sel < 0 {
			p.auto = true
			p.list = g.Slots(true)
			p.sel = max(newest(p.list), 0)
		}
	} else {
		for i, s := range p.list {
			if !s.Exists {
				p.sel = i
				break
			}
		}
	}
	g.slots = p
}

func newest(list []slotInfo) int {
	best := -1
	for i, s := range list {
		if s.Exists && !s.Err && (best < 0 || s.SavedAt.After(list[best].SavedAt)) {
			best = i
		}
	}
	return best
}

func (p *slotPanel) refresh(g *Game) { p.list = g.Slots(p.auto) }

func (p *slotPanel) choose(i int) {
	if i != p.sel {
		p.sel, p.confirm, p.naming = i, "", false
	}
}

func (p *slotPanel) label() string {
	if p.auto {
		return fmt.Sprintf(locale.T("自动存档 %d", "Autosave %d"), p.sel+1)
	}
	return fmt.Sprintf(locale.T("存档 %02d", "Slot %02d"), p.sel+1)
}

// updateSlots handles the window; it reports whether it was closed or a
// save was loaded.
func (g *Game) updateSlots() int {
	p := g.slots
	p.tick++
	cur := p.list[p.sel]
	if p.naming {
		return g.updateNaming()
	}
	mx, my := ebiten.CursorPosition()
	click := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	hit := func(r rect) bool { return click && r.has(mx, my) }
	act, del, ren := false, false, false
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		if p.confirm != "" {
			p.confirm, p.msg = "", ""
			return slotsOpen
		}
		g.slots = nil
		return slotsClosed
	case hit(p.close):
		g.slots = nil
		return slotsClosed
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowUp), inpututil.IsKeyJustPressed(ebiten.KeyW):
		p.choose((p.sel + len(p.list) - 1) % len(p.list))
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowDown), inpututil.IsKeyJustPressed(ebiten.KeyS):
		p.choose((p.sel + 1) % len(p.list))
	case !p.save && (inpututil.IsKeyJustPressed(ebiten.KeyTab) || hit(p.tabMan) && p.auto || hit(p.tabAuto) && !p.auto):
		p.auto, p.confirm, p.msg = !p.auto, "", ""
		p.refresh(g)
		p.sel = max(newest(p.list), 0)
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), hit(p.act):
		act = true
	case inpututil.IsKeyJustPressed(ebiten.KeyR), hit(p.rename):
		ren = true
	case inpututil.IsKeyJustPressed(ebiten.KeyDelete), inpututil.IsKeyJustPressed(ebiten.KeyBackspace), hit(p.del):
		del = true
	case click:
		for i, r := range p.rows {
			if r.has(mx, my) {
				if i == p.sel && i == p.clickOn && p.tick-p.clickAt < 25 {
					act = true // double click
				}
				p.choose(i)
				p.clickAt, p.clickOn = p.tick, i
			}
		}
	}
	switch {
	case act && p.save && g.noSave:
		p.msg = locale.T("示例 / 截图模式不保存进度", "Sample / screenshot mode doesn't save")
	case act && p.save:
		if cur.Exists && p.confirm != "overwrite" {
			p.confirm, p.msg = "overwrite", fmt.Sprintf(locale.T("再按一次将覆盖%s", "Press again to overwrite %s"), p.label())
			return slotsOpen
		}
		g.saveToSlot("")
	case act:
		if !cur.Exists {
			p.msg = locale.T("这个位置还没有存档", "This slot is empty")
			return slotsOpen
		}
		if err := g.LoadSlot(p.auto, p.sel, !g.noSave); err != nil {
			p.msg = fmt.Sprintf(locale.T("读取失败：%v", "Loading failed: %v"), err)
			return slotsOpen
		}
		g.say(locale.T("已读取%s", "Loaded %s"), p.label())
		g.slots = nil
		return slotsLoaded
	case ren:
		switch {
		case p.auto:
			p.msg = locale.T("自动存档不能命名", "Autosaves can't be named")
		case !cur.Exists && !p.save:
			p.msg = locale.T("这个位置还没有存档", "This slot is empty")
		default:
			p.naming, p.confirm, p.msg = true, "", ""
			p.name = []rune(cur.Name)
		}
	case del:
		switch {
		case p.auto:
			p.msg = locale.T("自动存档会自动轮换，不能删除", "Autosaves rotate by themselves and can't be deleted")
		case !cur.Exists:
			p.msg = locale.T("这个位置还没有存档", "This slot is empty")
		case p.confirm != "delete":
			p.confirm, p.msg = "delete", fmt.Sprintf(locale.T("再按一次将删除%s", "Press again to delete %s"), p.label())
		default:
			if err := g.DeleteSlot(p.sel); err != nil {
				p.msg = fmt.Sprintf(locale.T("删除失败：%v", "Deleting failed: %v"), err)
			} else {
				p.msg = fmt.Sprintf(locale.T("已删除%s", "Deleted %s"), p.label())
			}
			p.confirm = ""
			p.refresh(g)
		}
	}
	return slotsOpen
}

// saveToSlot saves into the selected manual slot.
func (g *Game) saveToSlot(name string) {
	p := g.slots
	if err := g.SaveSlot(p.sel, name); err != nil {
		p.msg = fmt.Sprintf(locale.T("保存失败：%v", "Saving failed: %v"), err)
		return
	}
	p.confirm, p.msg = "", fmt.Sprintf(locale.T("已保存到%s", "Saved to %s"), p.label())
	g.say(locale.T("已保存到%s", "Saved to %s"), p.label())
	p.refresh(g)
}

// OpenSlots shows the save (or load) window, for screenshots.
func (g *Game) OpenSlots(save bool) { g.openSlots(save) }

// updateNaming edits the slot name: typed characters, Backspace, Enter to
// keep it (on an empty slot in save mode it saves there), Esc to cancel.
func (g *Game) updateNaming() int {
	p := g.slots
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 0x20 && r != 0x7f && len(p.name) < MaxSlotName {
			p.name = append(p.name, r)
		}
	}
	bs := inpututil.KeyPressDuration(ebiten.KeyBackspace)
	if len(p.name) > 0 && (bs == 1 || bs > 20 && bs%3 == 0) {
		p.name = p.name[:len(p.name)-1]
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		p.naming = false
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		p.naming = false
		name := string(p.name)
		if !p.list[p.sel].Exists {
			if g.noSave {
				p.msg = locale.T("示例 / 截图模式不保存进度", "Sample / screenshot mode doesn't save")
				return slotsOpen
			}
			g.saveToSlot(name)
			return slotsOpen
		}
		if err := g.RenameSlot(p.sel, name); err != nil {
			p.msg = fmt.Sprintf(locale.T("命名失败：%v", "Naming failed: %v"), err)
			return slotsOpen
		}
		p.msg = fmt.Sprintf(locale.T("已命名%s", "Named %s"), p.label())
		p.refresh(g)
	}
	return slotsOpen
}

// drawSlots draws the window centred on the screen.
func (g *Game) drawSlots(dst *ebiten.Image, w, h int) {
	p := g.slots
	if p == nil {
		return
	}
	vector.FillRect(dst, 0, 0, float32(w), float32(h), rgba(0, 0, 0, 140), false)
	const pw, rowH = 620, 34
	ph := float32(132 + rowH*MaxSlots + 96)
	x0, y0 := float32(w-pw)/2, (float32(h)-ph)/2
	p.panel = rect{x0, y0, x0 + pw, y0 + ph}
	vector.FillRect(dst, x0, y0, pw, ph, rgb(24, 31, 41), false)
	vector.StrokeRect(dst, x0, y0, pw, ph, 2, colAccent, false)

	title := locale.T("保存进度", "Save progress")
	if !p.save {
		title = locale.T("读取存档", "Load save")
	}
	text(dst, title+" · "+mapTitle(g.key), float64(x0+24), float64(y0+18), 20, color.White)

	// Tabs: manual slots, and the autosaves when loading.
	tabs := []string{fmt.Sprintf(locale.T("手动存档（%d）", "Saves (%d)"), MaxSlots)}
	if !p.save {
		tabs = append(tabs, fmt.Sprintf(locale.T("自动存档（%d）", "Autosaves (%d)"), MaxAutoSaves))
	}
	tx := x0 + 24
	for i, t := range tabs {
		tw, _ := textui.MeasureText(t, 14)
		r := rect{tx, y0 + 58, tx + float32(tw) + 24, y0 + 88}
		on := (i == 1) == p.auto
		if on {
			vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, colAccent, false)
			text(dst, t, float64(r.x0+12), float64(r.y0+7), 14, rgb(14, 22, 30))
		} else {
			vector.StrokeRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, 1, rgb(80, 92, 106), false)
			text(dst, t, float64(r.x0+12), float64(r.y0+7), 14, color.White)
		}
		if i == 0 {
			p.tabMan = r
		} else {
			p.tabAuto = r
		}
		tx = r.x1 + 10
	}
	if p.save {
		p.tabAuto = rect{}
	}

	// Rows.
	mx, my := ebiten.CursorPosition()
	p.rows = p.rows[:0]
	y := y0 + 104
	for i, s := range p.list {
		r := rect{x0 + 20, y, x0 + pw - 20, y + rowH - 4}
		p.rows = append(p.rows, r)
		bg := rgb(32, 41, 54)
		switch {
		case i == p.sel:
			bg = rgb(44, 70, 84)
		case r.has(mx, my):
			bg = rgb(38, 50, 64)
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		if i == p.sel {
			vector.FillRect(dst, r.x0, r.y0, 4, r.y1-r.y0, colAccent, false)
		}
		ty := float64(r.y0 + 7)
		text(dst, fmt.Sprintf("%02d", i+1), float64(r.x0+14), ty, 14, rgb(150, 176, 190))
		name := s.Name
		switch {
		case p.naming && i == p.sel:
			name = string(p.name)
			if p.tick/30%2 == 0 {
				name += "▏"
			}
		case !s.Exists:
			name = locale.T("— 空 —", "— empty —")
		case s.Err:
			name = locale.T("存档损坏", "Damaged save")
		case name == "" && p.auto:
			name = locale.T("自动存档", "Autosave")
		case name == "":
			name = locale.T("未命名", "Unnamed")
		}
		col := color.Color(color.White)
		if !s.Exists && !(p.naming && i == p.sel) {
			col = rgb(110, 124, 138)
		}
		text(dst, name, float64(r.x0+52), ty, 14, col)
		if s.Exists && !s.Err {
			info := fmt.Sprintf(locale.T("%d 站 · %d 线    %s", "stops %d · lines %d    %s"), s.Stops, s.Lines, s.SavedAt.Local().Format("01-02 15:04"))
			iw, _ := textui.MeasureText(info, 13)
			text(dst, info, float64(r.x1-14)-iw, ty+1, 13, rgb(150, 176, 190))
		}
		y += rowH
	}

	// Message and buttons.
	msg := p.msg
	switch {
	case p.naming:
		msg = fmt.Sprintf(locale.T("输入名称（最多 %d 字），回车确认，Esc 取消", "Type a name (up to %d characters), Enter to confirm, Esc to cancel"), MaxSlotName)
	case msg == "":
		msg = locale.T("↑↓ 选择   双击或回车", "↑↓ choose   Double-click or Enter to ") + map[bool]string{true: locale.T("保存", "save"), false: locale.T("读取", "load")}[p.save] + locale.T("   R 命名   Delete 删除   Esc 关闭", "   R name   Delete delete   Esc close")
		if !p.save {
			msg += locale.T("   Tab 切换", "   Tab switch")
		}
	}
	mcol := color.Color(rgb(150, 176, 190))
	if p.confirm != "" {
		mcol = colHover
	}
	text(dst, msg, float64(x0+24), float64(y+8), 13, mcol)

	by := y + 40
	act := locale.T("保存到此处", "Save here")
	if !p.save {
		act = locale.T("读取", "load")
	}
	if p.confirm == "overwrite" {
		act = locale.T("确认覆盖", "Overwrite")
	}
	del := locale.T("删除", "Delete")
	if p.confirm == "delete" {
		del = locale.T("确认删除", "Confirm delete")
	}
	p.act = rect{x0 + 24, by, x0 + 184, by + 34}
	p.rename = rect{x0 + 196, by, x0 + 296, by + 34}
	p.del = rect{x0 + 308, by, x0 + 408, by + 34}
	p.close = rect{x0 + pw - 124, by, x0 + pw - 24, by + 34}
	cur := p.list[p.sel]
	button := func(r rect, label string, bg color.RGBA, fg color.Color, enabled bool) {
		if !enabled {
			bg, fg = rgb(36, 44, 56), rgb(96, 108, 120)
		} else if r.has(mx, my) {
			bg = tint(bg, rgb(255, 255, 255), 0.18)
		}
		vector.FillRect(dst, r.x0, r.y0, r.x1-r.x0, r.y1-r.y0, bg, false)
		lw, _ := textui.MeasureText(label, 15)
		text(dst, label, float64(r.x0+r.x1)/2-lw/2, float64(r.y0)+8, 15, fg)
	}
	actBG := colAccent
	if p.confirm == "overwrite" {
		actBG = rgb(196, 120, 40)
	}
	button(p.act, act, actBG, rgb(14, 22, 30), p.save || cur.Exists && !cur.Err)
	button(p.rename, locale.T("命名", "Name"), rgb(52, 64, 80), color.White, !p.auto && (p.save || cur.Exists))
	delBG := rgb(52, 64, 80)
	if p.confirm == "delete" {
		delBG = rgb(150, 52, 48)
	}
	button(p.del, del, delBG, color.White, !p.auto && cur.Exists)
	button(p.close, locale.T("关闭", "Close"), rgb(52, 64, 80), color.White, true)
}

func mapTitle(key string) string {
	for _, m := range maps {
		if m.key == key {
			// Drop the card number ("01  网格新城").
			return strings.TrimLeft(m.title(), "0123456789 ")
		}
	}
	return key
}
