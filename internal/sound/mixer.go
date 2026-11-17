// Package sound synthesizes the game's sounds as they play (no recordings):
// the trams' electric motors, the "dang dang dang" of the gong as a tram
// crosses a junction, and the two-note chime before an announcement. A
// Mixer is an endless stereo float32 PCM stream for an Ebitengine audio
// player; the game updates it from the game thread, the audio thread reads
// it. Engine-free, so it is tested without an audio device.
package sound

import (
	"encoding/binary"
	"math"
	"sort"
	"sync"
)

// SampleRate of the stream, Hz.
const SampleRate = 24000

// Volumes are the player's settings, each 0..1.
type Volumes struct {
	Motor, Bell, Announce, Doors float64
}

// DefaultVolumes is how the game starts.
func DefaultVolumes() Volumes { return Volumes{Motor: 0.6, Bell: 0.7, Announce: 0.8, Doors: 0.6} }

// Motor is what the trams near the screen sound like together: how loud
// (0..1), how fast the loudest is going (0..1 of full speed) and where it
// is, from -1 (left) to 1 (right).
type Motor struct {
	Loud, Speed, Pan float64
}

// Ring is one tram's gong while any part of it is in a crossing: how near
// it is (0..1) and where, -1 left to 1 right.
type Ring struct {
	Gain, Pan float64
}

// ringer is one tram's gong: it keeps striking in its style while the tram
// is in the crossing, then the last stroke dies away.
type ringer struct {
	gain, pan float64
	t         float64 // s since it started
	stopAt    float64 // when the tram left the crossing (-1 still in it)
}

type chime struct {
	t    float64
	gain float64
}

// Mixer mixes the motor hum, gongs and chimes.
type Mixer struct {
	mu      sync.Mutex
	volumes Volumes
	target  Motor
	// The motor follows its target smoothly.
	loud, speed, pan    float64
	phase1, phase2      float64
	phaseHum            float64
	noise               uint32
	rumbleLP, rumbleLP2 float64
	ringers             map[int]*ringer
	chimes              []chime
	paused              bool
	bellStyle           BellStyle
	motorStyle          MotorStyle
	doorStyle           DoorStyle
	doorVoices          []doorVoice
	jointPhase          float64 // rail joints passed (old trams clatter)
}

// NewMixer starts silent.
func NewMixer() *Mixer {
	return &Mixer{volumes: DefaultVolumes(), noise: 0x2f6e2b1, ringers: map[int]*ringer{}}
}

// SetStyles chooses the gong and the motor sound.
func (m *Mixer) SetStyles(b BellStyle, mo MotorStyle) {
	m.mu.Lock()
	m.bellStyle, m.motorStyle = b%BellStyles, mo%MotorStyles
	m.mu.Unlock()
}

// SetDoorStyle chooses the doors' sound.
func (m *Mixer) SetDoorStyle(d DoorStyle) {
	m.mu.Lock()
	m.doorStyle = d % DoorStyles
	m.mu.Unlock()
}

// MaxDoors is how many door sounds play at once.
const MaxDoors = 4

// Door plays a tram's doors opening (or the warning and closing), gain
// 0..1 by how near it is.
func (m *Mixer) Door(open bool, gain, pan float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.doorVoices) >= MaxDoors || gain <= 0 {
		return
	}
	m.doorVoices = append(m.doorVoices, doorVoice{open: open, gain: math.Min(1, gain), pan: math.Min(1, math.Max(-1, pan)), style: m.doorStyle})
}

// SetVolumes changes the three volumes.
func (m *Mixer) SetVolumes(v Volumes) {
	clamp := func(x float64) float64 { return math.Min(1, math.Max(0, x)) }
	m.mu.Lock()
	m.volumes = Volumes{clamp(v.Motor), clamp(v.Bell), clamp(v.Announce), clamp(v.Doors)}
	m.mu.Unlock()
}

// SetMotor sets what the motors near the screen sound like now.
func (m *Mixer) SetMotor(s Motor) {
	if math.IsNaN(s.Loud) || math.IsNaN(s.Speed) || math.IsNaN(s.Pan) {
		return
	}
	m.mu.Lock()
	m.target = Motor{math.Min(1, math.Max(0, s.Loud)), math.Min(1, math.Max(0, s.Speed)), math.Min(1, math.Max(-1, s.Pan))}
	m.mu.Unlock()
}

// SetPaused silences the motors (gongs and chimes ring out).
func (m *Mixer) SetPaused(p bool) {
	m.mu.Lock()
	m.paused = p
	m.mu.Unlock()
}

// MaxBells is how many gongs ring at once: the nearest.
const MaxBells = 4

// SetRinging says which trams (by ID) are in a crossing now. Their gongs
// keep striking until they leave it; a newly entering tram starts one.
func (m *Mixer) SetRinging(rings map[int]Ring) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// The nearest MaxBells only.
	type kv struct {
		id int
		r  Ring
	}
	var list []kv
	for id, r := range rings {
		if r.Gain > 0 {
			list = append(list, kv{id, r})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].r.Gain != list[j].r.Gain {
			return list[i].r.Gain > list[j].r.Gain
		}
		return list[i].id < list[j].id
	})
	if len(list) > MaxBells {
		list = list[:MaxBells]
	}
	keep := map[int]bool{}
	for _, e := range list {
		keep[e.id] = true
		g := m.ringers[e.id]
		if g == nil || g.stopAt >= 0 {
			g = &ringer{stopAt: -1}
			m.ringers[e.id] = g
		}
		g.gain, g.pan = math.Min(1, e.r.Gain), math.Min(1, math.Max(-1, e.r.Pan))
	}
	for id, g := range m.ringers {
		if id >= 0 && !keep[id] && g.stopAt < 0 {
			g.stopAt = g.t // out of the crossing: no more strokes
		}
	}
}

// PreviewBell rings the gong for a moment, as the player picks a style.
func (m *Mixer) PreviewBell() {
	m.mu.Lock()
	m.ringers[-1] = &ringer{gain: 1, stopAt: 1.5}
	m.mu.Unlock()
}

// Chime plays the two notes before an announcement.
func (m *Mixer) Chime() {
	m.mu.Lock()
	m.chimes = append(m.chimes, chime{gain: 1})
	m.mu.Unlock()
}

// Ringing reports how many gongs are sounding (for tests).
func (m *Mixer) Ringing() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ringers)
}

const dt = 1.0 / SampleRate

func (m *Mixer) rand() float64 {
	m.noise ^= m.noise << 13
	m.noise ^= m.noise >> 17
	m.noise ^= m.noise << 5
	return float64(m.noise)/float64(math.MaxUint32)*2 - 1
}

// chimeTone is the "ding-dong" before announcements: two soft bell notes.
func chimeTone(t float64) float64 {
	note := func(f, start float64) float64 {
		u := t - start
		if u < 0 || u > 2 {
			return 0
		}
		return math.Min(1, u/0.01) * math.Exp(-u*2.2) * (0.7*math.Sin(2*math.Pi*f*u) + 0.2*math.Sin(2*math.Pi*2*f*u))
	}
	return note(784, 0) + note(659, 0.55) // G5, E5
}

// sample is one stereo frame.
func (m *Mixer) sample() (l, r float64) {
	// Motor: follow the target over about a quarter of a second.
	tgt := m.target
	if m.paused {
		tgt.Loud = 0
	}
	k := dt / 0.25
	m.loud += (tgt.Loud - m.loud) * k
	m.speed += (tgt.Speed - m.speed) * k
	m.pan += (tgt.Pan - m.pan) * k
	if m.loud > 0.0005 {
		v := m.motorSample() * m.loud * m.volumes.Motor
		l += v * (1 - 0.5*math.Max(0, m.pan))
		r += v * (1 + 0.5*math.Min(0, m.pan))
	}
	// Gongs.
	for id, g := range m.ringers {
		v := 0.32 * ring(m.bellStyle, g.t, g.stopAt) * g.gain * m.volumes.Bell
		l += v * (1 - 0.5*math.Max(0, g.pan))
		r += v * (1 + 0.5*math.Min(0, g.pan))
		g.t += dt
		if g.stopAt >= 0 && g.t > g.stopAt+ringTail {
			delete(m.ringers, id)
		}
	}
	keptD := m.doorVoices[:0]
	for _, d := range m.doorVoices {
		v := 0.4 * m.doorSample(&d) * d.gain * m.volumes.Doors
		l += v * (1 - 0.5*math.Max(0, d.pan))
		r += v * (1 + 0.5*math.Min(0, d.pan))
		d.t += dt
		if d.t < doorSoundLength {
			keptD = append(keptD, d)
		}
	}
	m.doorVoices = keptD
	keptC := m.chimes[:0]
	for _, c := range m.chimes {
		v := 0.3 * chimeTone(c.t) * c.gain * m.volumes.Announce
		l, r = l+v, r+v
		c.t += dt
		if c.t < 2.6 {
			keptC = append(keptC, c)
		}
	}
	m.chimes = keptC
	return math.Max(-1, math.Min(1, l)), math.Max(-1, math.Min(1, r))
}

// Read fills p with stereo float32 little-endian frames (8 bytes each).
func (m *Mixer) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(p) / 8 * 8
	for i := 0; i < n; i += 8 {
		l, r := m.sample()
		binary.LittleEndian.PutUint32(p[i:], math.Float32bits(float32(l)))
		binary.LittleEndian.PutUint32(p[i+4:], math.Float32bits(float32(r)))
	}
	return n, nil
}
