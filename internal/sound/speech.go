package sound

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Speaker reads announcements aloud with the system's voices (macOS `say`,
// as in the bus game). Each new announcement cuts off the one before.
type Speaker struct {
	mu     sync.Mutex
	path   string
	zh, en [VoiceStyles]string // voices by style
	style  VoiceStyle
	cancel context.CancelFunc
}

// voicePreferences are the voices tried for a style, Chinese and English
// (as in the bus game).
func voicePreferences(v VoiceStyle) (zh, en []string) {
	switch v {
	case VoiceFemale2:
		return []string{"Shelley", "Flo", "Sandy", "Tingting"}, []string{"Shelley", "Flo", "Sandy", "Samantha"}
	case VoiceMale1:
		return []string{"Reed", "Eddy", "Rocko", "Grandpa"}, []string{"Reed", "Eddy", "Rocko", "Grandpa"}
	case VoiceMale2:
		return []string{"Eddy", "Rocko", "Reed", "Grandpa"}, []string{"Eddy", "Rocko", "Reed", "Grandpa"}
	}
	return []string{"Tingting", "Shelley", "Flo"}, []string{"Samantha", "Shelley", "Flo"}
}

// SetStyle chooses the announcer's voice.
func (s *Speaker) SetStyle(v VoiceStyle) {
	s.mu.Lock()
	s.style = v % VoiceStyles
	s.mu.Unlock()
}

// NewSpeaker finds the system speech; it fails where there is none (the
// game then plays the chime alone).
func NewSpeaker() (*Speaker, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("spoken announcements need macOS")
	}
	path, err := exec.LookPath("/usr/bin/say")
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(path, "-v", "?").Output()
	if err != nil {
		return nil, err
	}
	list := string(out)
	s := &Speaker{path: path}
	for v := VoiceStyle(0); v < VoiceStyles; v++ {
		zh, en := voicePreferences(v)
		s.zh[v], s.en[v] = pickVoice(list, "zh_CN", zh), pickVoice(list, "en_US", en)
	}
	return s, nil
}

// pickVoice chooses a voice for a locale from `say -v ?` output: the first
// preferred name available in it, else any voice of that locale.
func pickVoice(list, locale string, preferred []string) string {
	var any string
	names := map[string]string{} // first word of the name -> full name
	for _, line := range strings.Split(list, "\n") {
		i := strings.Index(line, "#")
		if i < 0 {
			continue
		}
		fields := strings.Fields(line[:i])
		if len(fields) < 2 || fields[len(fields)-1] != locale {
			continue
		}
		full := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[:i]), locale))
		first := strings.Fields(full)[0]
		if _, ok := names[first]; !ok {
			names[first] = full
		}
		if any == "" {
			any = full
		}
	}
	for _, p := range preferred {
		if v, ok := names[p]; ok {
			return v
		}
	}
	return any
}

// Say reads text after delay (to let the chime ring first), at volume
// 0..1, in Chinese or English.
func (s *Speaker) Say(text string, english bool, volume float64, delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	voice := s.zh[s.style]
	if english {
		voice = s.en[s.style]
	}
	go func() {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		args := []string{"-r", "175"}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		cmd := exec.CommandContext(ctx, s.path, args...)
		cmd.Stdin = strings.NewReader(SpeechText(text, volume))
		_ = cmd.Run()
	}()
}

// Stop cuts off any announcement.
func (s *Speaker) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// SpeechText is what `say` reads: the volume as an embedded command, and
// the text with anything that looks like a speech command defused (stop
// names are the player's to change).
func SpeechText(text string, volume float64) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "[[", "［［"), "]]", "］］")
	return fmt.Sprintf("[[volm %.2f]]%s", minf(1, maxf(0, volume)), text)
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
