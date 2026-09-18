package input

import "testing"

func TestKeyCodeUSANSI(t *testing.T) {
	cases := map[string]uint16{
		"KeyA":      0x00,
		"Enter":     0x24,
		"Space":     0x31,
		"Escape":    0x35,
		"ArrowLeft": 0x7B,
		"MetaLeft":  0x37,
		"F1":        0x7A,
	}
	for code, want := range cases {
		got, ok := keyCode(code)
		if !ok {
			t.Fatalf("key %s should be mapped", code)
		}
		if got != want {
			t.Fatalf("key %s: got 0x%02x want 0x%02x", code, got, want)
		}
	}
	if _, ok := keyCode("Unidentified"); ok {
		t.Fatal("unknown key should not map")
	}
}

func TestMapPointClamps(t *testing.T) {
	x, y := MapPoint(-1, 2, 100, 50)
	if x != 0 || y != 50 {
		t.Fatalf("clamp failed: %v,%v", x, y)
	}
	x, y = MapPoint(0.5, 0.5, 200, 100)
	if x != 100 || y != 50 {
		t.Fatalf("midpoint failed: %v,%v", x, y)
	}
}

func TestModifierFlag(t *testing.T) {
	if modifierFlag("ShiftLeft") == 0 || modifierFlag("KeyA") != 0 {
		t.Fatal("modifier flags look wrong")
	}
}
