package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClipboardEventRoundTrip(t *testing.T) {
	raw := []byte(`{"t":"cb","d":"hello from linux"}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeClipboard {
		t.Fatalf("type: got %q", ev.Type)
	}
	if ev.Data != "hello from linux" {
		t.Fatalf("data: got %q", ev.Data)
	}

	out, err := json.Marshal(Event{Type: TypeClipboard, Data: "mac text"})
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != TypeClipboard || back.Data != "mac text" {
		t.Fatalf("marshal round-trip: %+v", back)
	}
}

func TestClipboardReqHasNoData(t *testing.T) {
	var ev Event
	if err := json.Unmarshal([]byte(`{"t":"cb-req"}`), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeClipboardReq || ev.Data != "" {
		t.Fatalf("unexpected req: %+v", ev)
	}
}

func TestMouseEventIgnoresMissingData(t *testing.T) {
	var ev Event
	if err := json.Unmarshal([]byte(`{"t":"mm","x":0.5,"y":0.25}`), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeMouseMove || ev.Data != "" || ev.X != 0.5 {
		t.Fatalf("mouse decode: %+v", ev)
	}
}

func TestKeyEventParsesPrintableChar(t *testing.T) {
	var ev Event
	if err := json.Unmarshal([]byte(`{"t":"kd","k":"KeyA","c":"a"}`), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeKeyDown || ev.Key != "KeyA" || ev.Char != "a" {
		t.Fatalf("key+char: %+v", ev)
	}

	shifted := Event{Type: TypeKeyDown, Key: "Digit1", Char: "!"}
	out, err := json.Marshal(shifted)
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != TypeKeyDown || back.Key != "Digit1" || back.Char != "!" {
		t.Fatalf("marshal round-trip: %+v", back)
	}
}

func TestKeyEventCharOnlyBurst(t *testing.T) {
	var ev Event
	if err := json.Unmarshal([]byte(`{"t":"kd","c":"p"}`), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != TypeKeyDown || ev.Key != "" || ev.Char != "p" {
		t.Fatalf("char-only burst: %+v", ev)
	}
}

func TestKeyEventSpecialKeysStayCodeOnly(t *testing.T) {
	for _, raw := range []string{
		`{"t":"kd","k":"Enter"}`,
		`{"t":"ku","k":"Tab"}`,
		`{"t":"kd","k":"Backspace"}`,
		`{"t":"kd","k":"Escape"}`,
	} {
		var ev Event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Char != "" || ev.Key == "" {
			t.Fatalf("special key should be code-only: %s -> %+v", raw, ev)
		}
	}

	out, err := json.Marshal(Event{Type: TypeKeyDown, Key: "Enter"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); strings.Contains(got, `"c"`) {
		t.Fatalf("Enter should omit empty c: %s", got)
	}
}
