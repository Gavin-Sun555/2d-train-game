package sound

import (
	"encoding/binary"
	"math"
	"testing"
)

func rms(m *Mixer, seconds float64) float64 {
	buf := make([]byte, int(seconds*SampleRate)*8)
	m.Read(buf)
	sum := 0.0
	for i := 0; i < len(buf); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))
		if math.IsNaN(v) || v < -1 || v > 1 {
			panic("sample out of range")
		}
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(buf)/4))
}

func TestMixer(t *testing.T) {
	m := NewMixer()
	if rms(m, 0.2) != 0 {
		t.Fatal("not silent at start")
	}
	for st := MotorStyle(0); st < MotorStyles; st++ {
		m.SetStyles(BellClang, st)
		m.SetMotor(Motor{Loud: 1, Speed: 0.8})
		if rms(m, 1) == 0 {
			t.Fatalf("motor style %d is silent", st)
		}
	}
	m.SetMotor(Motor{})
	rms(m, 2)
}

// A gong keeps striking for as long as the tram is in the crossing, in
// every style, then stops.
func TestGongRingsWhileInCrossing(t *testing.T) {
	for st := BellStyle(0); st < BellStyles; st++ {
		m := NewMixer()
		m.SetStyles(st, MotorModern)
		m.SetRinging(map[int]Ring{7: {Gain: 1}})
		for s := 0; s < 6; s++ { // six seconds in the crossing
			if v := rms(m, 1); v < 0.005 {
				t.Fatalf("style %d: gong quiet (%.4f) after %d s in the crossing", st, v, s)
			}
		}
		m.SetRinging(nil)
		rms(m, ringTail+0.2)
		if m.Ringing() != 0 || rms(m, 0.5) != 0 {
			t.Fatalf("style %d: gong rings on after the tram left", st)
		}
	}
	m := NewMixer()
	rings := map[int]Ring{}
	for i := 0; i < 10; i++ {
		rings[i] = Ring{Gain: float64(i+1) / 10}
	}
	m.SetRinging(rings)
	if m.Ringing() != MaxBells {
		t.Errorf("%d gongs at once", m.Ringing())
	}
	m.SetVolumes(Volumes{})
	m.Chime()
	if v := rms(m, 1); v != 0 {
		t.Errorf("sound at zero volume: %g", v)
	}
}

func TestVoicesAndText(t *testing.T) {
	list := "Albert              en_US    # Hello!\nEddy (Chinese (China mainland)) zh_CN    # 你好\nTingting            zh_CN    # 你好\nSamantha            en_US    # Hello\n"
	if v := pickVoice(list, "zh_CN", []string{"Tingting", "Eddy"}); v != "Tingting" {
		t.Errorf("zh voice %q", v)
	}
	if v := pickVoice(list, "en_US", []string{"Flo", "Samantha"}); v != "Samantha" {
		t.Errorf("en voice %q", v)
	}
	if v := pickVoice(list, "zh_CN", []string{"Nobody"}); v != "Eddy (Chinese (China mainland))" {
		t.Errorf("fallback voice %q", v)
	}
	if s := SpeechText("Market [[slnc 900]] St", 2); s != "[[volm 1.00]]Market ［［slnc 900］］ St" {
		t.Errorf("speech text %q", s)
	}
}

// Each door style sounds, opening and closing, and dies away.
func TestDoorSounds(t *testing.T) {
	for st := DoorStyle(0); st < DoorStyles; st++ {
		for _, open := range []bool{true, false} {
			m := NewMixer()
			m.SetDoorStyle(st)
			m.Door(open, 1, 0)
			if v := rms(m, doorSoundLength); v < 0.003 {
				t.Errorf("style %d open=%v: too quiet (%.4f)", st, open, v)
			}
			if v := rms(m, 0.5); v != 0 {
				t.Errorf("style %d open=%v: still sounding", st, open)
			}
		}
	}
	m := NewMixer()
	m.SetVolumes(Volumes{Motor: 1, Bell: 1, Announce: 1})
	m.Door(true, 1, 0)
	if v := rms(m, 1); v != 0 {
		t.Errorf("door sound at zero volume")
	}
}
