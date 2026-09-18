// Package capture produces an Annex-B H.264 elementary stream of the local screen.
//
// On macOS the default source is ffmpeg's AVFoundation screen device. ffmpeg
// must be on PATH (or passed via -ffmpeg). Homebrew: `brew install ffmpeg`.
package capture

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

// Config selects how ffmpeg grabs and encodes the primary display.
type Config struct {
	FFmpeg string // path to ffmpeg (default "ffmpeg")
	FPS    int
	Height int    // scale output height; 0 keeps native resolution
	Device string // AVFoundation video index; screen is usually "1"
}

// Source writes H.264 NAL units until the context is cancelled.
type Source interface {
	Run(ctx context.Context, write func(media.Sample) error) error
}

// FFmpegSource captures via an ffmpeg subprocess.
type FFmpegSource struct {
	Config Config
}

// New returns the production screen-capture source.
func New(cfg Config) Source {
	if cfg.FFmpeg == "" {
		cfg.FFmpeg = "ffmpeg"
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 20
	}
	if cfg.Device == "" {
		cfg.Device = "1"
	}
	return &FFmpegSource{Config: cfg}
}

// Run starts ffmpeg and forwards each NAL to write. It returns when ffmpeg
// exits or ctx is cancelled.
func (s *FFmpegSource) Run(ctx context.Context, write func(media.Sample) error) error {
	args := s.Config.Args()
	cmd := exec.CommandContext(ctx, s.Config.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	slog.Info("ffmpeg started", "args", args)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stderr.Read(buf)
			if n > 0 {
				slog.Debug("ffmpeg", "stderr", string(buf[:n]))
			}
			if rerr != nil {
				return
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- pumpH264(stdout, s.Config.FPS, write)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case runErr = <-errCh:
	}
	_ = cmd.Process.Kill()
	waitErr := cmd.Wait()
	if runErr != nil && ctx.Err() == nil {
		if waitErr != nil {
			return fmt.Errorf("ffmpeg encode: %v (process: %w)", runErr, waitErr)
		}
		return runErr
	}
	return nil
}

// Args builds the ffmpeg command line for the current OS.
func (c Config) Args() []string {
	fps := strconv.Itoa(c.FPS)
	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "nobuffer"}

	switch runtime.GOOS {
	case "darwin":
		args = append(args,
			"-f", "avfoundation",
			"-capture_cursor", "1",
			"-framerate", fps,
			"-i", c.Device+":none",
		)
	default:
		// Synthetic pattern so the WebRTC path can be exercised off-Mac.
		args = append(args,
			"-f", "lavfi",
			"-i", fmt.Sprintf("testsrc=size=1280x720:rate=%d", c.FPS),
		)
	}

	args = append(args, "-an")
	if c.Height > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", c.Height))
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-level", "3.1",
		"-pix_fmt", "yuv420p",
		"-g", fps,
		"-keyint_min", fps,
		"-bf", "0",
		"-x264-params", "scenecut=0:bframes=0:sliced-threads=1",
		"-f", "h264",
		"pipe:1",
	)
	return args
}

func pumpH264(r io.Reader, fps int, write func(media.Sample) error) error {
	reader, err := h264reader.NewReader(r)
	if err != nil {
		return err
	}
	dur := time.Second / time.Duration(fps)
	if dur <= 0 {
		dur = 50 * time.Millisecond
	}
	for {
		nal, err := reader.NextNAL()
		if err != nil {
			if err == io.EOF {
				return io.EOF
			}
			return err
		}
		data := withStartCode(nal.Data)
		sample := media.Sample{Data: data, Duration: 0}
		switch nal.UnitType {
		case h264reader.NalUnitTypeCodedSliceIdr,
			h264reader.NalUnitTypeCodedSliceNonIdr,
			h264reader.NalUnitTypeCodedSliceDataPartitionA,
			h264reader.NalUnitTypeCodedSliceDataPartitionB,
			h264reader.NalUnitTypeCodedSliceDataPartitionC:
			sample.Duration = dur
		}
		if err := write(sample); err != nil {
			return err
		}
	}
}

func withStartCode(nal []byte) []byte {
	if hasStartCode(nal) {
		return nal
	}
	out := make([]byte, 0, 4+len(nal))
	out = append(out, 0x00, 0x00, 0x00, 0x01)
	return append(out, nal...)
}

func hasStartCode(b []byte) bool {
	if len(b) >= 4 && b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 1 {
		return true
	}
	if len(b) >= 3 && b[0] == 0 && b[1] == 0 && b[2] == 1 {
		return true
	}
	return false
}

// ListDevices prints ffmpeg's AVFoundation device list (macOS) and returns.
func ListDevices(ffmpeg string) error {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command(ffmpeg, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	} else {
		cmd = exec.Command(ffmpeg, "-hide_banner", "-devices")
	}
	out, err := cmd.CombinedOutput()
	fmt.Print(string(out))
	return err
}
