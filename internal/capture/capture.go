// Package capture produces an Annex-B H.264 elementary stream of the local screen.
//
// On macOS the default source is ffmpeg's AVFoundation screen device. ffmpeg
// must be on PATH (or passed via -ffmpeg). Homebrew: `brew install ffmpeg`.
package capture

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

// Encoder names accepted by Config.Encoder.
const (
	EncoderLibx264      = "libx264"
	EncoderVideoToolbox = "videotoolbox"
)

// Config selects how ffmpeg grabs and encodes the primary display.
type Config struct {
	FFmpeg        string // path to ffmpeg (default "ffmpeg")
	FPS           int
	Width         int    // encoded canvas width; 0 with Height 0 = even native
	Height        int    // encoded canvas height; 0 with Width 0 = even native
	Device        string // AVFoundation video index; screen is usually "1"
	Encoder       string // libx264 (default) or videotoolbox (macOS)
	CaptureCursor bool   // draw the host pointer into the video (default false)
}

// Source writes H.264 access units until the context is cancelled.
type Source interface {
	Run(ctx context.Context, write func(media.Sample) error) error
}

// FFmpegSource captures via an ffmpeg subprocess.
type FFmpegSource struct {
	Config Config
}

// New returns the production screen-capture source.
func New(cfg Config) Source {
	cfg = cfg.normalized()
	return &FFmpegSource{Config: cfg}
}

func (c Config) normalized() Config {
	if c.FFmpeg == "" {
		c.FFmpeg = "ffmpeg"
	}
	if c.FPS <= 0 {
		c.FPS = 20
	}
	if c.Device == "" {
		c.Device = "1"
	}
	switch strings.ToLower(c.Encoder) {
	case "", EncoderLibx264, "x264":
		c.Encoder = EncoderLibx264
	case EncoderVideoToolbox, "h264_videotoolbox", "vt":
		c.Encoder = EncoderVideoToolbox
	default:
		c.Encoder = EncoderLibx264
	}
	if c.Encoder == EncoderVideoToolbox && runtime.GOOS != "darwin" {
		c.Encoder = EncoderLibx264
	}
	return c
}

// Run starts ffmpeg and forwards each access unit to write. It returns when
// ffmpeg exits or ctx is cancelled.
func (s *FFmpegSource) Run(ctx context.Context, write func(media.Sample) error) error {
	cfg := s.Config.normalized()
	args := cfg.Args()
	cmd := exec.CommandContext(ctx, cfg.FFmpeg, args...)
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
	slog.Info("ffmpeg started", "encoder", cfg.Encoder, "args", args)

	go logFFmpegStderr(stderr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- pumpH264(stdout, cfg.FPS, write)
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

func logFFmpegStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		slog.Warn("ffmpeg", "stderr", line)
	}
}

// Args builds the ffmpeg command line for the current OS.
func (c Config) Args() []string {
	c = c.normalized()
	fps := strconv.Itoa(c.FPS)
	args := []string{"-hide_banner", "-loglevel", "warning", "-fflags", "+nobuffer+genpts"}

	switch runtime.GOOS {
	case "darwin":
		args = append(args,
			"-f", "avfoundation",
			"-capture_cursor", c.captureCursorArg(),
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

	args = append(args,
		"-an",
		"-vf", c.scaleFilter(),
		"-pix_fmt", "yuv420p",
		"-r", fps,
	)
	args = append(args, c.encoderArgs(fps)...)
	args = append(args, "-f", "h264", "pipe:1")
	return args
}

func (c Config) encoderArgs(fps string) []string {
	switch c.Encoder {
	case EncoderVideoToolbox:
		// Hardware path on macOS. Still Annex-B + dump_extra so Chrome sees
		// SPS/PPS with keyframes. Falls back to software if VT rejects the
		// profile (`-allow_sw 1`).
		return []string{
			"-c:v", "h264_videotoolbox",
			"-profile:v", "baseline",
			"-allow_sw", "1",
			"-realtime", "1",
			"-bf", "0",
			"-g", fps,
			"-b:v", "4M",
			"-maxrate", "6M",
			"-bufsize", "2M",
			"-bsf:v", "dump_extra",
		}
	default:
		// Constrained-baseline, one slice per picture, headers on every IDR.
		return []string{
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-level", "3.1",
			"-bf", "0",
			"-g", fps,
			"-keyint_min", fps,
			"-sc_threshold", "0",
			"-x264-params", "repeat-headers=1:annexb=1:aud=1:bframes=0:scenecut=0:sliced-threads=0:threads=1:cabac=0:8x8dct=0",
			"-bsf:v", "dump_extra",
		}
	}
}

func (c Config) scaleFilter() string {
	w, h := even(c.Width), even(c.Height)
	switch {
	case w == 0 && h == 0:
		return "scale=trunc(iw/2)*2:trunc(ih/2)*2,setsar=1,format=yuv420p"
	case w == 0:
		return fmt.Sprintf("scale=-2:%d,setsar=1,format=yuv420p", h)
	case h == 0:
		return fmt.Sprintf("scale=%d:-2,setsar=1,format=yuv420p", w)
	default:
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1,format=yuv420p",
			w, h, w, h,
		)
	}
}

func (c Config) captureCursorArg() string {
	if c.CaptureCursor {
		return "1"
	}
	return "0"
}

func even(n int) int {
	if n <= 0 {
		return 0
	}
	return n - n%2
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

	var b auBuilder
	emit := func(au []byte) error {
		return write(media.Sample{Data: au, Duration: dur})
	}

	for {
		nal, err := reader.NextNAL()
		if err != nil {
			if err == io.EOF {
				if au, ok := b.Flush(); ok {
					if werr := emit(au); werr != nil {
						return werr
					}
				}
				return io.EOF
			}
			return err
		}
		if au, ok := b.Push(nal.Data); ok {
			if err := emit(au); err != nil {
				return err
			}
		}
	}
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
