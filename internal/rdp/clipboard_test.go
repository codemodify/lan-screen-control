package rdp

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/codemodify/lan-screen-control/internal/clipboard"
	"github.com/codemodify/lan-screen-control/internal/protocol"
)

type recBoard struct {
	mu    sync.Mutex
	text  string
	ok    bool
	count int
}

func (r *recBoard) WriteText(s string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.text = s
	r.ok = true
	r.count++
	return nil
}

func (r *recBoard) ReadText() (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.text, r.ok, nil
}

func (r *recBoard) ChangeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func TestHandleInputSetsClipboard(t *testing.T) {
	board := &recBoard{}
	hub, err := NewHub(Config{Source: blockingSource{}, Clipboard: board})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(protocol.Event{Type: protocol.TypeClipboard, Data: "paste me"})
	if err != nil {
		t.Fatal(err)
	}
	hub.handleInput(raw, nil)
	if board.text != "paste me" {
		t.Fatalf("host clipboard: %q", board.text)
	}
}

func TestHandleInputDropsOversizedClipboard(t *testing.T) {
	board := &recBoard{}
	hub, err := NewHub(Config{Source: blockingSource{}, Clipboard: board})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(protocol.Event{
		Type: protocol.TypeClipboard,
		Data: strings.Repeat("z", clipboard.MaxTextBytes+2),
	})
	if err != nil {
		t.Fatal(err)
	}
	hub.handleInput(raw, nil)
	if board.ok {
		t.Fatal("oversized clipboard should be dropped")
	}
}

func TestHandleInputIgnoresClipboardOnPointerEvents(t *testing.T) {
	board := &recBoard{}
	hub, err := NewHub(Config{Source: blockingSource{}, Clipboard: board})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(protocol.Event{Type: protocol.TypeMouseMove, X: 0.1, Y: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	hub.handleInput(raw, nil)
	if board.ok {
		t.Fatal("mouse events must not touch the pasteboard")
	}
}

func TestHandleClipboardReqUsesCurrent(t *testing.T) {
	board := &recBoard{text: "already-there", ok: true, count: 1}
	hub, err := NewHub(Config{Source: blockingSource{}, Clipboard: board})
	if err != nil {
		t.Fatal(err)
	}
	// dc is nil: pushClipboard should no-op without panicking.
	hub.handleInput([]byte(`{"t":"cb-req"}`), nil)
}
