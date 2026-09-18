//go:build darwin && cgo

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <stdint.h>

static CGEventSourceRef src = NULL;
static CGEventFlags flags = 0;
static int leftDown = 0;
static int rightDown = 0;
static int otherDown = 0;

static void ensureSource(void) {
	if (src == NULL) {
		src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	}
}

void LSCSetFlags(uint64_t f) {
	flags = (CGEventFlags)f;
}

void LSCMove(double x, double y) {
	ensureSource();
	CGPoint p = CGPointMake(x, y);
	CGEventType type = kCGEventMouseMoved;
	CGMouseButton btn = kCGMouseButtonLeft;
	if (leftDown) {
		type = kCGEventLeftMouseDragged;
		btn = kCGMouseButtonLeft;
	} else if (rightDown) {
		type = kCGEventRightMouseDragged;
		btn = kCGMouseButtonRight;
	} else if (otherDown) {
		type = kCGEventOtherMouseDragged;
		btn = kCGMouseButtonCenter;
	}
	CGWarpMouseCursorPosition(p);
	CGEventRef e = CGEventCreateMouseEvent(src, type, p, btn);
	if (e) {
		CGEventSetFlags(e, flags);
		CGEventPost(kCGHIDEventTap, e);
		CFRelease(e);
	}
}

void LSCButton(double x, double y, int button, int down) {
	ensureSource();
	CGPoint p = CGPointMake(x, y);
	CGEventType type;
	CGMouseButton btn;
	switch (button) {
	case 2:
		btn = kCGMouseButtonRight;
		type = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
		rightDown = down;
		break;
	case 1:
		btn = kCGMouseButtonCenter;
		type = down ? kCGEventOtherMouseDown : kCGEventOtherMouseUp;
		otherDown = down;
		break;
	default:
		btn = kCGMouseButtonLeft;
		type = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
		leftDown = down;
		break;
	}
	CGWarpMouseCursorPosition(p);
	CGEventRef e = CGEventCreateMouseEvent(src, type, p, btn);
	if (e) {
		CGEventSetFlags(e, flags);
		CGEventSetIntegerValueField(e, kCGMouseEventClickState, 1);
		CGEventPost(kCGHIDEventTap, e);
		CFRelease(e);
	}
}

void LSCScroll(double x, double y, int32_t dx, int32_t dy) {
	ensureSource();
	CGPoint p = CGPointMake(x, y);
	CGWarpMouseCursorPosition(p);
	CGEventRef e = CGEventCreateScrollWheelEvent(src, kCGScrollEventUnitPixel, 2, dy, dx);
	if (e) {
		CGEventSetFlags(e, flags);
		CGEventPost(kCGHIDEventTap, e);
		CFRelease(e);
	}
}

void LSCKey(uint16_t keyCode, int down) {
	ensureSource();
	CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)keyCode, down ? 1 : 0);
	if (e) {
		CGEventSetFlags(e, flags);
		CGEventPost(kCGHIDEventTap, e);
		CFRelease(e);
	}
}

int LSCDisplayWidth(void) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	return (int)b.size.width;
}

int LSCDisplayHeight(void) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	return (int)b.size.height;
}
*/
import "C"

import (
	"log/slog"

	"github.com/codemodify/lan-screen-control/internal/protocol"
)

// New returns a CoreGraphics injector. Accessibility permission is required.
func New() Injector {
	return &darwinInjector{flags: 0}
}

type darwinInjector struct {
	flags uint64
}

func (d *darwinInjector) DisplaySize() (int, int) {
	w := int(C.LSCDisplayWidth())
	h := int(C.LSCDisplayHeight())
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	return w, h
}

func (d *darwinInjector) Apply(ev protocol.Event) {
	w, h := d.DisplaySize()
	x, y := MapPoint(ev.X, ev.Y, w, h)

	switch ev.Type {
	case protocol.TypeMouseMove:
		C.LSCMove(C.double(x), C.double(y))
	case protocol.TypeMouseDown:
		C.LSCButton(C.double(x), C.double(y), C.int(ev.Button), 1)
	case protocol.TypeMouseUp:
		C.LSCButton(C.double(x), C.double(y), C.int(ev.Button), 0)
	case protocol.TypeWheel:
		dx, dy := wheelSteps(ev.DX, ev.DY)
		C.LSCScroll(C.double(x), C.double(y), C.int32_t(dx), C.int32_t(dy))
	case protocol.TypeKeyDown, protocol.TypeKeyUp:
		if ev.Repeat {
			return
		}
		code, ok := keyCode(ev.Key)
		if !ok {
			slog.Debug("unmapped key", "code", ev.Key)
			return
		}
		down := ev.Type == protocol.TypeKeyDown
		if bit := modifierFlag(ev.Key); bit != 0 {
			if down {
				d.flags |= bit
			} else {
				d.flags &^= bit
			}
			C.LSCSetFlags(C.uint64_t(d.flags))
		}
		var cDown C.int
		if down {
			cDown = 1
		}
		C.LSCKey(C.uint16_t(code), cDown)
	}
}

// wheelSteps converts browser wheel deltas (typically ~100 per notch) into
// Quartz pixel-unit scroll ticks. Sign is flipped so "scroll down" moves down.
func wheelSteps(dx, dy float64) (int32, int32) {
	return scaleWheel(-dx), scaleWheel(-dy)
}

func scaleWheel(delta float64) int32 {
	if delta == 0 {
		return 0
	}
	v := int32(delta)
	if v == 0 {
		if delta > 0 {
			return 1
		}
		return -1
	}
	return v
}
