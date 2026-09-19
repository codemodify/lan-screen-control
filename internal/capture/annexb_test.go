package capture

import (
	"bytes"
	"io"
	"testing"

	"github.com/pion/webrtc/v4/pkg/media"
)

func TestAUBuilderGroupsSlicesAndPrependsHeaders(t *testing.T) {
	sps := []byte{0x67, 0x42, 0xC0, 0x1E}
	pps := []byte{0x68, 0xCE, 0x38, 0x80}
	idr0 := []byte{0x65, 0x88, 0x80, 0x11} // first_mb_in_slice = 0
	idr1 := []byte{0x65, 0x00, 0x80, 0x22} // continuation slice
	p0 := []byte{0x61, 0x88, 0x80, 0x33}

	var b auBuilder
	if au, ok := b.Push(sps); ok || au != nil {
		t.Fatal("SPS must not emit an AU")
	}
	if au, ok := b.Push(pps); ok || au != nil {
		t.Fatal("PPS must not emit an AU")
	}
	if au, ok := b.Push(idr0); ok {
		t.Fatalf("first IDR slice should stay buffered, got %d bytes", len(au))
	}
	if au, ok := b.Push(idr1); ok {
		t.Fatalf("second IDR slice should stay in the same AU, got %d bytes", len(au))
	}

	au, ok := b.Push(p0)
	if !ok {
		t.Fatal("P-frame with first_mb=0 should flush the IDR AU")
	}
	if countStartCodes(au) != 4 { // SPS + PPS + idr0 + idr1
		t.Fatalf("IDR AU should be SPS+PPS+2 slices, got %d NALs", countStartCodes(au))
	}
	if !bytes.Contains(au, sps) || !bytes.Contains(au, pps) || !bytes.Contains(au, idr0) || !bytes.Contains(au, idr1) {
		t.Fatalf("IDR AU missing parameter sets or slices: %x", au)
	}
	if bytes.Contains(au, p0) {
		t.Fatal("P-frame must not be mixed into the IDR AU")
	}

	tail, ok := b.Flush()
	if !ok {
		t.Fatal("expected trailing P-frame AU")
	}
	if bytes.Contains(tail, sps) {
		t.Fatal("non-IDR AU should not repeat SPS")
	}
	if !bytes.Contains(tail, p0) {
		t.Fatalf("P-frame AU missing slice: %x", tail)
	}
}

func TestAUBuilderAUDFlushes(t *testing.T) {
	var b auBuilder
	_, _ = b.Push([]byte{0x67, 0x42})
	_, _ = b.Push([]byte{0x68, 0xCE})
	_, _ = b.Push([]byte{0x65, 0x88, 0x01})
	au, ok := b.Push([]byte{0x09, 0x10}) // AUD
	if !ok {
		t.Fatal("AUD should flush the buffered IDR")
	}
	if countStartCodes(au) != 3 {
		t.Fatalf("expected SPS+PPS+IDR, got %d NALs", countStartCodes(au))
	}
}

func TestPumpH264WritesOneSamplePerAU(t *testing.T) {
	stream := concatAnnexB(
		[]byte{0x67, 0x42, 0xC0, 0x1E},
		[]byte{0x68, 0xCE, 0x38, 0x80},
		[]byte{0x65, 0x88, 0x80, 0x11},
		[]byte{0x65, 0x00, 0x80, 0x22},
		[]byte{0x61, 0x88, 0x80, 0x33},
	)
	var samples []media.Sample
	err := pumpH264(bytes.NewReader(stream), 20, func(s media.Sample) error {
		samples = append(samples, s)
		return nil
	})
	if err != io.EOF {
		t.Fatalf("pumpH264: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("want 2 access units, got %d", len(samples))
	}
	if samples[0].Duration == 0 || samples[1].Duration == 0 {
		t.Fatal("each AU sample needs a frame duration")
	}
	if countStartCodes(samples[0].Data) != 4 {
		t.Fatalf("first sample should be one IDR AU (SPS+PPS+2 slices), got %d NALs", countStartCodes(samples[0].Data))
	}
}

func concatAnnexB(nals ...[]byte) []byte {
	var out []byte
	for _, n := range nals {
		out = append(out, startCodeAnnexB...)
		out = append(out, n...)
	}
	return out
}
