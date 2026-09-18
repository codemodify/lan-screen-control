//go:build !darwin

package input

import (
	"log/slog"
)

// New returns a logging stub. Real injection is implemented for macOS only.
func New() Injector {
	slog.Info("input injection is a no-op on this OS (macOS host required)")
	return stubInjector{}
}
