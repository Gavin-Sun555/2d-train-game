package textui

import (
	"bytes"
	"image/color"
	"os"
	"sync"

	ebiten "github.com/hajimehoshi/ebiten/v2"
	ebitenutil "github.com/hajimehoshi/ebiten/v2/ebitenutil"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
)

var (
	chineseFontSource *text.GoTextFaceSource
	fontLoaded        bool
	faceCache         = make(map[float64]*text.GoTextFace, 32)
	faceCacheMu       sync.RWMutex
	drawOptionsPool   = sync.Pool{
		New: func() any {
			return &text.DrawOptions{}
		},
	}
	measureCache   = make(map[measureKey]measureVal, 512)
	measureCacheMu sync.RWMutex
)

type measureKey struct {
	text string
	size float64
}

type measureVal struct {
	w, h float64
}

func getFace(size float64) *text.GoTextFace {
	faceCacheMu.RLock()
	f, ok := faceCache[size]
	faceCacheMu.RUnlock()
	if ok {
		return f
	}
	faceCacheMu.Lock()
	defer faceCacheMu.Unlock()
	if f, ok = faceCache[size]; ok {
		return f
	}
	f = &text.GoTextFace{
		Source: chineseFontSource,
		Size:   size,
	}
	faceCache[size] = f
	return f
}

func init() {
	loadChineseFont()
}

func loadChineseFont() {
	fontPaths := []string{
		// macOS
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Supplemental/Songti.ttc",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/Library/Fonts/Arial Unicode.ttf",
		// Windows
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\msyh.ttf`,
		`C:\Windows\Fonts\simsun.ttc`,
		// Linux
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	}

	for _, p := range fontPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// Try collection first
		sources, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
		if err == nil && len(sources) > 0 {
			chineseFontSource = sources[0]
			fontLoaded = true
			return
		}
		// Try single font
		source, err := text.NewGoTextFaceSource(bytes.NewReader(data))
		if err == nil {
			chineseFontSource = source
			fontLoaded = true
			return
		}
	}
}

// DrawText draws text using the loaded Chinese font, or falls back to DebugPrintAt
func DrawText(dst *ebiten.Image, str string, x, y float64, size float64, clr color.Color) {
	if fontLoaded && chineseFontSource != nil {
		face := getFace(size)
		op := drawOptionsPool.Get().(*text.DrawOptions)
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.GeoM.Translate(x, y)
		if clr != nil {
			op.ColorScale.ScaleWithColor(clr)
		}
		text.Draw(dst, str, face, op)
		drawOptionsPool.Put(op)
		return
	}

	// Fallback to debug print
	ebitenutil.DebugPrintAt(dst, str, int(x), int(y))
}

// MeasureText measures the width and height of a string
func MeasureText(str string, size float64) (float64, float64) {
	key := measureKey{text: str, size: size}
	measureCacheMu.RLock()
	val, ok := measureCache[key]
	measureCacheMu.RUnlock()
	if ok {
		return val.w, val.h
	}

	if fontLoaded && chineseFontSource != nil {
		face := getFace(size)
		w, h := text.Measure(str, face, 0)
		measureCacheMu.Lock()
		if len(measureCache) < 2048 {
			measureCache[key] = measureVal{w: w, h: h}
		}
		measureCacheMu.Unlock()
		return w, h
	}
	return float64(len(str) * 7), 13.0
}

// DrawTextRotated draws text centred at (x, y), rotated by angle radians.
func DrawTextRotated(dst *ebiten.Image, str string, x, y, size, angle float64, clr color.Color) {
	if !fontLoaded || chineseFontSource == nil {
		ebitenutil.DebugPrintAt(dst, str, int(x), int(y))
		return
	}
	face := getFace(size)
	w, h := text.Measure(str, face, 0)
	op := drawOptionsPool.Get().(*text.DrawOptions)
	op.GeoM.Reset()
	op.ColorScale.Reset()
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Rotate(angle)
	op.GeoM.Translate(x, y)
	if clr != nil {
		op.ColorScale.ScaleWithColor(clr)
	}
	text.Draw(dst, str, face, op)
	drawOptionsPool.Put(op)
}
