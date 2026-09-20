package input

import "unicode"

// PrintableChar returns a single printable rune from a protocol `c` field.
// Enter/Tab/Backspace and other named keys are rejected so they stay code-only.
func PrintableChar(s string) string {
	if s == "" {
		return ""
	}
	rs := []rune(s)
	if len(rs) != 1 {
		return ""
	}
	r := rs[0]
	if unicode.IsPrint(r) || r == ' ' {
		return string(r)
	}
	return ""
}

// keyInjectPlan is how a remote key event is posted on macOS.
type keyInjectPlan struct {
	code    uint16
	char    string
	allTaps bool
	unicode bool
}

// planKeyInject chooses virtual-keycode vs unicode injection.
// Locked + printable uses CGEventKeyboardSetUnicodeString (Secure Input).
// Locked Enter/Escape/Tab/Backspace stay on the keycode path, posted to all taps.
func planKeyInject(locked bool, key, char string) (keyInjectPlan, bool) {
	ch := PrintableChar(char)
	code, ok := keyCode(key)
	if locked && ch != "" {
		p := keyInjectPlan{char: ch, allTaps: true, unicode: true}
		if ok {
			p.code = code
		}
		return p, true
	}
	if ok {
		return keyInjectPlan{code: code, char: ch, allTaps: locked, unicode: ch != ""}, true
	}
	if ch != "" {
		return keyInjectPlan{char: ch, allTaps: locked, unicode: true}, true
	}
	return keyInjectPlan{}, false
}

// keyCode maps a UI Events KeyboardEvent.code value to a macOS virtual key code
// (ANSI / US layout, from HIToolbox/Events.h).
func keyCode(code string) (uint16, bool) {
	v, ok := macKeyCodes[code]
	return v, ok
}

// modifierFlag is the CGEventFlags bit associated with a modifier key.
func modifierFlag(code string) uint64 {
	switch code {
	case "ShiftLeft", "ShiftRight":
		return flagMaskShift
	case "ControlLeft", "ControlRight":
		return flagMaskControl
	case "AltLeft", "AltRight":
		return flagMaskAlternate
	case "MetaLeft", "MetaRight":
		return flagMaskCommand
	case "CapsLock":
		return flagMaskAlphaShift
	default:
		return 0
	}
}

// Quartz event flag masks (CGEventFlags).
const (
	flagMaskAlphaShift = 0x00010000
	flagMaskShift      = 0x00020000
	flagMaskControl    = 0x00040000
	flagMaskAlternate  = 0x00080000
	flagMaskCommand    = 0x00100000
)

// macKeyCodes is the US-ANSI virtual key table.
// Values match kVK_* constants in <HIToolbox/Events.h>.
var macKeyCodes = map[string]uint16{
	"KeyA": 0x00, "KeyS": 0x01, "KeyD": 0x02, "KeyF": 0x03,
	"KeyH": 0x04, "KeyG": 0x05, "KeyZ": 0x06, "KeyX": 0x07,
	"KeyC": 0x08, "KeyV": 0x09, "KeyB": 0x0B, "KeyQ": 0x0C,
	"KeyW": 0x0D, "KeyE": 0x0E, "KeyR": 0x0F, "KeyY": 0x10,
	"KeyT": 0x11, "KeyI": 0x22, "KeyP": 0x23, "KeyL": 0x25,
	"KeyJ": 0x26, "KeyK": 0x28, "KeyN": 0x2D, "KeyM": 0x2E,
	"KeyO": 0x1F, "KeyU": 0x20,

	"Digit1": 0x12, "Digit2": 0x13, "Digit3": 0x14, "Digit4": 0x15,
	"Digit5": 0x16, "Digit6": 0x17, "Digit7": 0x1A, "Digit8": 0x1C,
	"Digit9": 0x19, "Digit0": 0x1D,

	"Equal": 0x18, "Minus": 0x1B, "BracketRight": 0x1E, "BracketLeft": 0x21,
	"Quote": 0x27, "Semicolon": 0x29, "Backslash": 0x2A, "Comma": 0x2B,
	"Slash": 0x2C, "Period": 0x2F, "Backquote": 0x32, "IntlBackslash": 0x0A,

	"Enter": 0x24, "NumpadEnter": 0x4C, "Tab": 0x30, "Space": 0x31,
	"Backspace": 0x33, "Escape": 0x35, "Delete": 0x75,
	"Home": 0x73, "End": 0x77, "PageUp": 0x74, "PageDown": 0x79,
	"Help": 0x72, "Insert": 0x72,

	"ArrowLeft": 0x7B, "ArrowRight": 0x7C, "ArrowDown": 0x7D, "ArrowUp": 0x7E,

	"MetaLeft": 0x37, "MetaRight": 0x36,
	"ShiftLeft": 0x38, "ShiftRight": 0x3C,
	"CapsLock": 0x39,
	"AltLeft":  0x3A, "AltRight": 0x3D,
	"ControlLeft": 0x3B, "ControlRight": 0x3E,
	"Fn": 0x3F,

	"F1": 0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76,
	"F5": 0x60, "F6": 0x61, "F7": 0x62, "F8": 0x64,
	"F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F,
	"F13": 0x69, "F14": 0x6B, "F15": 0x71, "F16": 0x6A,
	"F17": 0x40, "F18": 0x4F, "F19": 0x50, "F20": 0x5A,

	"NumpadDecimal": 0x41, "NumpadMultiply": 0x43, "NumpadAdd": 0x45,
	"NumpadNumLock": 0x47, "NumLock": 0x47,
	"NumpadDivide": 0x4B, "NumpadSubtract": 0x4E, "NumpadEqual": 0x51,
	"Numpad0": 0x52, "Numpad1": 0x53, "Numpad2": 0x54, "Numpad3": 0x55,
	"Numpad4": 0x56, "Numpad5": 0x57, "Numpad6": 0x58, "Numpad7": 0x59,
	"Numpad8": 0x5B, "Numpad9": 0x5C,

	"AudioVolumeUp": 0x48, "AudioVolumeDown": 0x49, "AudioVolumeMute": 0x4A,
}
