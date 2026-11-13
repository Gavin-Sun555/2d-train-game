package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// mapInfo describes a map on the start screen.
type mapInfo struct {
	key, title, desc string
}

var maps = []mapInfo{
	{"grid", "01  网格新城", "规整的生成城市：支路、主干道、大道与河流，适合练手"},
	{"seattle", "02  西雅图 · 市中心与南湖联合区", "真实路网与建筑（OpenStreetMap），含 SLU 有轨电车示例"},
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
}

func NewApp() *App {
	a := &App{cur: -1, w: 1280, h: 720}
	for _, m := range maps {
		a.games = append(a.games, New(m.key))
	}
	return a
}

// Open enters a map directly (optionally with its sample network).
func (a *App) Open(key string, demo bool) error {
	for i, m := range maps {
		if m.key == key {
			a.cur = i
			if demo {
				return a.games[i].LoadDemo()
			}
			return nil
		}
	}
	return fmt.Errorf("unknown map %q", key)
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
	if g := a.Current(); g != nil {
		if err := g.Update(); err != nil {
			return err
		}
		if g.toMenu {
			g.toMenu = false
			a.cur = -1
		}
		return nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft), inpututil.IsKeyJustPressed(ebiten.KeyA):
		a.sel = (a.sel + len(maps) - 1) % len(maps)
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowRight), inpututil.IsKeyJustPressed(ebiten.KeyD):
		a.sel = (a.sel + 1) % len(maps)
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter), inpututil.IsKeyJustPressed(ebiten.KeySpace):
		a.cur = a.sel
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		for i := range maps {
			switch {
			case i < len(a.start) && a.start[i].has(mx, my):
				a.sel, a.cur = i, i
			case i < len(a.demo) && a.demo[i].has(mx, my):
				a.sel, a.cur = i, i
				g := a.games[i]
				if len(g.net.Lines) == 0 {
					if err := g.LoadDemo(); err != nil {
						g.say("示例载入失败：%v", err)
					}
				}
			}
		}
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
	title := "2D 有轨电车"
	tw, _ := textui.MeasureText(title, 34)
	text(dst, title, float64(a.w)/2-tw/2, 40, 34, color.White)
	sub := "选择地图开始：建设轨道 → 规划线路 → 运营"
	sw, _ := textui.MeasureText(sub, 16)
	text(dst, sub, float64(a.w)/2-sw/2, 92, 16, rgb(150, 176, 190))

	cardW, cardH := float32(thumbW+40), float32(thumbH+170)
	gap := float32(40)
	x0 := (float32(a.w) - (cardW*float32(len(maps)) + gap*float32(len(maps)-1))) / 2
	y0 := float32(140)
	a.start, a.demo = a.start[:0], a.demo[:0]
	mx, my := ebiten.CursorPosition()
	for i, m := range maps {
		x := x0 + float32(i)*(cardW+gap)
		border := rgb(60, 72, 88)
		if i == a.sel {
			border = colAccent
		}
		vector.FillRect(dst, x, y0, cardW, cardH, rgb(30, 38, 50), false)
		vector.StrokeRect(dst, x, y0, cardW, cardH, 2, border, false)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x+20), float64(y0+20))
		dst.DrawImage(a.thumbnail(i), op)
		text(dst, m.title, float64(x+20), float64(y0+thumbH+34), 19, color.White)
		text(dst, m.desc, float64(x+20), float64(y0+thumbH+64), 13, rgb(150, 176, 190))
		g := a.games[i]
		if len(g.net.Lines) > 0 || g.sim.Seconds() > 0 {
			text(dst, "（有进度，点击继续）", float64(x+20), float64(y0+thumbH+84), 12, colHover)
		}
		by := y0 + cardH - 50
		b1 := rect{x + 20, by, x + 20 + 200, by + 34}
		b2 := rect{x + cardW - 220, by, x + cardW - 20, by + 34}
		a.start, a.demo = append(a.start, b1), append(a.demo, b2)
		for k, b := range []rect{b1, b2} {
			bg := colAccent
			label := "开始建设"
			if k == 1 {
				bg, label = rgb(52, 64, 80), "载入示例线路"
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
	hint := "← → 选择   回车开始   游戏中按 Esc 返回本页"
	hw, _ := textui.MeasureText(hint, 13)
	text(dst, hint, float64(a.w)/2-hw/2, float64(a.h)-40, 13, rgb(120, 140, 152))
}
