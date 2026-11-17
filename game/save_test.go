package game

import (
	"encoding/json"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

func TestSaveAndLoadEachMap(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	for _, m := range []string{"grid", "seattle"} {
		t.Run(m, func(t *testing.T) {
			g := New(m)
			if ok, err := g.Load(); ok || err != nil {
				t.Fatalf("fresh map: loaded %v, %v", ok, err)
			}
			if err := g.Save(true); err != nil {
				t.Fatal(err)
			}
			if _, ok := SavedAt(m); ok {
				t.Fatal("an untouched map should not be saved")
			}
			if err := g.LoadDemo(); err != nil {
				t.Fatal(err)
			}
			ids := g.net.LineIDs()
			l := g.net.Lines[ids[0]]
			l.Trams, l.Sections = 2, 2
			st := g.net.Stops[l.Stops[0]]
			if err := g.net.SetStopSections(st.ID, 2); err != nil {
				t.Fatal(err)
			}
			g.SetCamera(123, 456, 2)
			if err := g.Save(false); err != nil {
				t.Fatal(err)
			}

			h := New(m)
			ok, err := h.Load()
			if !ok || err != nil {
				t.Fatalf("load: %v, %v", ok, err)
			}
			a, _ := json.Marshal(g.net.Export())
			b, _ := json.Marshal(h.net.Export())
			if string(a) != string(b) {
				t.Fatal("loaded network differs from the saved one")
			}
			if len(h.net.Links) != len(g.net.Links) || h.camX != 123 || h.zoom != 2 {
				t.Fatalf("links %d vs %d, camera %.0f@%.1f", len(h.net.Links), len(g.net.Links), h.camX, h.zoom)
			}
			if h.net.Stops[st.ID].Capacity() != 2 || h.net.TramSections(h.net.Lines[ids[0]]) != 2 {
				t.Error("platform length or train length lost")
			}
			h.Warp(120)
			trams := 0
			for _, v := range h.sim.Vehicles {
				if v.Line == ids[0] && v.Cars == 2 {
					trams++
				}
			}
			if trams == 0 {
				t.Error("loaded line does not run")
			}
			if err := h.Save(false); err != nil {
				t.Fatal(err)
			}
			if err := DeleteSave(m); err != nil {
				t.Fatal(err)
			}
			if ok, _ := New(m).Load(); ok {
				t.Error("deleted save still loads")
			}
		})
	}
}

func TestLongPlatformsAllowLongTrams(t *testing.T) {
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	for _, id := range g.net.StopIDs() {
		for k := street.MaxSections; k > street.DefaultPlatform; k-- {
			if g.net.SetStopSections(id, k) == nil {
				break
			}
		}
	}
	longest := 0
	for _, id := range g.net.LineIDs() {
		l := g.net.Lines[id]
		l.Sections, l.Trams = street.MaxSections, 2
		c, why := g.net.LineCapacity(l)
		t.Logf("line %d: up to %d sections (%s)", id, c, why)
		longest = max(longest, g.net.TramSections(l))
	}
	if longest <= street.DefaultPlatform {
		t.Fatalf("no line can run trams longer than %d sections", longest)
	}
	g.rebuild()
	for i := 0; i < 15*3600; i++ {
		g.sim.Step()
		if i%7 == 0 {
			if err := g.sim.CheckInvariants(); err != nil {
				t.Fatalf("%.0fs: %v", g.sim.Seconds(), err)
			}
		}
	}
	if g.sim.Stats.TramArrivals < 20 {
		t.Errorf("only %d stops served", g.sim.Stats.TramArrivals)
	}
}

func TestRestartClearsAMap(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	a := NewApp(false)
	if err := a.Open("grid", true); err != nil {
		t.Fatal(err)
	}
	a.SaveAll()
	if _, ok := SavedAt("grid"); !ok {
		t.Fatal("demo progress was not saved")
	}
	b := NewApp(false)
	if !b.games[0].hasProgress() || len(b.games[0].net.Lines) == 0 {
		t.Fatal("a new start screen should pick the save up")
	}
	for i, l := range b.games[0].net.LineIDs() {
		c, why := b.games[0].net.LineCapacity(b.games[0].net.Lines[l])
		t.Logf("line %d: %d sections max (%s)", i+1, c, why)
	}
	b.restart(0)
	if b.games[0].hasProgress() {
		t.Fatal("restart kept the track")
	}
	if _, ok := SavedAt("grid"); ok {
		t.Fatal("restart kept the save")
	}
	c := NewApp(true)
	c.Open("grid", true)
	c.SaveAll()
	if _, ok := SavedAt("grid"); ok {
		t.Fatal("a no-save app wrote a save")
	}
}

func TestSaveSlots(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	lines := len(g.net.Lines)
	if err := g.SaveSlot(2, "  两条线  "); err != nil {
		t.Fatal(err)
	}
	s := g.Slots(false)[2]
	if !s.Exists || s.Name != "两条线" || s.Lines != lines || s.Stops != len(g.net.Stops) {
		t.Fatalf("slot 3: %+v", s)
	}
	// Saving again without a name keeps it; renaming keeps the time.
	if err := g.SaveSlot(2, ""); err != nil {
		t.Fatal(err)
	}
	at := g.Slots(false)[2].SavedAt
	if err := g.RenameSlot(2, "改名"); err != nil {
		t.Fatal(err)
	}
	if s := g.Slots(false)[2]; s.Name != "改名" || !s.SavedAt.Equal(at) {
		t.Fatalf("rename: %+v", s)
	}
	if _, err := cleanSlotName("a\nb"); err == nil {
		t.Error("newline accepted in a name")
	}

	// Change the network, then load the slot back: the changed state is
	// kept as an autosave.
	g.net.DeleteLine(g.net.LineIDs()[0])
	if err := g.LoadSlot(false, 2, true); err != nil {
		t.Fatal(err)
	}
	if len(g.net.Lines) != lines {
		t.Fatalf("loaded %d lines, want %d", len(g.net.Lines), lines)
	}
	auto := g.Slots(true)
	if !auto[0].Exists || auto[0].Lines != lines-1 {
		t.Fatalf("autosave before load: %+v", auto[0])
	}
	// The loaded slot becomes the current progress.
	if err := g.Save(false); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil || len(h.net.Lines) != lines {
		t.Fatalf("current progress after loading a slot: %v %v", ok, err)
	}
	if err := g.LoadSlot(false, 5, true); err == nil {
		t.Error("loaded an empty slot")
	}
	if err := g.DeleteSlot(2); err != nil || g.Slots(false)[2].Exists {
		t.Fatalf("delete: %v", err)
	}
	if New("seattle").Slots(false)[2].Exists {
		t.Error("slots are shared between maps")
	}
}

func TestAutoSavesRotate(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	ids := g.net.LineIDs()
	for i := 0; i < MaxAutoSaves+2; i++ {
		g.net.Lines[ids[0]].Trams = i + 1 // a change each time
		if err := g.autoSave(); err != nil {
			t.Fatal(err)
		}
		if err := g.autoSave(); err != nil { // unchanged: no new autosave
			t.Fatal(err)
		}
	}
	n := 0
	for _, s := range g.Slots(true) {
		if s.Exists {
			n++
		}
	}
	if n != MaxAutoSaves {
		t.Fatalf("%d autosaves, want %d", n, MaxAutoSaves)
	}
	// The newest autosave holds the last change.
	p := &slotPanel{}
	p.list = g.Slots(true)
	h := New("grid")
	if err := h.LoadSlot(true, newest(p.list), false); err != nil {
		t.Fatal(err)
	}
	if h.net.Lines[ids[0]].Trams != MaxAutoSaves+2 {
		t.Fatalf("newest autosave has %d trams", h.net.Lines[ids[0]].Trams)
	}
}

func TestSaveKeepsPassengerSettings(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	id := g.net.LineIDs()[0]
	g.net.Lines[id].PerSection = 55
	g.passIdx = 2
	if err := g.Save(true); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if h.net.Lines[id].SectionLoad() != 55 || h.passIdx != 2 || h.sim.PassengerRate != 2 {
		t.Fatalf("per section %d, rate %d", h.net.Lines[id].SectionLoad(), h.passIdx)
	}
}

// The car count is saved as a number; older saves stored an index into
// 0/200/400/600 and get the nearest of today's choices.
func TestCarCountSaved(t *testing.T) {
	for old, want := range map[int]int{0: 200, 1: 200, 2: 500, 3: 500} {
		if got := densities[nearestDensity([]int{0, 200, 400, 600}[old])]; got != want {
			t.Errorf("old index %d → %d cars, want %d", old, got, want)
		}
	}
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.setDensity(4)
	if err := g.Save(true); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if densities[h.densityIdx] != 3000 || h.sim.TargetCars != 3000 {
		t.Errorf("loaded %d cars (sim %d), want 3000", densities[h.densityIdx], h.sim.TargetCars)
	}
}

// Leaving a map always leaves an autosave, even when the track did not
// change; leaving again unchanged refreshes it instead of adding copies.
func TestExitAutosaves(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	if err := g.Save(true); err != nil {
		t.Fatal(err)
	}
	h := New("grid")
	if ok, err := h.Load(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	h.updateAutoSave() // as when the map is opened: nothing changed yet
	h.SetCamera(321, 654, 1.5)
	if err := h.exitSave(); err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		for _, s := range h.Slots(true) {
			if s.Exists {
				n++
			}
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("%d autosaves after leaving, want 1", count())
	}
	first := h.Slots(true)[newest(h.Slots(true))].SavedAt
	if err := h.exitSave(); err != nil {
		t.Fatal(err)
	}
	if count() != 1 || !h.Slots(true)[newest(h.Slots(true))].SavedAt.After(first) {
		t.Fatalf("leaving again: %d autosaves, refreshed %v", count(), h.Slots(true)[newest(h.Slots(true))].SavedAt.After(first))
	}
	// The current progress kept the camera.
	k := New("grid")
	if ok, err := k.Load(); !ok || err != nil || k.camX != 321 || k.zoom != 1.5 {
		t.Fatalf("camera not saved on leaving: %v %v %.0f@%.1f", ok, err, k.camX, k.zoom)
	}
	// A changed network adds a second autosave.
	h.net.DeleteLine(h.net.LineIDs()[0])
	if err := h.exitSave(); err != nil || count() != 2 {
		t.Fatalf("after a change: %v, %d autosaves", err, count())
	}
}

// A save keeps the running service: loaded again (as after quitting the
// game), the trams are where they were, not back at the first stop.
func TestSaveKeepsTramPositions(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.Run()
	g.Warp(400)
	type pos struct {
		link street.LinkID
		s    float64
		load int
	}
	trams := map[int]pos{}
	for _, v := range g.sim.Vehicles {
		if v.Kind == traffic.TramKind {
			id, s := v.Link()
			trams[v.ID] = pos{id, s, v.Load()}
		}
	}
	if len(trams) == 0 {
		t.Fatal("no trams running")
	}
	if err := g.SaveSlot(0, "mid-run"); err != nil {
		t.Fatal(err)
	}
	if err := g.exitSave(); err != nil {
		t.Fatal(err)
	}
	for _, load := range []func(h *Game) error{
		func(h *Game) error { _, err := h.Load(); return err },
		func(h *Game) error { return h.LoadSlot(false, 0, false) },
	} {
		h := New("grid") // a fresh start of the game
		if err := load(h); err != nil {
			t.Fatal(err)
		}
		h.Run()
		if h.sim.Seconds() != g.sim.Seconds() {
			t.Fatalf("clock %.0fs, want %.0fs", h.sim.Seconds(), g.sim.Seconds())
		}
		found := 0
		for _, v := range h.sim.Vehicles {
			if p, ok := trams[v.ID]; ok {
				id, s := v.Link()
				if id != p.link || s != p.s || v.Load() != p.load {
					t.Fatalf("tram %d at link %d %.1f m (load %d), was link %d %.1f m (load %d)", v.ID, id, s, v.Load(), p.link, p.s, p.load)
				}
				found++
			}
		}
		if found != len(trams) {
			t.Fatalf("%d of %d trams restored", found, len(trams))
		}
		h.Warp(60) // and it carries on
	}
}
