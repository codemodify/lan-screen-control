//go:build darwin && cgo

package power

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -framework CoreGraphics
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <stdint.h>

// LSCDeclareUserActivity tells the power manager a user is present so a
// dark display lights up. The short-lived assertion is released immediately;
// the idle timer has already been reset. Returns 1 on success.
int LSCDeclareUserActivity(void) {
	IOPMAssertionID id = 0;
	IOReturn r = IOPMAssertionDeclareUserActivity(
		CFSTR("lan-screen-control client connected"),
		kIOPMUserActiveLocal,
		&id);
	if (r == kIOReturnSuccess && id != 0) {
		IOPMAssertionRelease(id);
		return 1;
	}
	return r == kIOReturnSuccess ? 1 : 0;
}

// LSCNudgeMouse posts a 1px move and restore. That is HID activity and can
// wake a sleeping display from a LaunchAgent when Accessibility is granted.
void LSCNudgeMouse(void) {
	CGEventRef probe = CGEventCreate(NULL);
	if (!probe) {
		return;
	}
	CGPoint p = CGEventGetLocation(probe);
	CFRelease(probe);

	CGEventSourceRef src = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	if (!src) {
		return;
	}
	CGPoint nudged = CGPointMake(p.x + 1.0, p.y);
	CGEventRef e1 = CGEventCreateMouseEvent(src, kCGEventMouseMoved, nudged, kCGMouseButtonLeft);
	if (e1) {
		CGEventPost(kCGHIDEventTap, e1);
		CFRelease(e1);
	}
	CGEventRef e2 = CGEventCreateMouseEvent(src, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	if (e2) {
		CGEventPost(kCGHIDEventTap, e2);
		CFRelease(e2);
	}
	CFRelease(src);
}

// LSCCreateNoDisplaySleep holds PreventUserIdleDisplaySleep. Returns 0 on failure.
uint32_t LSCCreateNoDisplaySleep(void) {
	IOPMAssertionID id = 0;
	IOReturn r = IOPMAssertionCreateWithName(
		kIOPMAssertionTypeNoDisplaySleep,
		kIOPMAssertionLevelOn,
		CFSTR("lan-screen-control remote session"),
		&id);
	if (r != kIOReturnSuccess) {
		return 0;
	}
	return (uint32_t)id;
}

void LSCReleaseAssertion(uint32_t id) {
	if (id != 0) {
		IOPMAssertionRelease((IOPMAssertionID)id);
	}
}
*/
import "C"

import (
	"log/slog"
	"os/exec"
	"sync"
)

// WakeDisplay pulses user activity so a display that is dark from idle
// sleep lights up before capture starts. No-op if every strategy fails.
func WakeDisplay() {
	ok := C.LSCDeclareUserActivity() != 0
	C.LSCNudgeMouse()
	if ok {
		slog.Info("woke display for remote session")
		return
	}
	slog.Warn("IOPM user-activity assertion failed; trying caffeinate -u")
	caffeinateWake()
}

// PreventDisplaySleep holds PreventUserIdleDisplaySleep until the returned
// function is called. The release func is safe to call more than once.
func PreventDisplaySleep() (release func()) {
	id := C.LSCCreateNoDisplaySleep()
	var cmd *exec.Cmd
	if id != 0 {
		slog.Info("holding display awake for remote session")
	} else {
		slog.Warn("IOPM PreventUserIdleDisplaySleep failed; trying caffeinate -d")
		cmd = caffeinateHold()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if id != 0 {
				C.LSCReleaseAssertion(C.uint32_t(id))
			}
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
			if id != 0 || cmd != nil {
				slog.Info("released display-sleep prevention")
			}
		})
	}
}

func caffeinateWake() {
	cmd := exec.Command("caffeinate", "-u", "-t", "1")
	if err := cmd.Start(); err != nil {
		slog.Warn("caffeinate wake failed", "err", err)
		return
	}
	go func() { _ = cmd.Wait() }()
	slog.Info("woke display via caffeinate -u")
}

func caffeinateHold() *exec.Cmd {
	cmd := exec.Command("caffeinate", "-d")
	if err := cmd.Start(); err != nil {
		slog.Warn("caffeinate hold failed", "err", err)
		return nil
	}
	slog.Info("holding display awake via caffeinate -d")
	return cmd
}
