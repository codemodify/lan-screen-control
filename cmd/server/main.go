// Command server is the LAN remote-desktop host.
//
// It captures the local screen (ffmpeg + H.264), serves a one-page WebRTC
// client, and injects mouse/keyboard events on macOS.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/codemodify/lan-screen-control/internal/capture"
	"github.com/codemodify/lan-screen-control/internal/input"
	"github.com/codemodify/lan-screen-control/internal/rdp"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:62000", "HTTP listen address (LAN-accessible)")
	ffmpeg := flag.String("ffmpeg", "ffmpeg", "path to the ffmpeg binary")
	device := flag.String("device", "1", "AVFoundation video device index (screen is usually 1)")
	fps := flag.Int("fps", 20, "capture frame rate")
	width := flag.Int("width", 1280, "encoded canvas width in pixels (0 with -height 0 = even native)")
	height := flag.Int("height", 720, "encoded canvas height in pixels (0 with -width 0 = even native)")
	encoder := flag.String("encoder", "libx264", "H.264 encoder: libx264 (default) or videotoolbox (macOS)")
	stun := flag.String("stun", "stun:stun.l.google.com:19302", "optional STUN URL; empty disables STUN")
	listDevices := flag.Bool("list-devices", false, "print ffmpeg capture devices and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if *listDevices {
		if err := capture.ListDevices(*ffmpeg); err != nil {
			slog.Error("list devices", "err", err)
			os.Exit(1)
		}
		return
	}

	hub, err := rdp.NewHub(rdp.Config{
		Source: capture.New(capture.Config{
			FFmpeg:  *ffmpeg,
			FPS:     *fps,
			Width:   *width,
			Height:  *height,
			Device:  *device,
			Encoder: *encoder,
		}),
		Input: input.New(),
		STUN:  *stun,
	})
	if err != nil {
		slog.Error("webrtc init", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           rdp.Handler(hub),
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("lan-screen-control listening",
		"addr", "http://"+*addr+"/",
		"device", *device,
		"fps", *fps,
		"width", *width,
		"height", *height,
		"encoder", *encoder,
	)
	slog.Info("grant Screen Recording + Accessibility to this process (or to Terminal) on macOS")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
}
