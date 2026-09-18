package rdp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/codemodify/lan-screen-control/internal/input"
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
}
