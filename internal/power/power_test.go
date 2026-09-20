package power

import "testing"

func TestWakeDisplayDoesNotPanic(t *testing.T) {
	WakeDisplay()
}

func TestPreventDisplaySleepReleaseIdempotent(t *testing.T) {
	release := PreventDisplaySleep()
	if release == nil {
		t.Fatal("PreventDisplaySleep must return a release func")
	}
	release()
	release()
}
