package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

const saveVersion = 1

// saveFile is one map's progress.
type saveFile struct {
	Version  int          `json:"version"`
	Map      string       `json:"map"`
	SavedAt  time.Time    `json:"savedAt"`
	Net      street.Saved `json:"net"`
	Camera   [3]float64   `json:"camera"`
	Layout   int          `json:"layout"`
	Platform int          `json:"platform"`
	Density  int          `json:"density"` // index (older saves: into 0/200/400/600)
	// Cars is the number of AI cars chosen (0 in older saves).
	Cars int    `json:"cars,omitempty"`
	Name string `json:"name,omitempty"` // player's name for a manual slot
	// Passengers is the passenger arrival speed + 1 (0: default).
	Passengers int `json:"passengers,omitempty"`
	// Landmarks are the player's names for landmarks, by OSM name.
	Landmarks map[string]string `json:"landmarks,omitempty"`
	// Sim is the running service (trams where they are, passengers, the
	// clock), so a loaded save carries on where it left off.
	Sim *traffic.Snapshot `json:"sim,omitempty"`
}

// SaveDir is where saves live: $TRAIN_GAME_SAVES, or the user's config
// folder (~/Library/Application Support/2d-train-game/saves on a Mac).
func SaveDir() string {
	if d := os.Getenv("TRAIN_GAME_SAVES"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "2d-train-game", "saves")
}

func savePath(key string) string { return filepath.Join(SaveDir(), key+".json") }

// hasProgress reports whether the player has built anything on the map.
func (g *Game) hasProgress() bool {
	if len(g.net.Edits()) > 0 {
		return true
	}
	if len(g.landmarkNames) > 0 {
		return true
	}
	for _, r := range g.net.Roads {
		if r.Tram != street.NoTram {
			return true
		}
	}
	return false
}

// state is the save contents, without the time stamp.
func (g *Game) state() ([]byte, saveFile) {
	f := saveFile{Version: saveVersion, Map: g.key, Net: g.net.Export(),
		Camera: [3]float64{g.camX, g.camY, g.zoom}, Layout: g.layout, Platform: g.platform, Density: g.densityIdx, Cars: densities[g.densityIdx], Passengers: g.passIdx + 1,
		Landmarks: g.landmarkSave()}
	if !g.dirty && g.sim.Tick > 0 {
		f.Sim = g.sim.Snapshot() // the service as it runs on the saved network
	}
	return stateKey(f), f
}

// Save writes the map's progress if it changed since the last save (or
// always, with force). A map with nothing built and no save is skipped.
func (g *Game) Save(force bool) error {
	key, f := g.state()
	if !force && bytes.Equal(key, g.lastSave) {
		return nil
	}
	if g.lastSave == nil && !g.hasProgress() {
		return nil
	}
	f.SavedAt = time.Now()
	if err := writeSave(savePath(g.key), f); err != nil {
		return err
	}
	g.lastSave = key
	return nil
}

// writeSave writes a save file atomically.
func writeSave(path string, f saveFile) error {
	data, err := json.Marshal(f) // compact: a running service holds thousands of vehicles
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load restores the map's saved progress. It reports false when there is
// no save.
func (g *Game) Load() (bool, error) { return g.loadPath(savePath(g.key)) }

// loadPath restores progress from a save file.
func (g *Game) loadPath(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var f saveFile
	if err := json.Unmarshal(data, &f); err != nil {
		return false, fmt.Errorf(locale.T("存档损坏：%v", "The save is damaged: %v"), err)
	}
	if f.Version != saveVersion || f.Map != g.key {
		return false, errors.New(locale.T("存档版本或地图不符", "The save is for another version or map"))
	}
	// The save is read onto the map's own network: its road edits are
	// replayed there, then its track, stops and lines.
	net := g.freshNet()
	moved, off := false, mapdata.Pt{}
	if g.scene != nil && len(f.Net.Edits) == 0 && f.Net.RoadCount != len(net.Roads) {
		// A save from an earlier, smaller Seattle map.
		f.Net, off, moved = migrateSeattle(f.Net, net)
	}
	if err := net.Import(f.Net); err != nil {
		return false, err
	}
	g.net = net
	g.attachCorridorCheck()
	g.roadsChanged()
	if f.Camera[2] > 0 {
		g.camX, g.camY, g.zoom = f.Camera[0], f.Camera[1], f.Camera[2]
		if moved {
			g.camX += off[0]
			g.camY += off[1]
		}
	}
	g.layout = min(max(f.Layout, 0), len(layouts)-1)
	if f.Platform > 0 {
		g.platform = min(max(f.Platform, street.MinSections), street.MaxSections)
	}
	cars := f.Cars
	if cars == 0 { // an older save: its index was into 0, 200, 400, 600
		cars = []int{0, 200, 400, 600}[min(max(f.Density, 0), 3)]
	}
	g.densityIdx = nearestDensity(cars)
	g.landmarkNames = nil
	for k, v := range f.Landmarks {
		if g.landmarkNames == nil {
			g.landmarkNames = map[string]string{}
		}
		g.landmarkNames[k] = v
	}
	if f.Passengers > 0 {
		g.passIdx = min(f.Passengers-1, len(passNames())-1)
	}
	g.rebuild()
	if f.Sim != nil && !moved {
		if err := g.sim.Restore(f.Sim); err != nil {
			g.say(locale.T("运营状态无法恢复，从头开始运营：%v", "The service couldn't be resumed and starts afresh: %v"), err)
		}
	}
	g.lastSave, _ = g.state()
	if moved {
		g.lastSave = []byte{} // write the converted save
	}
	return true, nil
}

// SavedAt is when the map was last saved.
func SavedAt(key string) (time.Time, bool) {
	data, err := os.ReadFile(savePath(key))
	if err != nil {
		return time.Time{}, false
	}
	var f struct {
		SavedAt time.Time `json:"savedAt"`
	}
	if json.Unmarshal(data, &f) != nil {
		return time.Time{}, false
	}
	return f.SavedAt, true
}

// DeleteSave removes the map's save.
func DeleteSave(key string) error {
	err := os.Remove(savePath(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// updateSave saves on Cmd/Ctrl+S, and every few seconds when the network
// has changed.
func (g *Game) updateSave(enabled bool) {
	if !enabled {
		return
	}
	mod := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if mod && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if err := g.Save(true); err != nil {
			g.say(locale.T("保存失败：%v", "Saving failed: %v"), err)
		} else if g.lastSave == nil {
			g.say("%s", locale.T("还没有建设任何轨道，无需保存", "Nothing built yet, nothing to save"))
		} else {
			g.say("%s", locale.T("已保存", "Saved"))
		}
		return
	}
	g.saveTick++
	// While the service runs, the trams' positions are kept up to date in
	// the progress file once a minute, in case the game is not closed
	// normally (leaving the map or quitting always saves).
	if g.saveTick%(60*60) == 0 && g.page == pageRun && !g.paused && g.hasProgress() {
		if err := g.Save(true); err != nil {
			g.say(locale.T("自动保存失败：%v", "Autosave failed: %v"), err)
		}
		return
	}
	if g.saveTick%(60*5) == 0 {
		if err := g.Save(false); err != nil {
			g.say(locale.T("自动保存失败：%v", "Autosave failed: %v"), err)
		}
	}
	g.updateAutoSave()
}

// nearestDensity is the car-count option closest to n.
func nearestDensity(n int) int {
	best := 0
	for i, d := range densities {
		if abs(d-n) < abs(densities[best]-n) {
			best = i
		}
	}
	return best
}

// stateKey is what decides whether a save changed: the network and the
// landmark names (not the camera or the time).
func stateKey(f saveFile) []byte {
	b, _ := json.Marshal(struct {
		Net       street.Saved
		Landmarks map[string]string
	}{f.Net, f.Landmarks})
	return b
}
