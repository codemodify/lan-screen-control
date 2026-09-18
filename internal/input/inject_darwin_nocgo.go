//go:build darwin && !cgo

package input

import (
	"log/slog"
)

// New returns a no-op injector. Rebuild on the Mac with CGO enabled (the
// default when running `go build` locally) for real Accessibility injection.
func New() Injector {
	slog.Warn("input injection disabled: this darwin binary was built with CGO_ENABLED=0; rebuild on the Mac with CGO")
	return stubInjector{}
}
