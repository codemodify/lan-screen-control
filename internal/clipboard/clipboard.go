// Package clipboard syncs plain-text UTF-8 between the browser and the Mac
// pasteboard while a WebRTC session is active.
//
// Real NSPasteboard read/write is compiled only for darwin+cgo. Other builds
// (Linux, CGO_ENABLED=0) get a no-op Board so the server still links.
package clipboard

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// MaxTextBytes is the v1 cap for a single clipboard payload (UTF-8).
const MaxTextBytes = 1 << 20

// DefaultPollInterval is how often the host pasteboard change-count is sampled.
const DefaultPollInterval = 250 * time.Millisecond

// ErrTooLarge is returned when a client payload exceeds MaxTextBytes.
var ErrTooLarge = errors.New("clipboard text exceeds 1MB limit")

// Board is the OS pasteboard.
type Board interface {
	WriteText(s string) error
	ReadText() (text string, ok bool, err error)
	ChangeCount() int
}

// Sync applies remote text, watches the host pasteboard, and suppresses echoes
// of our own writes (change-count + last text).
type Sync struct {
	board    Board
	interval time.Duration

	mu            sync.Mutex
	lastSelfCount int
	lastSelfText  string
	lastSeenCount int
}

// NewSync watches board at DefaultPollInterval.
func NewSync(board Board) *Sync {
	return NewSyncInterval(board, DefaultPollInterval)
}

// NewSyncInterval is NewSync with a custom poll period (tests).
func NewSyncInterval(board Board, interval time.Duration) *Sync {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	return &Sync{board: board, interval: interval}
}

// TooLarge reports whether s exceeds the v1 byte cap.
func TooLarge(s string) bool {
	return len(s) > MaxTextBytes
}

// ApplyRemote writes text that arrived from the browser onto the host
// pasteboard. The following change-count is ignored so we do not echo it back.
func (s *Sync) ApplyRemote(text string) error {
	if s == nil || s.board == nil {
		return nil
	}
	if TooLarge(text) {
		slog.Warn("dropping oversized clipboard from client", "bytes", len(text), "limit", MaxTextBytes)
		return ErrTooLarge
	}
	if err := s.board.WriteText(text); err != nil {
		return err
	}
	s.mu.Lock()
	s.lastSelfCount = s.board.ChangeCount()
	s.lastSelfText = text
	s.lastSeenCount = s.lastSelfCount
	s.mu.Unlock()
	return nil
}

// Current returns the host pasteboard text if it is within the size limit.
func (s *Sync) Current() (string, bool) {
	if s == nil || s.board == nil {
		return "", false
	}
	text, ok, err := s.board.ReadText()
	if err != nil || !ok || text == "" {
		return "", false
	}
	if TooLarge(text) {
		slog.Warn("dropping oversized host clipboard", "bytes", len(text), "limit", MaxTextBytes)
		return "", false
	}
	return text, true
}

// Watch polls the pasteboard until ctx is cancelled and calls send for each
// external text change. The first tick also pushes the current text (if any)
// so a newly connected browser can paste locally right away.
func (s *Sync) Watch(ctx context.Context, send func(string)) {
	if s == nil || s.board == nil || send == nil {
		return
	}
	if text, ok := s.readChange(true); ok {
		send(text)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if text, ok := s.readChange(false); ok {
				send(text)
			}
		}
	}
}

func (s *Sync) readChange(force bool) (string, bool) {
	count := s.board.ChangeCount()

	s.mu.Lock()
	seen := s.lastSeenCount
	selfCount := s.lastSelfCount
	selfText := s.lastSelfText
	if !force && count == seen {
		s.mu.Unlock()
		return "", false
	}
	s.lastSeenCount = count
	s.mu.Unlock()

	if !force && count == selfCount {
		return "", false
	}

	text, ok, err := s.board.ReadText()
	if err != nil || !ok || text == "" {
		return "", false
	}
	if TooLarge(text) {
		slog.Warn("dropping oversized host clipboard", "bytes", len(text), "limit", MaxTextBytes)
		return "", false
	}
	if !force && text == selfText {
		return "", false
	}

	s.mu.Lock()
	s.lastSelfText = text
	s.mu.Unlock()
	return text, true
}
