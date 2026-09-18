package rdp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/pion/webrtc/v4"

	"github.com/codemodify/lan-screen-control/web"
)

// SignalRequest is the browser's complete (non-trickle) SDP offer.
type SignalRequest struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

// SignalResponse is either an SDP answer or a structured error.
type SignalResponse struct {
	SDP   string `json:"sdp,omitempty"`
	Type  string `json:"type,omitempty"`
	Error string `json:"error,omitempty"`
}

// Handler serves the static client and the HTTP signaling API.
func Handler(hub *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/signal", hub.handleSignal)
	mux.HandleFunc("GET /api/status", hub.handleStatus)
	mux.Handle("GET /", http.FileServer(http.FS(web.FS)))
	return mux
}

func (h *Hub) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"busy": h.Busy()})
}

func (h *Hub) handleSignal(w http.ResponseWriter, r *http.Request) {
	var req SignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SignalResponse{Error: "invalid JSON body"})
		return
	}
	if req.SDP == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, SignalResponse{Error: "sdp and type are required"})
		return
	}

	answer, err := h.AnswerOffer(webrtc.SessionDescription{Type: webrtc.NewSDPType(req.Type), SDP: req.SDP})
	if errors.Is(err, ErrBusy) {
		slog.Warn("rejected second client")
		writeJSON(w, http.StatusConflict, SignalResponse{
			Error: "another client is already connected",
		})
		return
	}
	if err != nil {
		slog.Error("signaling failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, SignalResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, SignalResponse{SDP: answer.SDP, Type: answer.Type.String()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
