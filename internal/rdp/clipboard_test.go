package rdp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

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

func TestClipboardDataChannelRoundTrip(t *testing.T) {
	board := &recBoard{}
	hub, err := NewHub(Config{Source: blockingSource{}, Clipboard: board})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(hub))
	t.Cleanup(srv.Close)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		t.Fatal(err)
	}

	incoming := make(chan protocol.Event, 4)
	dc, err := pc.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{})
	dc.OnOpen(func() { close(opened) })
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var ev protocol.Event
		if json.Unmarshal(msg.Data, &ev) == nil {
			incoming <- ev
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-webrtc.GatheringCompletePromise(pc):
	case <-time.After(5 * time.Second):
		t.Fatal("ICE gathering timed out")
	}

	res := postSignal(t, srv, *pc.LocalDescription())
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("signal: %d", res.StatusCode)
	}
	var answer SignalResponse
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.NewSDPType(answer.Type),
		SDP:  answer.SDP,
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-opened:
	case <-time.After(8 * time.Second):
		t.Fatal("data channel did not open")
	}

	payload, err := json.Marshal(protocol.Event{Type: protocol.TypeClipboard, Data: "from-browser"})
	if err != nil {
		t.Fatal(err)
	}
	if err := dc.Send(payload); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for board.text != "from-browser" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if board.text != "from-browser" {
		t.Fatalf("Mac pasteboard not set: %q", board.text)
	}

	if err := board.WriteText("from-mac"); err != nil {
		t.Fatal(err)
	}

	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev := <-incoming:
			if ev.Type == protocol.TypeClipboard && ev.Data == "from-mac" {
				return
			}
		case <-timeout:
			t.Fatal("did not receive host clipboard over the data channel")
		}
	}
}

func boolPtr(v bool) *bool { return &v }
