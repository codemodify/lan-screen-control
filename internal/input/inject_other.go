//go:build !darwin

package input

import (
	"log/slog"
)

// New returns a logging stub. Real injection is implemented for macOS only.
func New() Injector {
	return NewWithFrame(0, 0)
}

// NewWithFrame is New plus the encoded canvas size (padded mode).
func NewWithFrame(frameW, frameH int) Injector {
	slog.Info("input injection is a no-op on this OS (macOS host required)")
	return stubInjector{frameW: evenDim(frameW), frameH: evenDim(frameH)}
}
