package main

import (
	"errors"
	"image"
	"image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

var errShotDone = errors.New("screenshot written")

// shotGame wraps a game, fast-forwards it, and saves one frame as PNG.
type shotGame struct {
	ebiten.Game
	path         string
	warmupFrames int
	frame        int
	done         bool
}

func (s *shotGame) Update() error {
	if s.done {
		return errShotDone
	}
	s.frame++
	return s.Game.Update()
}

func (s *shotGame) Draw(dst *ebiten.Image) {
	s.Game.Draw(dst)
	if s.frame < s.warmupFrames || s.done {
		return
	}
	b := dst.Bounds()
	img := image.NewRGBA(b)
	dst.ReadPixels(img.Pix)
	f, err := os.Create(s.path)
	if err == nil {
		_ = png.Encode(f, img)
		_ = f.Close()
	}
	s.done = true
}
