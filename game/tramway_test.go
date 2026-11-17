package game

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

func TestCorridorTerrainRules(t *testing.T) {
	g := New("grid")

	var waterPt, tallBldgPt, lowBldgPt street.Vec
	var foundWater, foundTall, foundLow bool
	for y := 2; y < g.city.H-2 && !foundWater; y++ {
		for x := 2; x < g.city.W-2; x++ {
			if g.city.At(x, y).Kind == city.Water && g.city.At(x-1, y).Kind == city.Water && g.city.At(x+1, y).Kind == city.Water {
				waterPt = street.Vec{X: (float64(x) + 0.5) * city.TileSize, Y: (float64(y) + 0.5) * city.TileSize}
				foundWater = true
				break
			}
		}
	}
	for _, lot := range g.city.Lots {
		mid := street.Vec{X: lot.X + lot.W/2, Y: lot.Y + lot.H/2}
		if lot.Style == city.LotTower && !foundTall {
			tallBldgPt, foundTall = mid, true
		}
		if lot.Style == city.LotHomes && !foundLow {
			lowBldgPt, foundLow = mid, true
		}
	}
	if !foundWater || !foundTall || !foundLow {
		t.Fatalf("missing test terrain: water=%v tall=%v low=%v", foundWater, foundTall, foundLow)
	}

	// Water: ground (0->0) and ramps (0->1) are refused; elevated (1->1) and underground (-1->-1) are allowed.
	wa := street.Vec{X: waterPt.X - 15, Y: waterPt.Y}
	wb := street.Vec{X: waterPt.X + 15, Y: waterPt.Y}
	if err := g.checkCorridor(wa, wb, 0, 0); err == nil {
		t.Error("expected ground track over water to be refused")
	}
	if err := g.checkCorridor(wa, wb, 0, 1); err == nil {
		t.Error("expected 0->1 ramp over water to be refused")
	}
	if err := g.checkCorridor(wa, wb, 1, 1); err != nil {
		t.Errorf("elevated viaduct over water refused: %v", err)
	}
	if err := g.checkCorridor(wa, wb, -1, -1); err != nil {
		t.Errorf("underground tunnel under water refused: %v", err)
	}

	// Tall tower (40 m): ground (0->0) and elevated (1->1) refused; underground (-1->-1) allowed.
	ta := street.Vec{X: tallBldgPt.X - 10, Y: tallBldgPt.Y}
	tb := street.Vec{X: tallBldgPt.X + 10, Y: tallBldgPt.Y}
	if err := g.checkCorridor(ta, tb, 0, 0); err == nil {
		t.Error("expected ground track through tower to be refused")
	}
	if err := g.checkCorridor(ta, tb, 1, 1); err == nil {
		t.Error("expected elevated viaduct through tall tower to be refused")
	}
	if err := g.checkCorridor(ta, tb, -1, -1); err != nil {
		t.Errorf("underground tunnel beneath tall tower refused: %v", err)
	}

	// Low-rise homes (8 m): ground (0->0) refused, elevated (1->1) clears 8 m roofs, underground (-1->-1) allowed.
	la := street.Vec{X: lowBldgPt.X - 10, Y: lowBldgPt.Y}
	lb := street.Vec{X: lowBldgPt.X + 10, Y: lowBldgPt.Y}
	if err := g.checkCorridor(la, lb, 0, 0); err == nil {
		t.Error("expected ground track through low-rise houses to be refused")
	}
	if err := g.checkCorridor(la, lb, 1, 1); err != nil {
		t.Errorf("expected elevated viaduct to clear 8 m low-rise houses: %v", err)
	}
}

func TestMultiLevelAndOffRoadBuildSaveAndRender(t *testing.T) {
	saveDir := os.Getenv("TRAIN_GAME_SHOT_SAVE")
	if saveDir == "" {
		saveDir = t.TempDir()
	}
	t.Setenv("TRAIN_GAME_SAVES", saveDir)

	g := New("grid")
	g.zoom = 2.0
	g.setPage(pageBuild)
	g.tool = toolTrack
	g.layout = 0 // Dedicated double-track

	clickAt := func(lv int, x, y float64) {
		t.Helper()
		g.ShowLevel(lv)
		g.cursor = street.Vec{X: x, Y: y}
		g.buildTool(true, false)
	}
	rightClick := func() {
		g.buildTool(false, true)
	}

	// --- Line 1 corridor: Street (0) -> Tunnel (-1) -> Off-Road Park Level Crossing (0) -> Street (0) ---
	clickAt(0, 415, 330) // street (330,330)-(500,330)
	rightClick()
	clickAt(0, 1770, 415) // street (1770,330)-(1770,500)
	rightClick()

	clickAt(0, 500, 330)   // start at street junction (500, 330)
	clickAt(-1, 575, 410)  // 0 -> -1 tunnel portal inside Park (522..638, 342..478)
	clickAt(-1, 1495, 410) // -1 -> -1 downtown subway tunnel beneath towers & arterials
	clickAt(0, 1555, 410)  // -1 -> 0 portal emerging in Park (1482..1598, 342..478)
	clickAt(0, 1715, 410)  // 0 -> 0 off-road park track crossing X=1610 street into Park (1622..1758)!
	clickAt(0, 1770, 330)  // 0 -> 0 connect into street junction (1770, 330)
	rightClick()

	// --- Line 2 corridor: Street (0) -> Elevated River Viaduct (+1) -> Off-Road Park (0) -> Street (0) ---
	elevX := 1330.0
	for _, cand := range []float64{1330, 1395, 550, 610, 210, 280} {
		if g.checkCorridor(street.Vec{X: cand, Y: 695}, street.Vec{X: cand, Y: 1030}, 1, 1) == nil {
			elevX = cand
			break
		}
	}
	clickAt(0, 1290, 575)  // street (1290,500)-(1290,650)
	rightClick()
	clickAt(0, 1375, 1130) // street (1290,1130)-(1460,1130)
	rightClick()

	clickAt(0, 1290, 650)  // start at street junction (1290, 650)
	clickAt(1, elevX, 695) // 0 -> +1 ramp inside northern riverbank Park
	clickAt(1, elevX, 1030) // +1 -> +1 viaduct spanning across the river & streets!
	clickAt(0, 1290, 1130) // +1 -> 0 ramp inside southern Park into junction (1290, 1130)
	rightClick()

	// Place stops on Ground streets (0), Underground (-1), Off-Road Park (0), and Elevated Viaduct (+1).
	g.tool = toolStop
	clickAt(0, 415, 330)    // L1 west street terminus
	clickAt(-1, 1030, 410)  // L1 underground station
	clickAt(0, 1665, 410)   // L1 off-road park stop
	clickAt(0, 1770, 415)   // L1 east street terminus

	clickAt(0, 1290, 575)   // L2 north street terminus
	clickAt(1, elevX, 820)  // L2 elevated river-view station (+1)
	clickAt(0, 1375, 1130)  // L2 south street terminus

	if len(g.net.Stops) != 7 {
		t.Fatalf("built %d stops, want 7 (elevX=%.0f, last msg: %s)", len(g.net.Stops), elevX, g.message)
	}
	levelsSeen := map[int]int{}
	offRoadStops := 0
	for _, st := range g.net.Stops {
		levelsSeen[g.net.StopLevel(st)]++
		if g.net.Roads[st.Road].Tramway && g.net.StopLevel(st) == 0 {
			offRoadStops++
		}
	}
	if levelsSeen[1] != 1 || levelsSeen[-1] != 1 || levelsSeen[0] != 5 || offRoadStops != 1 {
		t.Errorf("unexpected stop distribution: levels=%v offRoadGround=%d", levelsSeen, offRoadStops)
	}

	// Plan Line 1 (Chinese modern CRRC Suzhou LRV) & Line 2 (1935 Dalian vintage streetcar).
	g.setPage(pagePlan)
	pickStop := func(lv int, x, y float64) {
		t.Helper()
		st, ok := g.net.StopNearAt(street.Vec{X: x, Y: y}, lv, 30)
		if !ok {
			t.Fatalf("no stop near (%.0f, %.0f) lv %d", x, y, lv)
		}
		g.pending = append(g.pending, st.ID)
	}
	pickStop(0, 415, 330)
	pickStop(-1, 1030, 410)
	pickStop(0, 1665, 410)
	pickStop(0, 1770, 415)
	g.commitLine()

	pickStop(0, 1290, 575)
	pickStop(1, elevX, 820)
	pickStop(0, 1375, 1130)
	g.commitLine()

	if len(g.net.Lines) != 2 {
		t.Fatalf("expected 2 lines committed, got %d (%s)", len(g.net.Lines), g.message)
	}
	ids := g.net.LineIDs()
	g.net.Lines[ids[0]].Trams = 4
	g.net.Lines[ids[0]].Model = "crrc-suzhou"
	g.net.Lines[ids[1]].Trams = 3
	g.net.Lines[ids[1]].Model = "dalian-3000-1935"

	g.ShowLevel(0)
	g.Run()
	g.Warp(240)
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}

	// Reload into a fresh Game instance and verify all edits, stops, levels and lines survive.
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatalf("Load failed: ok=%v err=%v", ok, err)
	}
	if len(h.net.Stops) != 7 || len(h.net.Lines) != 2 {
		t.Fatalf("loaded %d stops, %d lines; want 7, 2", len(h.net.Stops), len(h.net.Lines))
	}

	dst := ebiten.NewImage(1280, 720)
	h.Layout(1280, 720)
	for _, lv := range []int{0, 1, -1} {
		h.ShowLevel(lv)
		h.Draw(dst)
	}
	h.ShowLevel(0)

	// Verify underground cutaway triggers cleanly when following an underground tram.
	enteredTunnel := false
	for step := 0; step < 3600*4 && !enteredTunnel; step++ {
		h.sim.Step()
		for _, v := range h.sim.Vehicles {
			if v.Kind == traffic.TramKind && h.vehicleElevation(v) < -0.6 {
				h.follow = v.ID
				h.zoom = 18
				h.updateFollow()
				h.Draw(dst)
				enteredTunnel = true
				break
			}
		}
	}
	if !enteredTunnel {
		t.Fatal("no tram entered the underground tunnel within 4 minutes")
	}

	// Exercise the model picker popup across all 3 filter tabs (All / Modern / Classic).
	h.OpenLinePanel()
	h.modelMenu = &modelPicker{line: ids[0]}
	for f := 0; f < 3; f++ {
		h.modelMenu.filter = f
		h.Draw(dst)
	}
}

func TestSeattleCorridorAndTramwayRemoval(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("seattle")

	// 1. Water check on Seattle (Lake Union / Elliott Bay): ground refused, viaduct & tunnel allowed.
	var waterPt street.Vec
	foundWater := false
	for i := range g.scene.areas {
		a := &g.scene.areas[i]
		if a.kind == "water" && a.m.box[2]-a.m.box[0] > 300 && a.m.box[3]-a.m.box[1] > 300 {
			cand := street.Vec{X: (a.m.box[0] + a.m.box[2]) / 2, Y: (a.m.box[1] + a.m.box[3]) / 2}
			if g.waterAt(cand) && !g.highwayConflict(cand, 1, 1) {
				waterPt, foundWater = cand, true
				break
			}
		}
	}
	if !foundWater {
		t.Fatal("could not find water interior point on Seattle map")
	}
	wa, wb := street.Vec{X: waterPt.X - 12, Y: waterPt.Y}, street.Vec{X: waterPt.X + 12, Y: waterPt.Y}
	if err := g.checkCorridor(wa, wb, 0, 0); err == nil {
		t.Error("Seattle: expected ground track over water to be refused")
	}
	if err := g.checkCorridor(wa, wb, -1, -1); err != nil {
		t.Errorf("Seattle: expected underground tunnel under water to succeed: %v", err)
	}

	// 2. Same-level motorway conflict on Seattle is refused, while underground (-1) under a ground/elevated motorway is allowed.
	var hwPt street.Vec
	var hwLv int
	foundHw := false
	for i := range g.scene.highways {
		hw := &g.scene.highways[i]
		if hw.level >= 0 && len(hw.pts) >= 2 {
			mid := hw.pts[0].Lerp(hw.pts[1], 0.5)
			if mid.X > 100 && mid.Y > 100 && mid.X < g.mapW-100 && mid.Y < g.mapH-100 && !g.highwayConflict(mid, -1, -1) {
				hwPt, hwLv, foundHw = mid, hw.level, true
				break
			}
		}
	}
	if !foundHw {
		t.Fatal("could not find surface/elevated motorway on Seattle map")
	}
	ha, hb := street.Vec{X: hwPt.X - 8, Y: hwPt.Y}, street.Vec{X: hwPt.X + 8, Y: hwPt.Y}
	if err := g.checkCorridor(ha, hb, hwLv, hwLv); err == nil {
		t.Errorf("Seattle: expected level-%d track across level-%d motorway to be refused", hwLv, hwLv)
	}
	if err := g.checkCorridor(ha, hb, -1, -1); err != nil {
		t.Errorf("Seattle: expected -1 tunnel beneath level-%d motorway to succeed: %v", hwLv, err)
	}

	// 3. Build an underground tunnel in Seattle, place a stop on it, then remove it via toolRemove and replay via Save/Load.
	g.zoom = 2.0
	g.setPage(pageBuild)
	g.tool = toolTrack
	g.ShowLevel(-1)
	p1 := street.Vec{X: waterPt.X - 60, Y: waterPt.Y}
	p2 := street.Vec{X: waterPt.X + 60, Y: waterPt.Y}
	g.cursor = p1
	g.buildTool(true, false)
	g.cursor = p2
	g.buildTool(true, false)
	g.buildTool(false, true)

	g.tool = toolStop
	g.cursor = waterPt
	g.buildTool(true, false)
	if len(g.net.Stops) != 1 {
		t.Fatalf("Seattle: expected 1 underground stop, got %d (%s)", len(g.net.Stops), g.message)
	}

	// Demolish the standalone underground segment via toolRemove and verify stop is cleaned up & save replays cleanly.
	g.tool = toolRemove
	g.cursor = waterPt
	g.buildTool(true, false)
	if len(g.net.Stops) != 0 {
		t.Fatalf("Seattle: expected stop on removed tunnel to be deleted, got %d", len(g.net.Stops))
	}
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}
	h := New("seattle")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatalf("Seattle Load after tramway + rmtramway failed: ok=%v err=%v", ok, err)
	}
	if len(h.net.Edits()) != 2 {
		t.Fatalf("Seattle expected 2 edits (tramway + rmtramway), got %d", len(h.net.Edits()))
	}
}
