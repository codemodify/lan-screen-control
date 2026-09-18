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
type stubInjector struct {
	frameW, frameH int
}

func (stubInjector) DisplaySize() (int, int) { return 1920, 1080 }

func (s stubInjector) Apply(ev protocol.Event) {
	// Intentionally empty: this host cannot inject into macOS.
	_ = ev
	_, _ = s.frameW, s.frameH
}

func evenDim(n int) int {
	if n <= 0 {
		return 0
	}
	return n - n%2
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// MapPoint converts a normalized [0,1] video coordinate to display points.
func MapPoint(x, y float64, width, height int) (px, py float64) {
	return clamp01(x) * float64(width), clamp01(y) * float64(height)
}

// ContainRect fits an inner size into an outer box the same way CSS
// object-fit:contain / ffmpeg pad+decrease does (centered letterbox).
func ContainRect(outerW, outerH, innerW, innerH float64) (x, y, w, h float64) {
	if outerW <= 0 || outerH <= 0 || innerW <= 0 || innerH <= 0 {
		return 0, 0, outerW, outerH
	}
	scale := outerW / innerW
	if outerH/innerH < scale {
		scale = outerH / innerH
	}
	w = innerW * scale
	h = innerH * scale
	return (outerW - w) / 2, (outerH - h) / 2, w, h
}

// MapPointFromFrame maps a click that is normalized over the encoded
// video frame (including letterbox pad) onto the host display.
// If frameW/frameH are 0, the frame is treated as matching the display.
func MapPointFromFrame(x, y float64, displayW, displayH, frameW, frameH int) (px, py float64) {
	x, y = clamp01(x), clamp01(y)
	if displayW <= 0 || displayH <= 0 {
		return 0, 0
	}
	if frameW <= 0 || frameH <= 0 {
		return MapPoint(x, y, displayW, displayH)
	}
	ox, oy, cw, ch := ContainRect(float64(frameW), float64(frameH), float64(displayW), float64(displayH))
	if cw <= 0 || ch <= 0 {
		return MapPoint(x, y, displayW, displayH)
	}
	fx := x * float64(frameW)
	fy := y * float64(frameH)
	return MapPoint((fx-ox)/cw, (fy-oy)/ch, displayW, displayH)
}
