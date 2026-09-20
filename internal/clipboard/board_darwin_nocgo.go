//go:build darwin && !cgo

package clipboard

import "log/slog"

type stubBoard struct{}

// New returns a no-op pasteboard. Rebuild on the Mac with CGO enabled (the
// default when running `go build` locally) for real NSPasteboard access.
func New() Board {
	slog.Warn("clipboard sync disabled: this darwin binary was built with CGO_ENABLED=0; rebuild on the Mac with CGO")
	return stubBoard{}
}

func (stubBoard) WriteText(string) error { return nil }

func (stubBoard) ReadText() (string, bool, error) { return "", false, nil }

func (stubBoard) ChangeCount() int { return 0 }
