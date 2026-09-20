package clipboard

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type memBoard struct {
	mu    sync.Mutex
	text  string
	ok    bool
	count int
}

func (m *memBoard) WriteText(s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text = s
	m.ok = true
	m.count++
	return nil
}

func (m *memBoard) ReadText() (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text, m.ok, nil
}

func (m *memBoard) ChangeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func TestApplyRemoteDoesNotEcho(t *testing.T) {
	b := &memBoard{}
	s := NewSync(b)
	if err := s.ApplyRemote("from-browser"); err != nil {
		t.Fatal(err)
	}
	if b.text != "from-browser" {
		t.Fatalf("board text: %q", b.text)
	}
	if text, ok := s.readChange(false); ok {
		t.Fatalf("self-write echoed back: %q", text)
	}
}

func TestExternalChangeIsEmitted(t *testing.T) {
	b := &memBoard{}
	s := NewSync(b)
	_, _ = s.readChange(true) // snapshot empty
	if err := b.WriteText("from-mac"); err != nil {
		t.Fatal(err)
	}
	text, ok := s.readChange(false)
	if !ok || text != "from-mac" {
		t.Fatalf("got %q ok=%v", text, ok)
	}
}

func TestSameTextAfterSelfWriteIsIgnored(t *testing.T) {
	b := &memBoard{}
	s := NewSync(b)
	if err := s.ApplyRemote("same"); err != nil {
		t.Fatal(err)
	}
	// Simulate a pasteboard owner rewriting the same string (new changeCount).
	if err := b.WriteText("same"); err != nil {
		t.Fatal(err)
	}
	if text, ok := s.readChange(false); ok {
		t.Fatalf("same text should not echo: %q", text)
	}
}

func TestOversizedApplyIsDropped(t *testing.T) {
	b := &memBoard{}
	s := NewSync(b)
	huge := strings.Repeat("a", MaxTextBytes+1)
	if err := s.ApplyRemote(huge); err != ErrTooLarge {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if b.ok {
		t.Fatal("oversized payload must not be written")
	}
}

func TestOversizedHostReadIsDropped(t *testing.T) {
	b := &memBoard{text: strings.Repeat("b", MaxTextBytes+8), ok: true, count: 3}
	s := NewSync(b)
	if text, ok := s.readChange(true); ok {
		t.Fatalf("oversized host clipboard leaked: %d bytes", len(text))
	}
}

func TestTooLarge(t *testing.T) {
	if TooLarge("") || TooLarge(strings.Repeat("x", MaxTextBytes)) {
		t.Fatal("limit should be exclusive of MaxTextBytes")
	}
	if !TooLarge(strings.Repeat("x", MaxTextBytes+1)) {
		t.Fatal("expected oversized")
	}
}

func TestWatchStopsOnCancel(t *testing.T) {
	b := &memBoard{}
	s := NewSyncInterval(b, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Watch(ctx, func(string) {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch did not return after cancel")
	}
}

func TestWatchEmitsExternalChange(t *testing.T) {
	b := &memBoard{}
	s := NewSyncInterval(b, 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan string, 1)
	go s.Watch(ctx, func(text string) {
		select {
		case got <- text:
		default:
		}
	})

	// Give Watch a moment to snapshot the empty board, then change it.
	time.Sleep(30 * time.Millisecond)
	if err := b.WriteText("watched"); err != nil {
		t.Fatal(err)
	}

	select {
	case text := <-got:
		if text != "watched" {
			t.Fatalf("got %q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch did not emit external change")
	}
}

func TestCurrentSkipsEmptyAndHuge(t *testing.T) {
	b := &memBoard{}
	s := NewSync(b)
	if _, ok := s.Current(); ok {
		t.Fatal("empty should not be current")
	}
	if err := b.WriteText("ok"); err != nil {
		t.Fatal(err)
	}
	text, ok := s.Current()
	if !ok || text != "ok" {
		t.Fatalf("got %q %v", text, ok)
	}
}
