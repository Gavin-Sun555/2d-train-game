package game

import (
	"strings"
	"testing"
	"time"

	"github.com/Gavin-Sun555/2d-train-game/internal/sound"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

type recorder struct {
	motor         sound.Motor
	loudest       float64
	bells, chimes int // bells: frames with a gong ringing
	ringing       map[int]sound.Ring
	paused        bool
	said          []string
	volumes       sound.Volumes
	styles        soundStyles
	opens, closes int
}

func (r *recorder) SetMotor(m sound.Motor) {
	r.motor = m
	r.loudest = max(r.loudest, m.Loud)
}
func (r *recorder) SetPaused(p bool) { r.paused = p }
func (r *recorder) SetRinging(m map[int]sound.Ring) {
	r.ringing = m
	if len(m) > 0 {
		r.bells++
	}
}
func (r *recorder) PreviewBell() {}
func (r *recorder) SetStyles(b sound.BellStyle, m sound.MotorStyle) {
	r.styles.Bell, r.styles.Motor = b, m
}
func (r *recorder) SetStyle(v sound.VoiceStyle)    { r.styles.Voice = v }
func (r *recorder) SetDoorStyle(d sound.DoorStyle) { r.styles.Door = d }
func (r *recorder) Door(open bool, gain, pan float64) {
	if open {
		r.opens++
	} else {
		r.closes++
	}
}
func (r *recorder) Chime()                                            { r.chimes++ }
func (r *recorder) SetVolumes(v sound.Volumes)                        { r.volumes = v }
func (r *recorder) Say(t string, en bool, v float64, d time.Duration) { r.said = append(r.said, t) }
func (r *recorder) Stop()                                             {}

func withSound(t *testing.T) *recorder {
	r := &recorder{}
	oldSink, oldSpeaker, oldVol := sink, speaker, volumes
	sink, speaker, volumes = r, r, sound.DefaultVolumes()
	t.Cleanup(func() { sink, speaker, volumes = oldSink, oldSpeaker, oldVol })
	return r
}

// Trams near the screen are heard, and gong at crossings; trams far away
// are not. The followed tram announces each stop before arriving and its
// way after leaving one.
func TestTramSounds(t *testing.T) {
	r := withSound(t)
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.Run()
	for g.follow == 0 && g.sim.Seconds() < 120 {
		g.sim.Step()
		g.FollowFirst() // the camera stays on a tram
	}
	if g.follow == 0 {
		t.Fatal("no tram to follow")
	}
	for i := 0; i < 8*60*60; i++ {
		g.sim.Step()
		g.updateFollow()
		g.updateSound()
	}
	if r.loudest == 0 {
		t.Error("no motor heard near the followed tram")
	}
	if r.bells == 0 {
		t.Error("no gong at a crossing")
	}
	arriving, leaving := 0, 0
	for _, s := range r.said {
		switch {
		case strings.Contains(s, "到站") || strings.Contains(s, "终点站"):
			arriving++
		case strings.Contains(s, "开往"):
			leaving++
		}
	}
	if r.opens == 0 || r.closes == 0 {
		t.Errorf("door sounds: %d opening, %d closing", r.opens, r.closes)
	}
	if arriving == 0 || leaving == 0 || r.chimes != len(r.said) {
		t.Errorf("announcements: %d arriving, %d leaving, %d chimes: %q", arriving, leaving, r.chimes, r.said)
	}
	// Far away: nothing heard, no gong.
	g.follow = 0
	g.SetCamera(-50000, -50000, 1)
	bells := r.bells
	said := len(r.said)
	for i := 0; i < 3*60*60; i++ {
		g.sim.Step()
		g.updateSound()
	}
	if r.motor.Loud != 0 || r.bells != bells || len(r.said) != said || len(r.ringing) != 0 {
		t.Errorf("heard from afar: motor %.2f, %d gongs, %d announcements", r.motor.Loud, r.bells-bells, len(r.said)-said)
	}
	// Paused: the motors stop.
	g.paused = true
	g.updateSound()
	if !r.paused {
		t.Error("not paused")
	}
	_ = traffic.TramKind
}

// The three volumes are separate, kept in steps of 10%, and remembered.
func TestVolumesRemembered(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	r := withSound(t)
	setVolumes(sound.Volumes{Motor: 0.33, Bell: 0, Announce: 1.4, Doors: 0.5}, true)
	if r.volumes != (sound.Volumes{Motor: 0.3, Bell: 0, Announce: 1, Doors: 0.5}) {
		t.Fatalf("volumes %+v", r.volumes)
	}
	volumes = sound.DefaultVolumes()
	InitLanguage("zh")
	if volumes != (sound.Volumes{Motor: 0.3, Bell: 0, Announce: 1, Doors: 0.5}) {
		t.Errorf("not remembered: %+v", volumes)
	}
}

// The gong rings on every frame any part of a tram is in a crossing (not
// two or three strokes); the stop is announced as the tram enters the last
// crossing before it, not further back.
func TestGongAndAnnouncementTiming(t *testing.T) {
	r := withSound(t)
	g := New("grid")
	if err := g.LoadDemo(); err != nil {
		t.Fatal(err)
	}
	g.Run()
	for g.follow == 0 && g.sim.Seconds() < 120 {
		g.sim.Step()
		g.FollowFirst()
	}
	inFrames, ringFrames := 0, 0
	announcedAt := []float64{}
	said := 0
	for i := 0; i < 10*60*60; i++ {
		g.sim.Step()
		g.updateFollow()
		g.updateSound()
		v := g.sim.Find(g.follow)
		if v == nil {
			t.Fatal("lost the tram")
		}
		if g.sim.InCrossing(v) {
			inFrames++
			if _, ok := r.ringing[v.ID]; ok {
				ringFrames++
			}
		}
		if len(r.said) > said {
			said = len(r.said)
			if strings.Contains(r.said[said-1], "到站") || strings.Contains(r.said[said-1], "终点站") {
				d, _ := g.sim.DistanceToStop(v)
				announcedAt = append(announcedAt, d)
			}
		}
	}
	if inFrames == 0 || ringFrames != inFrames {
		t.Errorf("in a crossing %d frames, gong ringing %d", inFrames, ringFrames)
	}
	if len(announcedAt) == 0 {
		t.Fatal("no arrival announced")
	}
	for _, d := range announcedAt {
		if d > 250 { // a block or so: the last crossing, not earlier
			t.Errorf("arrival announced %.0f m before the stop", d)
		}
	}
	t.Logf("arrivals announced at %v m", announcedAt)
}

// The styles are kept and remembered.
func TestSoundStylesRemembered(t *testing.T) {
	t.Setenv("TRAIN_GAME_SAVES", t.TempDir())
	r := withSound(t)
	setStyles(soundStyles{Bell: 2, Motor: 5, Voice: -1, Door: 6}, true)
	want := soundStyles{Bell: 2, Motor: 1, Voice: 3, Door: 2}
	if r.styles != want || styles != want {
		t.Fatalf("styles %+v / %+v, want %+v", r.styles, styles, want)
	}
	styles = soundStyles{}
	InitLanguage("zh")
	if styles != want {
		t.Errorf("not remembered: %+v", styles)
	}
	styles = soundStyles{}
}
