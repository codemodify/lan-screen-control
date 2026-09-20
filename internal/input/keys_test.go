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

func TestContainRect1680x1050In1280x720(t *testing.T) {
	x, y, w, h := ContainRect(1280, 720, 1680, 1050)
	if y != 0 || h != 720 {
		t.Fatalf("16:10 in 16:9 should be pillarboxed: y=%v h=%v", y, h)
	}
	if abs(w-1152) > 0.01 || abs(x-64) > 0.01 {
		t.Fatalf("content rect want 64,0,1152,720 got %v,%v,%v,%v", x, y, w, h)
	}
}

func TestMapPointFromFrameUnpadsLetterbox(t *testing.T) {
	// Encoded 1280x720 with a 1680x1050 display (pillarbox).
	cx, cy := MapPointFromFrame(0.5, 0.5, 1680, 1050, 1280, 720)
	if abs(cx-840) > 0.5 || abs(cy-525) > 0.5 {
		t.Fatalf("frame center should be display center, got %v,%v", cx, cy)
	}

	// Left pad (x=32/1280) must clamp to the left edge of the desktop.
	lx, ly := MapPointFromFrame(32.0/1280.0, 0.5, 1680, 1050, 1280, 720)
	if lx != 0 || abs(ly-525) > 0.5 {
		t.Fatalf("left pad should map to x=0, got %v,%v", lx, ly)
	}

	// Left edge of the picture (x=64/1280) is display x=0.
	px, py := MapPointFromFrame(64.0/1280.0, 0, 1680, 1050, 1280, 720)
	if abs(px) > 0.5 || abs(py) > 0.5 {
		t.Fatalf("content origin should be display origin, got %v,%v", px, py)
	}

	// Native (no pad): identity.
	nx, ny := MapPointFromFrame(0.25, 0.8, 1680, 1050, 0, 0)
	if abs(nx-420) > 0.01 || abs(ny-840) > 0.01 {
		t.Fatalf("native map: got %v,%v", nx, ny)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestPrintableChar(t *testing.T) {
	cases := map[string]string{
		"a":     "a",
		"A":     "A",
		"!":     "!",
		" ":     " ",
		"1":     "1",
		"é":     "é",
		"":      "",
		"Enter": "",
		"Tab":   "",
		"ab":    "",
		"\n":    "",
		"\t":    "",
	}
	for in, want := range cases {
		if got := PrintableChar(in); got != want {
			t.Fatalf("PrintableChar(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPlanKeyInjectLockedUnicode(t *testing.T) {
	p, ok := planKeyInject(true, "KeyA", "a")
	if !ok || !p.unicode || !p.allTaps || p.char != "a" || p.code != 0x00 {
		t.Fatalf("locked printable: %+v ok=%v", p, ok)
	}

	p, ok = planKeyInject(true, "Digit1", "!")
	if !ok || !p.unicode || p.char != "!" {
		t.Fatalf("locked shifted digit: %+v ok=%v", p, ok)
	}

	// Unlock helper burst: character only, no KeyboardEvent.code.
	p, ok = planKeyInject(true, "", "p")
	if !ok || !p.unicode || !p.allTaps || p.char != "p" || p.code != 0 {
		t.Fatalf("char-only burst: %+v ok=%v", p, ok)
	}
}

func TestPlanKeyInjectLockedControlKeys(t *testing.T) {
	for _, code := range []string{"Enter", "Escape", "Tab", "Backspace"} {
		p, ok := planKeyInject(true, code, "")
		if !ok || p.unicode || !p.allTaps {
			t.Fatalf("%s should stay keycode+all taps: %+v ok=%v", code, p, ok)
		}
		want, mapped := keyCode(code)
		if !mapped || p.code != want {
			t.Fatalf("%s code: got 0x%02x want 0x%02x", code, p.code, want)
		}
	}
}

func TestPlanKeyInjectUnlockedKeycode(t *testing.T) {
	p, ok := planKeyInject(false, "KeyA", "a")
	if !ok || p.allTaps || p.code != 0x00 || p.char != "a" || !p.unicode {
		t.Fatalf("unlocked printable should keep keycode and attach unicode: %+v ok=%v", p, ok)
	}

	p, ok = planKeyInject(false, "Enter", "")
	if !ok || p.unicode || p.allTaps || p.code != 0x24 {
		t.Fatalf("unlocked enter: %+v ok=%v", p, ok)
	}

	if _, ok := planKeyInject(false, "Unidentified", ""); ok {
		t.Fatal("unmapped key without char should be dropped")
	}
}

func TestModifierFlag(t *testing.T) {
	if modifierFlag("ShiftLeft") == 0 || modifierFlag("KeyA") != 0 {
		t.Fatal("modifier flags look wrong")
	}
}

func TestPasswordFieldPoint(t *testing.T) {
	x, y := PasswordFieldPoint(1680, 1050)
	if abs(x-840) > 0.01 {
		t.Fatalf("password field x should be display center, got %v", x)
	}
	if y < 1050*0.5 || y > 1050*0.7 {
		t.Fatalf("password field y should sit below center, got %v", y)
	}
	zx, zy := PasswordFieldPoint(0, 0)
	if zx != 0 || zy != 0 {
		t.Fatalf("zero display should map to origin, got %v,%v", zx, zy)
	}
}

func TestStubPrepareForRemote(t *testing.T) {
	New().PrepareForRemote()
}
