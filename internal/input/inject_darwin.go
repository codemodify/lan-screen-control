//go:build darwin && cgo

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework IOKit
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <stdint.h>

#ifndef kIOMainPortDefault
#define kIOMainPortDefault kIOMasterPortDefault
#endif

#include <stdlib.h>

static CGEventSourceRef src = NULL;
static CGEventFlags flags = 0;
static int leftDown = 0;
static int rightDown = 0;
static int otherDown = 0;

// Accessibility must be granted to the lan-screen-control binary (LaunchAgent).
// HID tap is preferred; session tap is a lock-screen fallback for loginwindow.

static int consoleLocked(void) {
	CFDictionaryRef dict = CGSessionCopyCurrentDictionary();
	if (dict) {
		const void *val = CFDictionaryGetValue(dict, CFSTR("CGSSessionScreenIsLocked"));
		int locked = (val && CFGetTypeID(val) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)val));
		CFRelease(dict);
		if (locked) {
			return 1;
		}
	}
	io_service_t hid = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("IOHIDSystem"));
	if (!hid) {
		return 0;
	}
	CFTypeRef prop = IORegistryEntryCreateCFProperty(hid, CFSTR("IOConsoleLocked"), kCFAllocatorDefault, 0);
	IOObjectRelease(hid);
	if (!prop) {
		return 0;
	}
	int locked = 0;
	if (CFGetTypeID(prop) == CFBooleanGetTypeID()) {
		locked = CFBooleanGetValue((CFBooleanRef)prop);
	} else if (CFGetTypeID(prop) == CFNumberGetTypeID()) {
		int n = 0;
		CFNumberGetValue((CFNumberRef)prop, kCFNumberIntType, &n);
		locked = n != 0;
	}
	CFRelease(prop);
	return locked;
}

static void ensureSource(void) {
	if (src == NULL) {
		src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
		if (src) {
			CGEventSourceSetLocalEventsSuppressionInterval(src, 0);
		}
	}
}

// postEvent prefers the HID tap (reaches loginwindow when trusted). If the
// console is locked, also post at the session tap as a fallback.
static void postEvent(CGEventRef e) {
	if (!e) {
		return;
	}
	CGEventPost(kCGHIDEventTap, e);
	if (consoleLocked()) {
		CGEventPost(kCGSessionEventTap, e);
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
		postEvent(e);
		CFRelease(e);
	}
}

// button is a CGMouseButton (left 0, right 1, center 2), not a DOM button
// (left 0, middle 1, right 2). Callers convert with quartzButton first.
// The button-number field is set explicitly: CGEventCreateMouseEvent ignores
// its button argument except for "other" mouse events, and an unset number
// is easy to misread as button 1 (right).
void LSCButton(double x, double y, int button, int down) {
	ensureSource();
	CGPoint p = CGPointMake(x, y);
	CGEventType type;
	CGMouseButton btn = (CGMouseButton)button;
	switch (button) {
	case kCGMouseButtonRight:
		type = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
		rightDown = down;
		break;
	case kCGMouseButtonCenter:
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
		// flags must already match the browser's modifier snapshot.
		// kCGEventFlagMaskControl on a left click is a macOS context click.
		CGEventSetFlags(e, flags);
		CGEventSetIntegerValueField(e, kCGMouseEventClickState, 1);
		CGEventSetIntegerValueField(e, kCGMouseEventButtonNumber, (int64_t)btn);
		postEvent(e);
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
		postEvent(e);
		CFRelease(e);
	}
}

static void attachUnicode(CGEventRef e, const char *utf8) {
	if (!e || !utf8 || !utf8[0]) {
		return;
	}
	CFStringRef s = CFStringCreateWithCString(kCFAllocatorDefault, utf8, kCFStringEncodingUTF8);
	if (!s) {
		return;
	}
	CFIndex n = CFStringGetLength(s);
	if (n > 0) {
		if (n > 16) {
			n = 16;
		}
		UniChar buf[16];
		CFStringGetCharacters(s, CFRangeMake(0, n), buf);
		CGEventKeyboardSetUnicodeString(e, (UniCharCount)n, buf);
	}
	CFRelease(s);
}

// postKeyEvent always hits the HID tap. When allTaps is set (locked console),
// also post session + annotated session so Secure Input / loginwindow sees it.
static void postKeyEvent(CGEventRef e, int allTaps) {
	if (!e) {
		return;
	}
	CGEventPost(kCGHIDEventTap, e);
	if (allTaps) {
		CGEventPost(kCGSessionEventTap, e);
		CGEventPost(kCGAnnotatedSessionEventTap, e);
	}
}

// LSCKeyUnicode posts a key event, optionally with a unicode string
// (CGEventKeyboardSetUnicodeString). Secure Input on the lock screen
// ignores virtual keycodes but accepts the unicode payload.
void LSCKeyUnicode(uint16_t keyCode, const char *utf8, int down, int allTaps) {
	ensureSource();
	CGEventRef e = CGEventCreateKeyboardEvent(src, (CGKeyCode)keyCode, down ? 1 : 0);
	if (!e) {
		return;
	}
	CGEventSetFlags(e, flags);
	CGEventSetIntegerValueField(e, kCGKeyboardEventAutorepeat, 0);
	attachUnicode(e, utf8);
	postKeyEvent(e, allTaps);
	CFRelease(e);
}

void LSCKey(uint16_t keyCode, int down) {
	LSCKeyUnicode(keyCode, NULL, down, 0);
}

int LSCDisplayWidth(void) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	return (int)b.size.width;
}

int LSCDisplayHeight(void) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	return (int)b.size.height;
}

void LSCPasswordFieldPoint(double *x, double *y) {
	CGRect b = CGDisplayBounds(CGMainDisplayID());
	if (x) {
		*x = b.origin.x + b.size.width * 0.50;
	}
	if (y) {
		*y = b.origin.y + b.size.height * 0.58;
	}
}

int LSCConsoleLocked(void) {
	return consoleLocked();
}
*/
import "C"

import (
	"log/slog"
	"sync"
	"time"
	"unsafe"

	"github.com/codemodify/lan-screen-control/internal/protocol"
)

// New returns a CoreGraphics injector. Accessibility must be granted to the
// lan-screen-control binary (or to Terminal if you launch from a shell).
// A LaunchAgent does not inherit Terminal's TCC rights.
func New() Injector {
	return NewWithFrame(0, 0)
}

// NewWithFrame is New plus the encoded canvas size used when both -width and
// -height are set (ffmpeg letterbox). Zero frame size means native aspect.
func NewWithFrame(frameW, frameH int) Injector {
	return &darwinInjector{frameW: evenDim(frameW), frameH: evenDim(frameH)}
}

type darwinInjector struct {
	flags          uint64
	frameW, frameH int

	prepMu        sync.Mutex
	lastLockClick time.Time
	unicodeLog    sync.Once
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
	switch ev.Type {
	case protocol.TypeMouseMove, protocol.TypeMouseDown, protocol.TypeMouseUp, protocol.TypeWheel:
		// Pointer events carry the keys actually held. A latched Control
		// from a dropped keyup would make every later left click a context click.
		d.syncPointerMods(ev.Mods)
	}

	w, h := d.DisplaySize()
	x, y := MapPointFromFrame(ev.X, ev.Y, w, h, d.frameW, d.frameH)

	switch ev.Type {
	case protocol.TypeMouseMove:
		C.LSCMove(C.double(x), C.double(y))
	case protocol.TypeMouseDown:
		C.LSCButton(C.double(x), C.double(y), C.int(quartzButton(ev.Button)), 1)
	case protocol.TypeMouseUp:
		C.LSCButton(C.double(x), C.double(y), C.int(quartzButton(ev.Button)), 0)
	case protocol.TypeWheel:
		dx, dy := wheelSteps(ev.DX, ev.DY)
		C.LSCScroll(C.double(x), C.double(y), C.int32_t(dx), C.int32_t(dy))
	case protocol.TypeKeyDown, protocol.TypeKeyUp:
		if ev.Repeat {
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
		locked := C.LSCConsoleLocked() != 0
		plan, ok := planKeyInject(locked, ev.Key, ev.Char)
		if !ok {
			slog.Debug("unmapped key", "code", ev.Key)
			return
		}
		if locked && plan.unicode && down {
			d.unicodeLog.Do(func() {
				slog.Info("console locked: injecting printable keys as unicode (Secure Input ignores virtual keycodes)")
			})
		}
		char := ""
		if plan.unicode {
			char = plan.char
		}
		d.postKey(plan.code, char, down, plan.allTaps)
	}
}

// syncPointerMods makes Shift/Control/Option/Command match the browser
// snapshot, and posts key-ups for anything that was latched but is no longer
// held. Intentional holds stay: if the browser still reports Control, the
// bit remains and a left click is a real Control-click.
func (d *darwinInjector) syncPointerMods(mods int) {
	newFlags, releases := reconcilePointerMods(d.flags, mods)
	if newFlags == d.flags && len(releases) == 0 {
		return
	}
	d.flags = newFlags
	C.LSCSetFlags(C.uint64_t(d.flags))
	if len(releases) == 0 {
		return
	}
	locked := C.LSCConsoleLocked() != 0
	for _, code := range releases {
		kc, ok := keyCode(code)
		if !ok {
			continue
		}
		d.postKey(kc, "", false, locked)
	}
}

func (d *darwinInjector) postKey(code uint16, char string, down, allTaps bool) {
	var cDown, cAll C.int
	if down {
		cDown = 1
	}
	if allTaps {
		cAll = 1
	}
	var cChar *C.char
	if char != "" {
		cChar = C.CString(char)
		defer C.free(unsafe.Pointer(cChar))
	}
	C.LSCKeyUnicode(C.uint16_t(code), cChar, cDown, cAll)
}

// PrepareForRemote left-clicks the lock-screen password field after the
// session-accept wake so remote keystrokes can land without a visible picture.
func (d *darwinInjector) PrepareForRemote() {
	d.prepMu.Lock()
	defer d.prepMu.Unlock()
	if C.LSCConsoleLocked() == 0 {
		return
	}
	if !d.lastLockClick.IsZero() && time.Since(d.lastLockClick) < 1500*time.Millisecond {
		return
	}
	// Short pause so loginwindow can present the password field after wake.
	time.Sleep(200 * time.Millisecond)
	if C.LSCConsoleLocked() == 0 {
		return
	}
	var x, y C.double
	C.LSCPasswordFieldPoint(&x, &y)
	C.LSCMove(x, y)
	time.Sleep(80 * time.Millisecond)
	C.LSCButton(x, y, 0, 1)
	time.Sleep(40 * time.Millisecond)
	C.LSCButton(x, y, 0, 0)
	d.lastLockClick = time.Now()
	slog.Info("console locked: clicked password-field region so remote typing can land")
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
