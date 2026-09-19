package rdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/codemodify/lan-screen-control/internal/capture"
	"github.com/codemodify/lan-screen-control/internal/input"
	"github.com/codemodify/lan-screen-control/internal/protocol"
)

// ErrBusy is returned when a second client tries to take the single slot.
var ErrBusy = errors.New("another client is already connected")

// Config wires capture, injection, and ICE for a Hub.
type Config struct {
	Source capture.Source
	Input  input.Injector
	STUN   string // empty disables STUN; host candidates are always gathered
}

// Hub allows exactly one concurrent WebRTC session.
type Hub struct {
	cfg Config
	api *webrtc.API

	mu      sync.Mutex
	current *Session
}

// Session is one connected browser.
type Session struct {
	pc     *webrtc.PeerConnection
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once

	stateMu sync.Mutex
	disc    *time.Timer
}

// NewHub builds the media API and empty session slot.
func NewHub(cfg Config) (*Hub, error) {
	api, err := newWebRTCAPI()
	if err != nil {
		return nil, err
	}
	return &Hub{cfg: cfg, api: api}, nil
}

// Busy reports whether a session is currently holding the slot.
func (h *Hub) Busy() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current != nil && !h.current.finished()
}

// AnswerOffer creates a peer connection for the browser's SDP offer.
// If a session is already active the offer is rejected with ErrBusy.
func (h *Hub) AnswerOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	h.mu.Lock()
	if h.current != nil && !h.current.finished() {
		h.mu.Unlock()
		return nil, ErrBusy
	}
	// Drop a stale pointer so a new session can take the slot.
	h.current = nil
	h.mu.Unlock()

	sess, answer, err := h.start(offer)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	if h.current != nil && !h.current.finished() {
		h.mu.Unlock()
		sess.close()
		return nil, ErrBusy
	}
	h.current = sess
	h.mu.Unlock()
	return answer, nil
}

func (h *Hub) start(offer webrtc.SessionDescription) (*Session, *webrtc.SessionDescription, error) {
	ice := webrtc.Configuration{}
	if h.cfg.STUN != "" {
		ice.ICEServers = []webrtc.ICEServer{{URLs: []string{h.cfg.STUN}}}
	}

	pc, err := h.api.NewPeerConnection(ice)
	if err != nil {
		return nil, nil, fmt.Errorf("new peer connection: %w", err)
	}

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		"screen",
		"lan-screen-control",
	)
	if err != nil {
		_ = pc.Close()
		return nil, nil, fmt.Errorf("video track: %w", err)
	}
	rtpSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		_ = pc.Close()
		return nil, nil, fmt.Errorf("add track: %w", err)
	}
	go drainRTCP(rtpSender)

	ctx, cancel := context.WithCancel(context.Background())
	sess := &Session{pc: pc, cancel: cancel, done: make(chan struct{})}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		slog.Info("data channel open", "label", dc.Label())
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			h.handleInput(msg.Data)
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		slog.Info("peer connection", "state", state.String())
		sess.onPeerState(state)
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		sess.close()
		return nil, nil, fmt.Errorf("set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		sess.close()
		return nil, nil, fmt.Errorf("create answer: %w", err)
	}
	gather := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		sess.close()
		return nil, nil, fmt.Errorf("set local description: %w", err)
	}
	select {
	case <-gather:
	case <-time.After(8 * time.Second):
		slog.Warn("ICE gathering timed out; sending partial SDP")
	}

	go func() {
		defer sess.close()
		src := h.cfg.Source
		if src == nil {
			return
		}
		err := src.Run(ctx, func(sample media.Sample) error {
			return videoTrack.WriteSample(sample)
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			slog.Error("screen capture stopped", "err", err)
		}
	}()

	local := pc.LocalDescription()
	return sess, local, nil
}

func (h *Hub) handleInput(raw []byte) {
	if h.cfg.Input == nil {
		return
	}
	var ev protocol.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		slog.Debug("bad input event", "err", err)
		return
	}
	h.cfg.Input.Apply(ev)
}

func (s *Session) onPeerState(state webrtc.PeerConnectionState) {
	var startGrace, doClose bool
	s.stateMu.Lock()
	switch state {
	case webrtc.PeerConnectionStateConnected:
		if s.disc != nil {
			s.disc.Stop()
			s.disc = nil
		}
	case webrtc.PeerConnectionStateDisconnected:
		startGrace = s.disc == nil
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		if s.disc != nil {
			s.disc.Stop()
			s.disc = nil
		}
		doClose = true
	}
	if startGrace {
		s.disc = time.AfterFunc(1500*time.Millisecond, s.close)
	}
	s.stateMu.Unlock()
	if doClose {
		s.close()
	}
}

func (s *Session) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.stateMu.Lock()
		if s.disc != nil {
			s.disc.Stop()
			s.disc = nil
		}
		s.stateMu.Unlock()
		s.cancel()
		_ = s.pc.Close()
		close(s.done)
	})
}

func (s *Session) finished() bool {
	if s == nil {
		return true
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func newWebRTCAPI() (*webrtc.API, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: "goog-remb"},
				{Type: "ccm", Parameter: "fir"},
				{Type: "nack"},
				{Type: "nack", Parameter: "pli"},
			},
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}

	se := webrtc.SettingEngine{}
	se.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})
	// Fail a vanished client quickly so the single session slot is released.
	se.SetICETimeouts(4*time.Second, 8*time.Second, 2*time.Second)

	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se)), nil
}
