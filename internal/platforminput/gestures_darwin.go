//go:build darwin && !ios

package platforminput

import (
	"math"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	pinchHookOnce                sync.Once
	selMagnification             objc.SEL
	selScrollingDeltaY           objc.SEL
	selHasPreciseScrollingDeltas objc.SEL
	selPhase                     objc.SEL
	selMomentumPhase             objc.SEL
	classReplaceMethod           func(cls objc.Class, name objc.SEL, imp objc.IMP, types string) objc.IMP
	origScrollWheel              func(self objc.ID, cmd objc.SEL, event objc.ID)
)

func ensureTrackpadPinchHook() {
	cls := objc.GetClass("GLFWContentView")
	if cls == 0 {
		return
	}
	pinchHookOnce.Do(func() {
		selMagnification = objc.RegisterName("magnification")
		selMagnifyWithEvent := objc.RegisterName("magnifyWithEvent:")
		pinchIMP := objc.NewIMP(func(_ objc.ID, _ objc.SEL, event objc.ID) {
			if event == 0 {
				return
			}
			mag := objc.Send[float64](event, selMagnification)
			if mag != 0 && !math.IsNaN(mag) && !math.IsInf(mag, 0) {
				RecordPinch(mag)
			}
		})
		cls.AddMethod(selMagnifyWithEvent, pinchIMP, "v@:@")

		libObjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&classReplaceMethod, libObjc, "class_replaceMethod")

		selScrollingDeltaY = objc.RegisterName("scrollingDeltaY")
		selHasPreciseScrollingDeltas = objc.RegisterName("hasPreciseScrollingDeltas")
		selPhase = objc.RegisterName("phase")
		selMomentumPhase = objc.RegisterName("momentumPhase")
		selScrollWheel := objc.RegisterName("scrollWheel:")

		scrollIMP := objc.NewIMP(func(self objc.ID, cmd objc.SEL, event objc.ID) {
			if event != 0 {
				deltaY := objc.Send[float64](event, selScrollingDeltaY)
				hasPrecise := objc.Send[bool](event, selHasPreciseScrollingDeltas)
				phase := objc.Send[uintptr](event, selPhase)
				momentumPhase := objc.Send[uintptr](event, selMomentumPhase)
				isTrackpad := hasPrecise || phase != 0 || momentumPhase != 0
				if hasPrecise {
					deltaY *= 0.1
				}
				RecordScroll(deltaY, isTrackpad)
			}
			if origScrollWheel != nil {
				origScrollWheel(self, cmd, event)
			}
		})
		prevIMP := classReplaceMethod(cls, selScrollWheel, scrollIMP, "v@:@")
		if prevIMP != 0 {
			purego.RegisterFunc(&origScrollWheel, uintptr(prevIMP))
		}
	})
}
