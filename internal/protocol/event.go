// Package protocol is the JSON schema for remote input over the WebRTC data channel.
package protocol

// Event is a single mouse or keyboard action from the browser.
// Coordinates X/Y are normalized to the video surface in the range [0, 1].
type Event struct {
	Type   string  `json:"t"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"b,omitempty"`
	DX     float64 `json:"dx,omitempty"`
	DY     float64 `json:"dy,omitempty"`
	Key    string  `json:"k,omitempty"`
	Repeat bool    `json:"r,omitempty"`
}

// Event type identifiers (kept short for the data channel).
const (
	TypeMouseMove = "mm"
	TypeMouseDown = "md"
	TypeMouseUp   = "mu"
	TypeWheel     = "wh"
	TypeKeyDown   = "kd"
	TypeKeyUp     = "ku"
)
