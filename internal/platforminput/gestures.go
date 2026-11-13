package platforminput

import (
	"math"
	"sync"

	ebiten "github.com/hajimehoshi/ebiten/v2"
)

var (
	pinchMu            sync.Mutex
	pinchMagnification float64
	trackpadScrollY    float64
	mouseWheelScrollY  float64
	sawTrackpadScroll  bool
	sawMouseWheel      bool
)

func RecordPinch(delta float64) {
	if delta == 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return
	}
	pinchMu.Lock()
	pinchMagnification += delta
	pinchMu.Unlock()
}

func RecordScroll(deltaY float64, isTrackpad bool) {
	if deltaY == 0 || math.IsNaN(deltaY) || math.IsInf(deltaY, 0) {
		return
	}
	pinchMu.Lock()
	if isTrackpad {
		trackpadScrollY += deltaY
		sawTrackpadScroll = true
	} else {
		mouseWheelScrollY += deltaY
		sawMouseWheel = true
	}
	pinchMu.Unlock()
}

func TakePinchDelta() float64 {
	ensureTrackpadPinchHook()
	pinchMu.Lock()
	delta := pinchMagnification
	pinchMagnification = 0
	pinchMu.Unlock()
	return delta
}

func ResetScrollAndPinch() {
	ensureTrackpadPinchHook()
	pinchMu.Lock()
	pinchMagnification = 0
	trackpadScrollY = 0
	mouseWheelScrollY = 0
	sawTrackpadScroll = false
	sawMouseWheel = false
	pinchMu.Unlock()
}

// TakeMouseWheelY returns vertical scroll wheel input from a physical mouse wheel
// while ignoring macOS trackpad two-finger vertical scrolling.
func TakeMouseWheelY() float64 {
	ensureTrackpadPinchHook()
	_, wheelY := ebiten.Wheel()

	pinchMu.Lock()
	tpSaw := sawTrackpadScroll
	mwSaw := sawMouseWheel
	mwY := mouseWheelScrollY
	trackpadScrollY = 0
	mouseWheelScrollY = 0
	sawTrackpadScroll = false
	sawMouseWheel = false
	pinchMu.Unlock()

	if tpSaw && !mwSaw {
		// Scroll event came from macOS trackpad two-finger movement; ignore for map zoom.
		return 0
	}
	if mwSaw {
		if wheelY != 0 && !tpSaw {
			return wheelY
		}
		return mwY
	}
	return wheelY
}
