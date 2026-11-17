package game

import (
	"errors"
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"github.com/Gavin-Sun555/2d-train-game/internal/textui"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

// Stops can be renamed on the Plan page: click a stop's name label, or
// point at a stop and press N. The names are part of the network, so they
// are saved, shown on the lines and read out in the announcements.

const MaxStopName = 24 // runes

type stopLabel struct {
	id street.StopID
	r  rect
}

// RenameStop gives a stop a new name.
func (g *Game) RenameStop(id street.StopID, name string) error {
	st := g.net.Stops[id]
	if st == nil {
		return errors.New(locale.T("车站不存在", "No such stop"))
	}
	name, err := cleanSlotName(name)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New(locale.T("站名不能为空", "A stop needs a name"))
	}
	st.Name = name
	return nil
}

// openStopRename opens the rename window for a stop.
func (g *Game) openStopRename(id street.StopID) {
	if st := g.net.Stops[id]; st != nil {
		g.rename = &renameBox{i: -1, stop: id, name: []rune(st.Name)}
	}
}

// stopLabelClicked opens the rename window if a click on the Plan page hit
// a stop's name label.
func (g *Game) stopLabelClicked(mx, my int) bool {
	if g.page != pagePlan {
		return false
	}
	for k := len(g.stopLabels) - 1; k >= 0; k-- {
		if l := g.stopLabels[k]; l.r.has(mx, my) {
			g.openStopRename(l.id)
			return true
		}
	}
	return false
}

func (g *Game) drawStopRename(dst *ebiten.Image, b *renameBox, x0, y0, pw, ph float32) {
	st := g.net.Stops[b.stop]
	if st == nil {
		g.rename = nil
		return
	}
	text(dst, locale.T("车站改名", "Rename stop"), float64(x0+20), float64(y0+16), 18, color.White)
	text(dst, clip(locale.T("原名：", "Now: ")+st.Name, 12, pw-40), float64(x0+20), float64(y0+44), 12, colDim)
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
		text(dst, fmt.Sprintf(locale.T("回车确定，Esc 取消；最多 %d 个字", "Enter to confirm, Esc to cancel; up to %d characters"), MaxStopName), float64(x0+20), float64(y0+104), 12, colDim)
	}
	var btns []hudButton
	bx := float64(x0 + pw - 20)
	b.reset = rect{}
	for _, it := range []struct {
		label string
		r     *rect
		on    bool
	}{{locale.T("取消", "Cancel"), &b.esc, false}, {locale.T("确定", "OK"), &b.ok, true}} {
		w, _ := textui.MeasureText(it.label, 13)
		bx -= w + 14
		style := btnPlain
		if it.on {
			style = btnOn
		}
		buttonAt(dst, &btns, bx, float64(y0+ph-44), 28, it.label, style, nil)
		*it.r = btns[len(btns)-1].r
		bx -= 8
	}
}
