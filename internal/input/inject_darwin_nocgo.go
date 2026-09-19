//go:build darwin && !cgo

package input

import (
	"log/slog"
)

// New returns a no-op injector. Rebuild on the Mac with CGO enabled (the
// default when running `go build` locally) for real Accessibility injection.
func New() Injector {
	return NewWithFrame(0, 0)
}

// NewWithFrame is New plus the encoded canvas size (padded mode).
func NewWithFrame(frameW, frameH int) Injector {
	slog.Warn("input injection disabled: this darwin binary was built with CGO_ENABLED=0; rebuild on the Mac with CGO")
	return stubInjector{frameW: evenDim(frameW), frameH: evenDim(frameH)}
}
