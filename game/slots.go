package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Besides the current progress (<key>.json, autosaved every few seconds and
// opened by "continue"), every map has its own save slots: ten manual slots
// the player saves to, names and deletes, and five autosaves written in turn
// while playing, so earlier states can be brought back.
const (
	MaxSlots     = 10
	MaxAutoSaves = 5
	MaxSlotName  = 24 // runes
)

// AutoSaveEvery is how often a changed map adds an autosave.
var AutoSaveEvery = 5 * time.Minute

// slotInfo describes one slot for the list.
type slotInfo struct {
	Exists       bool
	Name         string
	SavedAt      time.Time
	Stops, Lines int
	Err          bool // unreadable file
}

func slotPath(key string, auto bool, i int) string {
	kind := "slot"
	if auto {
		kind = "auto"
	}
	return filepath.Join(SaveDir(), key, fmt.Sprintf("%s-%02d.json", kind, i+1))
}

// readSlot reads a slot's summary without loading it.
func readSlot(path string) slotInfo {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return slotInfo{}
	}
	var f struct {
		Name    string    `json:"name"`
		SavedAt time.Time `json:"savedAt"`
		Net     struct {
			Stops []json.RawMessage `json:"stops"`
			Lines []json.RawMessage `json:"lines"`
		} `json:"net"`
	}
	if err != nil || json.Unmarshal(data, &f) != nil {
		return slotInfo{Exists: true, Err: true}
	}
	return slotInfo{Exists: true, Name: f.Name, SavedAt: f.SavedAt, Stops: len(f.Net.Stops), Lines: len(f.Net.Lines)}
}

// Slots lists the map's manual slots or autosaves.
func (g *Game) Slots(auto bool) []slotInfo {
	n := MaxSlots
	if auto {
		n = MaxAutoSaves
	}
	out := make([]slotInfo, n)
	for i := range out {
		out[i] = readSlot(slotPath(g.key, auto, i))
	}
	return out
}

func cleanSlotName(name string) (string, error) {
	name = strings.TrimSpace(name)
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New(locale.T("名称不能包含换行或控制字符", "Names can't contain line breaks or control characters"))
		}
	}
	if utf8.RuneCountInString(name) > MaxSlotName {
		return "", fmt.Errorf(locale.T("名称最多 %d 个字", "Names have at most %d characters"), MaxSlotName)
	}
	return name, nil
}

// SaveSlot saves the map to a manual slot. An empty name keeps the slot's
// current name.
func (g *Game) SaveSlot(i int, name string) error {
	if i < 0 || i >= MaxSlots {
		return fmt.Errorf(locale.T("没有存档 %d", "There is no slot %d"), i+1)
	}
	name, err := cleanSlotName(name)
	if err != nil {
		return err
	}
	path := slotPath(g.key, false, i)
	if name == "" {
		name = readSlot(path).Name
	}
	_, f := g.state()
	f.SavedAt, f.Name = time.Now(), name
	return writeSave(path, f)
}

// LoadSlot replaces the map's progress with a slot. The progress it
// replaces is kept as an autosave first.
func (g *Game) LoadSlot(auto bool, i int, keep bool) error {
	path := slotPath(g.key, auto, i)
	if !readSlot(path).Exists {
		return errors.New(locale.T("这个位置还没有存档", "This slot is empty"))
	}
	if keep {
		if err := g.autoSave(); err != nil {
			return fmt.Errorf(locale.T("读取前自动存档失败：%v", "Autosave before loading failed: %v"), err)
		}
	}
	if _, err := g.loadPath(path); err != nil {
		return err
	}
	// The current progress becomes the loaded slot at the next autosave.
	g.lastSave, g.autoLast, g.autoAt = []byte{}, nil, time.Now()
	return nil
}

// RenameSlot changes a manual slot's name, keeping its time.
func (g *Game) RenameSlot(i int, name string) error {
	name, err := cleanSlotName(name)
	if err != nil {
		return err
	}
	path := slotPath(g.key, false, i)
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New(locale.T("这个位置还没有存档", "This slot is empty"))
	}
	var f saveFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf(locale.T("存档损坏：%v", "The save is damaged: %v"), err)
	}
	f.Name = name
	return writeSave(path, f)
}

// DeleteSlot removes a manual slot.
func (g *Game) DeleteSlot(i int) error {
	err := os.Remove(slotPath(g.key, false, i))
	if errors.Is(err, os.ErrNotExist) {
		return errors.New(locale.T("这个位置还没有存档", "This slot is empty"))
	}
	return err
}

// autoSave writes an autosave if the map changed since the last one,
// replacing an empty or the oldest autosave.
func (g *Game) autoSave() error {
	key, _ := g.state()
	if bytes.Equal(key, g.autoLast) || !g.hasProgress() {
		return nil
	}
	return g.writeAutoSave()
}

// writeAutoSave adds an autosave of the map as it is now, replacing an
// empty or the oldest one.
func (g *Game) writeAutoSave() error {
	key, f := g.state()
	slot, oldest := 0, time.Time{}
	for i, s := range g.Slots(true) {
		if !s.Exists {
			slot = i
			break
		}
		if i == 0 || s.SavedAt.Before(oldest) {
			slot, oldest = i, s.SavedAt
		}
	}
	f.SavedAt = time.Now()
	if err := writeSave(slotPath(g.key, true, slot), f); err != nil {
		return err
	}
	g.autoLast, g.autoAt = key, f.SavedAt
	return nil
}

// exitSave saves the map as the player leaves it (back to the menu, or
// quitting): the current progress, camera and settings included, and an
// autosave. If the newest autosave already holds the same network it is
// refreshed rather than adding a copy, so leaving never pushes an older,
// different autosave out for nothing.
func (g *Game) exitSave() error {
	if err := g.Save(g.hasProgress()); err != nil {
		return err
	}
	if !g.hasProgress() {
		return nil
	}
	key, f := g.state()
	slots := g.Slots(true)
	if i := newest(slots); i >= 0 {
		if data, err := os.ReadFile(slotPath(g.key, true, i)); err == nil {
			var old saveFile
			if json.Unmarshal(data, &old) == nil && bytes.Equal(stateKey(old), key) {
				f.SavedAt = time.Now()
				if err := writeSave(slotPath(g.key, true, i), f); err != nil {
					return err
				}
				g.autoLast, g.autoAt = key, f.SavedAt
				return nil
			}
		}
	}
	return g.writeAutoSave()
}

// updateAutoSave adds an autosave every AutoSaveEvery while the map changes.
func (g *Game) updateAutoSave() {
	if g.autoAt.IsZero() {
		g.autoAt = time.Now()
		g.autoLast, _ = g.state() // what was loaded is already saved
		return
	}
	if time.Since(g.autoAt) < AutoSaveEvery {
		return
	}
	g.autoAt = time.Now()
	if err := g.autoSave(); err != nil {
		g.say(locale.T("自动存档失败：%v", "Autosave failed: %v"), err)
	}
}
