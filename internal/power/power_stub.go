//go:build !darwin || !cgo

package power

// WakeDisplay is a no-op off macOS / without CGO.
func WakeDisplay() {}

// PreventDisplaySleep returns an idempotent no-op release function.
func PreventDisplaySleep() (release func()) {
	return func() {}
}
