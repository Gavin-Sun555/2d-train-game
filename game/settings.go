package game

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"github.com/Gavin-Sun555/2d-train-game/internal/sound"
)

// Settings that belong to the player rather than to a map: for now the
// display language. They live in settings.json beside the saves.
type settings struct {
	Language string `json:"language,omitempty"` // "zh" or "en"; empty: follow the system
	// Volumes of the tram motors, the crossing gong and announcements.
	Volumes *sound.Volumes `json:"volumes,omitempty"`
	// Sounds are the chosen gong, motor and announcer's voice.
	Sounds *soundStyles `json:"sounds,omitempty"`
}

func settingsPath() string { return filepath.Join(SaveDir(), "settings.json") }

func readSettings() settings {
	// Settings saved before a sound existed keep its default.
	vol, st := sound.DefaultVolumes(), soundStyles{}
	s := settings{Volumes: &vol, Sounds: &st}
	if data, err := os.ReadFile(settingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
	} else {
		s.Volumes, s.Sounds = nil, nil
	}
	return s
}

func writeSettings(s settings) error {
	data, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(SaveDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(settingsPath(), data, 0o644)
}

// InitLanguage picks the display language at start-up: the one given (a
// command-line choice, not remembered), else the player's saved choice,
// else the system's (Chinese if the system prefers Chinese, English
// otherwise).
func InitLanguage(code string) {
	if v := readSettings().Volumes; v != nil {
		setVolumes(*v, false)
	}
	if st := readSettings().Sounds; st != nil {
		setStyles(*st, false)
	}
	if l, ok := locale.FromCode(code); ok && code != "" {
		locale.Current = l
		return
	}
	if l, ok := locale.FromCode(readSettings().Language); ok {
		locale.Current = l
		return
	}
	locale.Current = locale.System()
}

// setLanguage switches the display language and remembers the choice.
func (a *App) setLanguage(l locale.Language) {
	locale.Current = l
	if a.NoSave {
		return
	}
	s := readSettings()
	s.Language = l.Code()
	if err := writeSettings(s); err != nil {
		a.notice = locale.T("无法保存设置：", "Couldn't save the settings: ") + err.Error()
	}
}
