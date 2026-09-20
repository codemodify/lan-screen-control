//go:build !darwin

package clipboard

import "log/slog"

type stubBoard struct{}

// New returns a no-op pasteboard. Real NSPasteboard support is macOS-only.
func New() Board {
	slog.Info("clipboard sync is a no-op on this OS (macOS host required)")
	return stubBoard{}
}

func (stubBoard) WriteText(string) error { return nil }

func (stubBoard) ReadText() (string, bool, error) { return "", false, nil }

func (stubBoard) ChangeCount() int { return 0 }
