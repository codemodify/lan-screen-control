// Package input injects remote pointer and keyboard events into the host OS.
// The real implementation is macOS-only (CoreGraphics, requires Accessibility).
package input

import "github.com/codemodify/lan-screen-control/internal/protocol"

// Injector applies remote input to the local machine.
type Injector interface {
	Apply(ev protocol.Event)
	DisplaySize() (width, height int)
}

// stubInjector is used on non-macOS builds and on darwin binaries compiled
// without CGO. It keeps the Apply/DisplaySize contract so signaling still works.
type stubInjector struct{}

func (stubInjector) DisplaySize() (int, int) { return 1920, 1080 }

func (stubInjector) Apply(ev protocol.Event) {
	// Intentionally empty: this host cannot inject into macOS.
	_ = ev
}

// MapPoint converts a normalized [0,1] video coordinate to display points.
func MapPoint(x, y float64, width, height int) (px, py float64) {
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	if y < 0 {
		y = 0
	}
	if y > 1 {
		y = 1
	}
	return x * float64(width), y * float64(height)
}
