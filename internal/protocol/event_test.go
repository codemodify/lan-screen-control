package protocol

import (
	"encoding/json"
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
