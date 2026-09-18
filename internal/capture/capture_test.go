package capture

import (
	"runtime"
	"strings"
	"testing"
)

func TestFFmpegArgsWebRTCFriendly(t *testing.T) {
	cfg := Config{FFmpeg: "ffmpeg", FPS: 20, Width: 1280, Height: 720, Device: "1"}
	args := cfg.Args()
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"libx264",
		"pipe:1",
		"zerolatency",
		"yuv420p",
		"repeat-headers=1",
		"sliced-threads=0",
		"aud=1",
		"dump_extra",
		"force_original_aspect_ratio=decrease",
		"pad=1280:720",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in ffmpeg args, got %q", want, joined)
		}
	}
	if strings.Contains(joined, "sliced-threads=1") {
		t.Fatal("sliced-threads=1 produces multi-slice frames that green-screen Chrome")
	}
}

func TestScaleFilterEvenNative(t *testing.T) {
	got := (Config{Width: 0, Height: 0}).scaleFilter()
	if !strings.Contains(got, "trunc(iw/2)*2") || !strings.Contains(got, "format=yuv420p") {
		t.Fatalf("native scale should force even yuv420p, got %q", got)
	}
}

func TestVideotoolboxEncoderSelection(t *testing.T) {
	cfg := (Config{Encoder: "videotoolbox"}).normalized()
	if runtime.GOOS == "darwin" {
		if cfg.Encoder != EncoderVideoToolbox {
			t.Fatalf("darwin should keep videotoolbox, got %s", cfg.Encoder)
		}
		return
	}
	if cfg.Encoder != EncoderLibx264 {
		t.Fatalf("non-darwin should fall back to libx264, got %s", cfg.Encoder)
	}
}

func TestCaptureCursorDefaultOff(t *testing.T) {
	if (Config{}).captureCursorArg() != "0" {
		t.Fatal("host cursor must be omitted from the capture by default")
	}
	if (Config{CaptureCursor: true}).captureCursorArg() != "1" {
		t.Fatal("-capture-cursor should pass 1 to ffmpeg")
	}
}

func TestEncoderAlias(t *testing.T) {
	if (Config{Encoder: "x264"}).normalized().Encoder != EncoderLibx264 {
		t.Fatal("x264 should alias to libx264")
	}
}
