package sound

import "math"

// BellStyle is the sound of a tram's gong, after real trams.
type BellStyle int

const (
	BellClang    BellStyle = iota // a heavy gong, struck about twice a second: "dang, dang"
	BellElectric                  // an electric bell rung continuously, as on European trams
	BellDingDing                  // two quick strokes, then a pause: Hong Kong's "ding ding"
	BellSoft                      // a gentle two-note chime
	BellStyles
)

// MotorStyle is the sound of a tram's traction equipment.
type MotorStyle int

const (
	MotorModern  MotorStyle = iota // a modern low-floor tram: smooth inverter whine
	MotorVVVF                      // an early inverter that "sings" up a scale
	MotorClassic                   // an old tram: gear growl and rail joints
	MotorQuiet                     // rolling noise only
	MotorStyles
)

// VoiceStyle picks the announcer's voice.
type VoiceStyle int

const (
	VoiceFemale1 VoiceStyle = iota
	VoiceFemale2
	VoiceMale1
	VoiceMale2
	VoiceStyles
)

// ringTail is how long the last stroke rings on.
const ringTail = 1.6

// bellPattern is a style's strokes: offsets within each period (s).
func bellPattern(b BellStyle) (period float64, offsets []float64) {
	switch b {
	case BellElectric:
		return 0.11, []float64{0}
	case BellDingDing:
		return 1.1, []float64{0, 0.17}
	case BellSoft:
		return 1.4, []float64{0, 0.45}
	}
	return 0.55, []float64{0}
}

// stroke is the sound of one stroke, k counting strokes within a period.
func stroke(b BellStyle, t float64, k int) float64 {
	if t < 0 || t > ringTail {
		return 0
	}
	attack := math.Min(1, t/0.002)
	sin := func(f, decay, a float64) float64 { return a * math.Sin(2*math.Pi*f*t) * math.Exp(-t*decay) }
	switch b {
	case BellElectric:
		// A small, bright bell struck by a fast hammer: short and ringing.
		return attack * (sin(1880, 9, 0.5) + sin(4250, 16, 0.25) + sin(6020, 25, 0.12))
	case BellDingDing:
		f := 1320.0
		return attack * (sin(f, 4.5, 0.55) + sin(f*2.76, 8, 0.25) + sin(f*5.4, 14, 0.1))
	case BellSoft:
		f := []float64{988, 784}[k%2] // B5, G5
		return attack * (sin(f, 2.6, 0.55) + sin(2*f, 5, 0.15))
	}
	// The classic gong: a heavy bowl, low and long.
	f := 680.0
	return attack * (sin(f, 2.2, 0.55) + sin(f*2.32, 3.4, 0.3) + sin(f*4.25, 6, 0.18) + sin(f*6.8, 10, 0.08))
}

// ring is a gong at time t, struck in its pattern until stopAt (-1: still
// struck).
func ring(b BellStyle, t, stopAt float64) float64 {
	period, offs := bellPattern(b)
	s := 0.0
	first := int(math.Floor((t - ringTail) / period))
	for c := max(first, 0); float64(c)*period <= t; c++ {
		for k, o := range offs {
			at := float64(c)*period + o
			if at > t || (stopAt >= 0 && at > stopAt) {
				continue
			}
			s += stroke(b, t-at, k)
		}
	}
	return s
}

// motorSample is one sample of the motors in the mixer's style.
func (m *Mixer) motorSample() float64 {
	adv := func(p *float64, f float64) float64 {
		*p += 2 * math.Pi * f * dt
		if *p > 2*math.Pi {
			*p -= 2 * math.Pi
		}
		return *p
	}
	m.rumbleLP += (m.rand() - m.rumbleLP) * 0.05
	m.rumbleLP2 += (m.rumbleLP - m.rumbleLP2) * 0.05
	rumble := 1.6 * m.rumbleLP2 * m.speed
	sp := m.speed
	switch m.motorStyle {
	case MotorVVVF:
		// Early inverters step their carrier up a scale while accelerating:
		// do-re-mi ... then a steady whine.
		scale := []float64{349, 392, 440, 494, 523, 587, 659, 698, 784}
		step := min(int(sp*12), len(scale)-1)
		f := scale[step]
		if sp > 0.75 {
			f = 784 + 600*(sp-0.75)
		}
		whine := 0.11*math.Sin(adv(&m.phase1, f)) + 0.05*math.Sin(adv(&m.phase2, 2*f))
		hum := 0.08 * math.Sin(adv(&m.phaseHum, 100))
		return whine*(0.3+0.7*math.Min(1, sp*3)) + hum + rumble
	case MotorClassic:
		// Gears growl low; the wheels knock over rail joints.
		f := 55 + 260*sp
		p := adv(&m.phase1, f)
		growl := 0.0
		for h := 1.0; h <= 5; h++ { // a buzzy, saw-like tone
			growl += math.Sin(p*h) / h
		}
		growl *= 0.07 * (0.3 + 0.7*sp)
		m.jointPhase += sp * 14 / 15 * dt // a joint every 15 m at up to 14 m/s
		joint := 0.0
		if frac := m.jointPhase - math.Floor(m.jointPhase); sp > 0.05 && frac < 0.03 {
			joint = 0.5 * (m.rand()*0.5 + 0.5) * math.Exp(-frac*120)
		}
		hum := 0.1 * math.Sin(adv(&m.phaseHum, 50))
		return growl + hum + 1.3*rumble + joint
	case MotorQuiet:
		return 0.6*rumble + 0.03*math.Sin(adv(&m.phaseHum, 100))
	}
	// Modern: a smooth inverter whine over the traction hum.
	fWhine := 220 + 900*sp
	whine := (0.10*math.Sin(adv(&m.phase1, fWhine)) + 0.04*math.Sin(adv(&m.phase2, fWhine*1.5))) * (0.25 + 0.75*sp)
	hum := 0.12 * math.Sin(adv(&m.phaseHum, 100))
	return whine + hum + rumble
}

// DoorStyle is the sound of a tram's doors.
type DoorStyle int

const (
	DoorPneumatic DoorStyle = iota // air doors: a hiss and a thunk; two beeps before closing
	DoorElectric                   // electric plug doors: a motor's whir and a click; three beeps before closing
	DoorClassic                    // old folding doors: a rattle and a clunk; a bell before closing
	DoorSoft                       // two soft notes, up to open and down to close
	DoorStyles
)

// doorSoundLength is how long a door sound lasts, s (closing includes the
// warning, matching the simulation's DoorCloseTime).
const doorSoundLength = 3.0

type doorVoice struct {
	t         float64
	open      bool
	gain, pan float64
	style     DoorStyle
	lp        float64 // the hiss's filter
}

// doorSample is one sample of a door sound. Opening: the doors move from
// 0 to 2 s. Closing: a warning in the first second, the doors from 1 to
// 3 s.
func (m *Mixer) doorSample(d *doorVoice) float64 {
	t := d.t
	env := func(a, b float64) float64 { // 0 outside [a,b], soft edges
		if t < a || t > b {
			return 0
		}
		return math.Min(1, math.Min((t-a)/0.03, (b-t)/0.08))
	}
	tone := func(f, a, b float64) float64 { return env(a, b) * math.Sin(2*math.Pi*f*t) }
	thunk := func(at float64) float64 { // the doors meeting their stops
		u := t - at
		if u < 0 || u > 0.25 {
			return 0
		}
		return math.Sin(2*math.Pi*90*u) * math.Exp(-u*28) * 0.9
	}
	move0, move1 := 0.1, 1.9 // when the doors move
	if !d.open {
		move0, move1 = 1.1, 2.9
	}
	switch d.style {
	case DoorElectric:
		s := 0.0
		if !d.open {
			for _, at := range []float64{0, 0.3, 0.6} {
				s += 0.35 * tone(2400, at, at+0.16)
			}
		}
		if e := env(move0, move1); e > 0 {
			u := (t - move0) / (move1 - move0)
			f := 140 + 160*math.Sin(math.Pi*u) // the motor speeds up and slows
			s += 0.18 * e * math.Sin(2*math.Pi*f*t+3*math.Sin(2*math.Pi*f*0.5*t))
		}
		return s + 0.5*thunk(move1) // a click as they lock
	case DoorClassic:
		s := 0.0
		if !d.open {
			u := t
			if u < 1 {
				s += 0.4 * math.Sin(2*math.Pi*1500*u) * math.Exp(-u*4)
			}
		}
		if e := env(move0, move1); e > 0 {
			s += 0.25 * e * m.rand() * (0.5 + 0.5*math.Sin(2*math.Pi*11*t)) // rattle
		}
		return s + thunk(move1)
	case DoorSoft:
		f1, f2 := 523.0, 659.0 // C5 E5: up to open
		if !d.open {
			f1, f2 = 659, 523 // down to close
		}
		note := func(f, at float64) float64 {
			u := t - at
			if u < 0 || u > 1.2 {
				return 0
			}
			return math.Min(1, u/0.01) * math.Exp(-u*3) * math.Sin(2*math.Pi*f*u)
		}
		return 0.4 * (note(f1, 0) + note(f2, 0.3))
	}
	// Pneumatic.
	s := 0.0
	if !d.open {
		for _, at := range []float64{0, 0.45} {
			s += 0.35 * tone(1800, at, at+0.2)
		}
	}
	if e := env(move0, move0+0.7); e > 0 { // air released
		d.lp += (m.rand() - d.lp) * 0.35
		s += 0.45 * e * d.lp * math.Exp(-(t-move0)*2.5)
	}
	return s + thunk(move1)
}
