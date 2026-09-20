package rdp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/codemodify/lan-screen-control/internal/input"
	"github.com/codemodify/lan-screen-control/internal/protocol"
)

type blockingSource struct{}

func (blockingSource) Run(ctx context.Context, _ func(media.Sample) error) error {
	<-ctx.Done()
	return ctx.Err()
}

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	hub, err := NewHub(Config{
		Source: blockingSource{},
		Input:  input.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return hub
}

func offerer(t *testing.T) (*webrtc.PeerConnection, webrtc.SessionDescription) {
	t.Helper()
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
	if _, err := pc.CreateDataChannel("input", nil); err != nil {
		t.Fatal(err)
	}
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
	return pc, *pc.LocalDescription()
}

func postSignal(t *testing.T, srv *httptest.Server, desc webrtc.SessionDescription) *http.Response {
	t.Helper()
	body, err := json.Marshal(SignalRequest{SDP: desc.SDP, Type: desc.Type.String()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(srv.URL+"/api/signal", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSignalRejectsSecondClient(t *testing.T) {
	hub := newTestHub(t)
	srv := httptest.NewServer(Handler(hub))
	t.Cleanup(srv.Close)

	pc1, offer1 := offerer(t)
	res1 := postSignal(t, srv, offer1)
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("first client: got %d", res1.StatusCode)
	}
	var answer SignalResponse
	if err := json.NewDecoder(res1.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if err := pc1.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.NewSDPType(answer.Type),
		SDP:  answer.SDP,
	}); err != nil {
		t.Fatal(err)
	}

	_, offer2 := offerer(t)
	res2 := postSignal(t, srv, offer2)
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("second client should be 409, got %d", res2.StatusCode)
	}
	var busy SignalResponse
	if err := json.NewDecoder(res2.Body).Decode(&busy); err != nil {
		t.Fatal(err)
	}
	if busy.Error == "" {
		t.Fatal("expected a clear busy error")
	}

	statusRes, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer statusRes.Body.Close()
	var status map[string]bool
	if err := json.NewDecoder(statusRes.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status["busy"] {
		t.Fatal("status should report busy while first client is active")
	}

	_ = pc1.Close()
	deadline := time.Now().Add(10 * time.Second)
	for hub.Busy() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if hub.Busy() {
		t.Fatal("slot should free after the first peer closes")
	}

	_, offer3 := offerer(t)
	res3 := postSignal(t, srv, offer3)
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Fatalf("third client after release: got %d", res3.StatusCode)
	}
}

func TestIndexServed(t *testing.T) {
	hub := newTestHub(t)
	srv := httptest.NewServer(Handler(hub))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("If the screen is locked")) {
		t.Fatal("index should tell the user they can type the password on a black lock screen")
	}

	jsRes, err := http.Get(srv.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer jsRes.Body.Close()
	js, err := io.ReadAll(jsRes.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(js, []byte(`document.addEventListener("keydown"`)) {
		t.Fatal("client must listen for keydown on the page, not only the video element")
	}
}

type recordingInjector struct {
	mu   sync.Mutex
	prep int
}

func (r *recordingInjector) Apply(protocol.Event)    {}
func (r *recordingInjector) DisplaySize() (int, int) { return 1920, 1080 }
func (r *recordingInjector) PrepareForRemote()       { r.mu.Lock(); r.prep++; r.mu.Unlock() }
func (r *recordingInjector) preps() int              { r.mu.Lock(); defer r.mu.Unlock(); return r.prep }

type captureInjector struct {
	mu sync.Mutex
	ev []protocol.Event
}

func (c *captureInjector) Apply(ev protocol.Event) {
	c.mu.Lock()
	c.ev = append(c.ev, ev)
	c.mu.Unlock()
}
func (c *captureInjector) DisplaySize() (int, int) { return 1920, 1080 }
func (c *captureInjector) PrepareForRemote()       {}
func (c *captureInjector) events() []protocol.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]protocol.Event, len(c.ev))
	copy(out, c.ev)
	return out
}

func TestHandleInputDeliversKeysWithoutVideo(t *testing.T) {
	inj := &captureInjector{}
	hub, err := NewHub(Config{Source: blockingSource{}, Input: inj})
	if err != nil {
		t.Fatal(err)
	}
	// blockingSource never produces frames — keys must still be injected.
	for _, raw := range [][]byte{
		[]byte(`{"t":"kd","k":"KeyA"}`),
		[]byte(`{"t":"ku","k":"KeyA"}`),
		[]byte(`{"t":"kd","k":"Enter"}`),
		[]byte(`{"t":"ku","k":"Enter"}`),
	} {
		hub.handleInput(raw, nil)
	}
	got := inj.events()
	if len(got) != 4 {
		t.Fatalf("want 4 key events, got %#v", got)
	}
	if got[0].Type != protocol.TypeKeyDown || got[0].Key != "KeyA" {
		t.Fatalf("first event: %#v", got[0])
	}
	if got[2].Type != protocol.TypeKeyDown || got[2].Key != "Enter" {
		t.Fatalf("enter down: %#v", got[2])
	}
}

func TestPrepareForRemoteOnAccept(t *testing.T) {
	rec := &recordingInjector{}
	hub, err := NewHub(Config{Source: blockingSource{}, Input: rec})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(hub))
	t.Cleanup(srv.Close)

	_, offer := offerer(t)
	res := postSignal(t, srv, offer)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("signal: got %d", res.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	for rec.preps() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rec.preps() == 0 {
		t.Fatal("PrepareForRemote should run after session accept (after wake-display)")
	}
}
