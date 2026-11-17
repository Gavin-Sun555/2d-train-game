package game

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"github.com/Gavin-Sun555/2d-train-game/internal/sound"
	"github.com/Gavin-Sun555/2d-train-game/street"
	"github.com/Gavin-Sun555/2d-train-game/traffic"
)

// Sound: the motors of the trams on or near the screen, the gong as one of
// them crosses a junction, and, for the tram being followed, an
// announcement before each stop and after leaving one. Each has its own
// volume. The sounds are synthesized (internal/sound); announcements are
// spoken by the system's voice where there is one.

// audioSink is what the game drives: the mixer, or a recorder in tests.
type audioSink interface {
	SetMotor(sound.Motor)
	SetPaused(bool)
	SetRinging(map[int]sound.Ring)
	PreviewBell()
	Chime()
	SetVolumes(sound.Volumes)
	SetStyles(sound.BellStyle, sound.MotorStyle)
	SetDoorStyle(sound.DoorStyle)
	Door(open bool, gain, pan float64)
}

// speechSink reads announcements aloud.
type speechSink interface {
	Say(text string, english bool, volume float64, delay time.Duration)
	Stop()
	SetStyle(sound.VoiceStyle)
}

var (
	audioOnce sync.Once
	sink      audioSink  // nil: no sound (tests, screenshots)
	speaker   speechSink // nil: no spoken announcements
	volumes   = sound.DefaultVolumes()
	styles    soundStyles
	player    *audio.Player
)

// EnableAudio opens the sound output (the real game only; tests and
// screenshots stay silent). Spoken announcements need the system's voice.
func EnableAudio() error {
	var err error
	audioOnce.Do(func() {
		ctx := audio.NewContext(sound.SampleRate)
		m := sound.NewMixer()
		m.SetVolumes(volumes)
		m.SetStyles(styles.Bell, styles.Motor)
		m.SetDoorStyle(styles.Door)
		player, err = ctx.NewPlayerF32(m)
		if err != nil {
			return
		}
		// Enough queued to never run dry between the audio callbacks.
		player.SetBufferSize(160 * time.Millisecond)
		player.Play()
		sink = m
		if s, e := sound.NewSpeaker(); e == nil {
			s.SetStyle(styles.Voice)
			speaker = s
		}
	})
	return err
}

// soundState is what one game remembers between frames.
type soundState struct {
	follow    int                       // the tram the announcements are for
	announced int                       // call it was announced arriving at (call+1; 0 none)
	leaving   bool                      // it was standing at a stop last frame
	crossed   bool                      // it has been through a crossing since it left the stop
	doors     map[int]traffic.DoorState // each tram's doors last frame
}

// soundStyles are the player's choices of gong, motor and voice.
type soundStyles struct {
	Bell  sound.BellStyle  `json:"bell"`
	Motor sound.MotorStyle `json:"motor"`
	Voice sound.VoiceStyle `json:"voice"`
	Door  sound.DoorStyle  `json:"door"`
}

func doorNames() []string {
	return locale.L([]string{"气动门", "电动塞拉门", "老式车门", "柔和提示"}, []string{"Air doors", "Electric doors", "Classic doors", "Soft chime"})
}

func bellNames() []string {
	return locale.L([]string{"当当铜钟", "欧式电铃", "叮叮双响", "柔和提示音"}, []string{"Clang gong", "Electric bell", "Ding-ding", "Soft chime"})
}

func motorNames() []string {
	return locale.L([]string{"现代低地板", "变频器“唱歌”", "老式电车", "安静"}, []string{"Modern low-floor", "Singing inverter", "Classic tram", "Quiet"})
}

func voiceNames() []string {
	return locale.L([]string{"女声 1", "女声 2", "男声 1", "男声 2"}, []string{"Female 1", "Female 2", "Male 1", "Male 2"})
}

// Hearing distances, in screen pixels beyond the edge of the window.
const hearMargin = 200.0

// audibility is how loud something at p is, 0..1 (1 on screen, fading out
// within hearMargin of the edge, quieter zoomed far out), and where it is,
// -1 left to 1 right.
func (g *Game) audibility(p street.Vec) (gain, pan float64) {
	x, y := g.toScreen(p)
	w, h := float64(g.w), float64(g.h)
	out := math.Max(math.Max(-float64(x), float64(x)-w), math.Max(-float64(y), float64(y)-h))
	if out > hearMargin {
		return 0, 0
	}
	gain = 1
	if out > 0 {
		gain = 1 - out/hearMargin
	}
	gain *= math.Min(1, math.Max(0.25, g.zoom*1.5))
	pan = math.Max(-1, math.Min(1, (float64(x)-w/2)/(w/2)))
	return gain, pan
}

// updateSound runs once a frame on the open map.
func (g *Game) updateSound() {
	running := g.page == pageRun && !g.paused && g.slots == nil
	if running {
		g.updateAnnouncements() // shown as text even without sound
	}
	if sink == nil {
		return
	}
	sink.SetPaused(!running)
	if !running {
		sink.SetMotor(sound.Motor{})
		sink.SetRinging(nil)
		return
	}
	// Motors: the nearest trams count most; the loudest sets the pitch.
	var loud, speed, pan, best float64
	rings := map[int]sound.Ring{}
	if g.sound.doors == nil {
		g.sound.doors = map[int]traffic.DoorState{}
	}
	doorsNow := map[int]traffic.DoorState{}
	for _, v := range g.sim.Vehicles {
		if v.Kind != traffic.TramKind {
			continue
		}
		id, pos := v.Link()
		p, _ := g.net.Links[id].PointAt(pos)
		gain, pn := g.audibility(p)
		if gain <= 0 {
			continue
		}
		frac := math.Min(1, v.V/math.Max(v.V0, 40/3.6))
		contrib := gain * (0.35 + 0.65*frac)
		if v.Dwelling {
			contrib = gain * 0.2 // standing: auxiliaries only
		}
		loud += contrib
		if contrib > best {
			best, speed, pan = contrib, frac, pn
		}
		// The doors: a sound as they start to open, and as the closing
		// warning starts.
		ds, _ := v.Doors()
		doorsNow[v.ID] = ds
		if was, ok := g.sound.doors[v.ID]; ok && was != ds {
			switch ds {
			case traffic.DoorsOpening:
				sink.Door(true, gain, pn)
			case traffic.DoorsClosing:
				sink.Door(false, gain, pn)
			}
		}
		// The gong rings for as long as any part of it is in a crossing.
		if g.sim.InCrossing(v) {
			rings[v.ID] = sound.Ring{Gain: gain, Pan: pn}
		}
	}
	g.sound.doors = doorsNow
	sink.SetRinging(rings)
	sink.SetMotor(sound.Motor{Loud: math.Min(1, loud), Speed: speed, Pan: pan})
}

// Announcements for the followed tram: approaching a stop ("Arriving
// at …"), as it enters the last crossing before the stop (or, with no
// crossing on the way, a little before it), and after leaving one ("This
// tram is bound for …, next stop …").
const announceDistance = 100.0 // m before a stop with no crossing on the way

func (g *Game) updateAnnouncements() {
	s := &g.sound
	v := g.sim.Find(g.follow)
	if v == nil || g.follow == 0 {
		s.follow, s.announced, s.leaving = 0, 0, false
		return
	}
	l := g.net.Lines[v.Line]
	if l == nil {
		return
	}
	if s.follow != v.ID {
		// A newly followed tram: no announcement for where it already is.
		s.follow, s.leaving, s.crossed = v.ID, v.Dwelling, false
		s.announced = 0
		if g.sim.PastLastCrossing(v) {
			s.announced = g.sim.Call(v) + 1 // too late for this stop
		}
		return
	}
	call := g.sim.Call(v)
	stop := g.stopName(g.sim.TramStop(v))
	if s.leaving && !v.Dwelling {
		s.crossed = false
		g.announce(fmt.Sprintf(locale.T("本次电车开往 %s。下一站：%s。", "This tram is bound for %s. The next stop is %s."), g.lineEnd(l, call), stop))
	}
	s.leaving = v.Dwelling
	if g.sim.InCrossing(v) {
		s.crossed = true
	}
	d, ok := g.sim.DistanceToStop(v)
	due := ok && g.sim.PastLastCrossing(v) && (s.crossed || d <= announceDistance)
	if due && s.announced != call+1 {
		s.announced = call + 1
		if l.TurnsAt(call) {
			g.announce(fmt.Sprintf(locale.T("终点站 %s 就要到了，请全体乘客带好随身物品下车。", "Arriving at %s, the end of the line. All passengers please leave the tram."), stop))
		} else {
			g.announce(fmt.Sprintf(locale.T("前方到站：%s，下车的乘客请做好准备。", "Arriving at %s. Please get ready to leave the tram."), stop))
		}
	}
}

// lineEnd is where the tram running towards call turns back.
func (g *Game) lineEnd(l *street.Line, call int) string {
	calls := l.Calls()
	for k := 0; k < len(calls); k++ {
		i := (call + k) % len(calls)
		if l.TurnsAt(i) {
			return g.stopName(calls[i])
		}
	}
	return g.stopName(calls[len(calls)-1])
}

func (g *Game) stopName(id street.StopID) string {
	if st := g.net.Stops[id]; st != nil {
		return st.Name
	}
	return "—"
}

// announce plays the chime and then reads the text.
func (g *Game) announce(text string) {
	g.lastAnnouncement = text
	g.say("%s", locale.T("报站：", "Announcement: ")+text)
	if sink == nil || volumes.Announce <= 0 {
		return
	}
	sink.Chime()
	if speaker != nil {
		speaker.Say(text, locale.Current == locale.English, volumes.Announce, 1300*time.Millisecond)
	}
}

// silence stops the motors and any announcement (leaving a map).
func silence() {
	if sink != nil {
		sink.SetMotor(sound.Motor{})
		sink.SetPaused(true)
	}
	if speaker != nil {
		speaker.Stop()
	}
}

// setStyles changes the gong, motor and voice, applies and remembers them.
func setStyles(st soundStyles, remember bool) {
	st.Bell = (st.Bell%sound.BellStyles + sound.BellStyles) % sound.BellStyles
	st.Motor = (st.Motor%sound.MotorStyles + sound.MotorStyles) % sound.MotorStyles
	st.Voice = (st.Voice%sound.VoiceStyles + sound.VoiceStyles) % sound.VoiceStyles
	st.Door = (st.Door%sound.DoorStyles + sound.DoorStyles) % sound.DoorStyles
	styles = st
	if sink != nil {
		sink.SetStyles(st.Bell, st.Motor)
		sink.SetDoorStyle(st.Door)
	}
	if speaker != nil {
		speaker.SetStyle(st.Voice)
	}
	if remember {
		s := readSettings()
		s.Sounds = &styles
		_ = writeSettings(s)
	}
}

// setVolumes changes the volumes, applies and remembers them.
func setVolumes(v sound.Volumes, remember bool) {
	clamp := func(x float64) float64 { return math.Round(math.Min(1, math.Max(0, x))*10) / 10 }
	volumes = sound.Volumes{Motor: clamp(v.Motor), Bell: clamp(v.Bell), Announce: clamp(v.Announce), Doors: clamp(v.Doors)}
	if sink != nil {
		sink.SetVolumes(volumes)
	}
	if volumes.Announce == 0 && speaker != nil {
		speaker.Stop()
	}
	if remember {
		s := readSettings()
		s.Volumes = &volumes
		_ = writeSettings(s)
	}
}
