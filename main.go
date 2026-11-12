package main

import (
	"errors"
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Gavin-Sun555/2d-train-game/game"
)

func main() {
	scenario := flag.Int("scenario", 1, "built-in scenario to start (1-based)")
	shot := flag.String("shot", "", "write a PNG screenshot to this path after -frames updates, then exit")
	frames := flag.Int("frames", 600, "updates to run before -shot")
	warp := flag.Float64("warp", 0, "fast-forward this many simulated seconds before starting")
	flag.Parse()

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("2D Train Game")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	gm := game.New(*scenario - 1)
	gm.Warp(*warp)
	var g ebiten.Game = gm
	if *shot != "" {
		g = &shotGame{Game: g, path: *shot, warmupFrames: *frames}
	}
	if err := ebiten.RunGame(g); err != nil && !errors.Is(err, errShotDone) {
		log.Fatal(err)
	}
}
