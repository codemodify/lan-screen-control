// Package protocol is the JSON schema for remote input over the WebRTC data channel.
package protocol

// Event is a single mouse, keyboard, or clipboard action from the browser
// (or, for TypeClipboard, from the Mac host back to the browser).
// Coordinates X/Y are normalized to the video surface in the range [0, 1].
type Event struct {
	Type   string  `json:"t"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"b,omitempty"` // DOM button: 0 left, 1 middle, 2 right
	DX     float64 `json:"dx,omitempty"`
	DY     float64 `json:"dy,omitempty"`
	Key    string  `json:"k,omitempty"`
	Char   string  `json:"c,omitempty"` // printable character from KeyboardEvent.key (length 1)
	Repeat bool    `json:"r,omitempty"`
	Data   string  `json:"d,omitempty"`
	// Mods is the keyboard modifiers actually held for a pointer event
	// (shift=1, ctrl=2, alt=4, meta=8). Mouse injection uses this snapshot
	// instead of latched key state: a missed Control keyup would otherwise
	// stick, and macOS treats Control+left click as a context click.
	Mods int `json:"m,omitempty"`
}

// Pointer modifier bits carried on mouse/wheel events (Event.Mods).
const (
	ModShift = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
)

// Event type identifiers (kept short for the data channel).
const (
	TypeMouseMove    = "mm"
	TypeMouseDown    = "md"
	TypeMouseUp      = "mu"
	TypeWheel        = "wh"
	TypeKeyDown      = "kd"
	TypeKeyUp        = "ku"
	TypeClipboard    = "cb"     // set clipboard text (either direction)
	TypeClipboardReq = "cb-req" // browser asks the host to push current text
)
