package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Gavin-Sun555/2d-train-game/game"
)

func main() {
	demo := flag.Bool("demo", false, "start with a sample tram network")
	mapName := flag.String("map", "", "open a map directly: grid or seattle (default: start screen)")
	shot := flag.String("shot", "", "write a PNG screenshot to this path after -frames updates, then exit")
	frames := flag.Int("frames", 600, "updates to run before -shot")
	warp := flag.Float64("warp", 0, "fast-forward this many simulated seconds before starting")
	cam := flag.String("cam", "", "initial camera as x,y,zoom (metres, pixels per metre)")
	follow := flag.Bool("follow", false, "start on the Run page following the first tram")
	pageName := flag.String("page", "", "open a page: build, plan or run")
	fleet := flag.String("fleet", "", "set every line's vehicles as trams,sections (e.g. 3,4)")
	model := flag.String("model", "", "set every line's tram model key (e.g. siemens-s700, hu300-lightline)")
	platforms := flag.Int("platforms", 0, "lengthen every stop's platforms to hold this many sections")
	line := flag.Int("line", 0, "select this line (1-based) on the Plan page")
	slots := flag.String("slots", "", "open the save window: save or load")
	linesWin := flag.Bool("lines", false, "open the line management window")
	level := flag.Int("level", 0, "show a level: -2 (deep underground) to 2 (high elevated)")
	menu := flag.String("menu", "", "open a top-bar drop-down: cars, passengers or sound")
	toolN := flag.Int("tool", 0, "select a Build tool (1-based)")
	renameLm := flag.String("rename", "", "open the rename window of the stop with this name, or the landmark with this OpenStreetMap name")
	lang := flag.String("lang", "", "display language: zh or en (default: the saved choice, else the system's)")
	flag.Parse()
	game.InitLanguage(*lang)
	if *shot == "" { // screenshots stay silent
		if err := game.EnableAudio(); err != nil {
			log.Printf("no sound: %v", err)
		}
	}

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("2D Train Game")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true) // save before quitting
	// Demos and screenshots never touch the player's saves.
	app := game.NewApp(*demo || *shot != "")
	if *mapName == "" && *demo {
		*mapName = "grid"
	}
	if *mapName != "" {
		if err := app.Open(*mapName, *demo); err != nil {
			log.Fatal(err)
		}
		gm := app.Current()
		if *platforms > 0 {
			gm.SetPlatforms(*platforms)
		}
		if *fleet != "" {
			var t, c int
			if _, err := fmt.Sscanf(*fleet, "%d,%d", &t, &c); err != nil {
				log.Fatalf("-fleet: %v", err)
			}
			gm.SetFleet(t, c)
		}
		if *model != "" {
			gm.SetModel(*model)
		}
		if *line > 0 {
			gm.SelectLine(*line - 1)
		}
		if *warp > 0 {
			gm.Run()
			gm.Warp(*warp)
		}
		if *cam != "" {
			var x, y, z float64
			if _, err := fmt.Sscanf(*cam, "%f,%f,%f", &x, &y, &z); err != nil {
				log.Fatalf("-cam: %v", err)
			}
			gm.SetCamera(x, y, z)
		}
		if *pageName != "" {
			gm.ShowPage(*pageName)
		}
		if *follow {
			gm.Run()
			gm.FollowFirst()
		}
		if *slots != "" {
			gm.OpenSlots(*slots == "save")
		}
		if *linesWin {
			gm.OpenLinePanel()
		}
		if *level != 0 {
			gm.ShowLevel(*level)
		}
		if *menu != "" {
			gm.OpenMenu(*menu)
		}
		if *toolN > 0 {
			gm.SelectTool(*toolN - 1)
		}
		if *renameLm != "" {
			gm.OpenRename(*renameLm)
		}
	}
	var g ebiten.Game = app
	if *shot != "" {
		g = &shotGame{Game: g, path: *shot, warmupFrames: *frames}
	}
	if err := ebiten.RunGame(g); err != nil && !errors.Is(err, errShotDone) {
		log.Fatal(err)
	}
}
