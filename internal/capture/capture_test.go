package capture

import (
	"strings"
	"testing"
)

func TestFFmpegArgsIncludeH264Pipe(t *testing.T) {
	cfg := Config{FFmpeg: "ffmpeg", FPS: 20, Height: 720, Device: "1"}
	args := cfg.Args()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "libx264") || !strings.Contains(joined, "pipe:1") {
		t.Fatalf("expected H.264 pipe output, got %q", joined)
	}
	if !strings.Contains(joined, "zerolatency") {
		t.Fatalf("expected zerolatency tune, got %q", joined)
	}
}
