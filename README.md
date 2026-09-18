# lan-screen-control

Minimal LAN remote desktop: a **Go server on a Mac** captures the primary display and streams it over **WebRTC**. A browser on Linux (or any WebRTC + H.264 client) shows the screen and sends mouse/keyboard events back for injection.

This is intentionally small. It is not a RustDesk/TeamViewer wrapper. There is no cloud account, no TURN requirement, and **exactly one concurrent viewer**.

## How it works

```
Linux browser  --HTTP POST SDP-->  Go server on Mac :62000
               <--SDP answer-----
               <==== WebRTC H.264 video (LAN host ICE) ====
               ==== WebRTC data channel (mouse/keyboard) ==>
```

**Signaling** is a single HTTP POST of a complete SDP offer (ICE gathering finishes first; no trickle, no WebSocket).

- `POST /api/signal` with `{ "sdp": "...", "type": "offer" }`
- Success: `{ "sdp": "...", "type": "answer" }`
- If a session is already active: **HTTP 409** and `{ "error": "another client is already connected" }`
- `GET /api/status` returns `{ "busy": true|false }`

The same process serves the client at `http://<mac-lan-ip>:62000/`.

**Media:** ffmpeg captures the screen (`avfoundation` on macOS) and encodes **constrained-baseline H.264** (`libx264` by default: `yuv420p`, zerolatency, regular IDRs, `repeat-headers`, no slice threads). Access units (SPS/PPS + every slice of a picture) are assembled before each pion `WriteSample` so Chrome sees one complete frame per RTP timestamp. [pion/webrtc](https://github.com/pion/webrtc) packetizes those Annex-B AUs. On macOS you can try `-encoder videotoolbox` if `libx264` is a problem.

**Input:** the browser posts pointer/keyboard events on a WebRTC data channel. On macOS the server injects them with CoreGraphics (`CGEventPost`). That requires **Accessibility** permission.

**ICE:** host candidates are enough on a LAN. A public STUN server is optional (`-stun`, on by default) and is **not** a TURN server.

## Requirements (Mac host)

- macOS (Intel or Apple Silicon)
- Go 1.24+ (Go 1.22+ also works; the toolchain is downloaded automatically)
- [ffmpeg](https://ffmpeg.org/) with `libx264` (Homebrew: `brew install ffmpeg`)
- Xcode Command Line Tools (CGO / CoreGraphics; enabled by default for a native `go build`)

Check the AVFoundation screen index if capture fails:

```bash
ffmpeg -f avfoundation -list_devices true -i ""
# or: ./server -list-devices
```

The built-in screen device is usually index **1** (`Capture screen 0`). The camera is often `0`. Override with `-device`.

## macOS permissions

Grant these to **the binary you run**, or to **Terminal / iTerm** if you launch it from a shell:

1. **Screen Recording** — System Settings → Privacy & Security → Screen Recording.
   Without this, ffmpeg cannot read the display (blank frames or an authorization error).
2. **Accessibility** — System Settings → Privacy & Security → Accessibility.
   Without this, mouse/keyboard injection is ignored.

After changing permissions, quit and relaunch the server (and the terminal app, if that is what you authorized).

## Build and run on the Mac

```bash
git clone https://github.com/codemodify/lan-screen-control.git
cd lan-screen-control
go build -o server ./cmd/server
./server
```

The process listens on **`0.0.0.0:62000`**.

Useful flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `0.0.0.0:62000` | HTTP bind address |
| `-ffmpeg` | `ffmpeg` | ffmpeg binary |
| `-device` | `1` | AVFoundation video index |
| `-fps` | `20` | Capture frame rate |
| `-width` | `1280` | Encoded canvas width (`0` with `-height 0` = even native) |
| `-height` | `720` | Encoded canvas height (`0` with `-width 0` = even native) |
| `-encoder` | `libx264` | `libx264` (default, WebRTC-friendly) or `videotoolbox` (macOS) |
| `-stun` | `stun:stun.l.google.com:19302` | Optional STUN; empty disables it |
| `-list-devices` | | Print ffmpeg devices and exit |

The default canvas is **1280×720** with aspect-preserving scale + letterbox pad (even dimensions, `yuv420p`). Native Retina sizes often confuse software encoders; use `-width 0 -height 0` only if you need full resolution.

## Connect from a Linux browser

1. Find the Mac’s LAN IP (System Settings → Network, or `ipconfig getifaddr en0`).
2. On Linux, open **Chrome, Edge, or Firefox** (H.264 required) at:

   `http://<mac-lan-ip>:62000/`

3. The page auto-connects, plays the remote screen, and forwards pointer + keyboard events from the video surface.
4. Use **Fullscreen** for a desktop-like layout. Click the video so keystrokes are captured.

Both machines must be on the same LAN (or a routed network that allows TCP 62000 plus the UDP ports WebRTC picks). No cloud relay is used.

## One-client limit

Only **one** WebRTC viewer is allowed. A second browser that posts an offer receives **HTTP 409** and a clear on-page error: *Another client is already connected.* The existing session is left untouched. Closing the first tab (or waiting for ICE failure, ~8s) frees the slot.

## Cross-compile notes

Native build **on the Mac** is the supported path (`CGO` on, so CoreGraphics injection is linked):

```bash
go build -o server ./cmd/server
```

From Linux you can produce a Darwin binary **without** injection:

```bash
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o server-darwin-arm64 ./cmd/server
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o server-darwin-amd64 ./cmd/server
```

That binary still serves the page and can stream if ffmpeg is present, but input injection is disabled until you rebuild on macOS with CGO. A full CGO cross-compile needs a macOS SDK / `osxcross` toolchain.

On Linux, `go build ./cmd/server` succeeds and the server will stream an ffmpeg `testsrc` pattern (so the WebRTC path can be exercised without a Mac).

## Project layout

```
cmd/server/          entrypoint
internal/capture/    ffmpeg screen capture → H.264
internal/input/      macOS CoreGraphics injection
internal/rdp/        single-client WebRTC + HTTP signaling
internal/protocol/   data-channel event schema
web/                 vanilla HTML/JS/CSS client (embedded in the binary)
```

## Known limitations

- **No authentication.** Anyone who can reach `:62000` on your LAN can take the first session and control the Mac. Use a trusted network only (or bind to a VPN interface via `-addr`).
- **Primary display only.** No multi-monitor picker.
- **US-ANSI keycodes.** Physical keys are mapped from `KeyboardEvent.code` to macOS virtual key codes; other layouts may mis-fire punctuation.
- **H.264 only.** The viewer browser must decode H.264 (typical for Chrome/Edge/Firefox). If the picture is a green rectangle with a thin strip of desktop, rebuild with this repo’s access-unit path (do not stream one NAL per sample).
- **ffmpeg is required** on the Mac for capture + encode.
- **Permissions are easy to get wrong.** If the picture is black, check Screen Recording. If the cursor does not move, check Accessibility.
- **No clipboard, file transfer, audio, or multi-user control.**
- A crashed viewer can hold the slot until ICE fails (a few seconds).
- Cursor is drawn by ffmpeg (`-capture_cursor 1`); remote cursor hiding on the Mac is not implemented.

## License

Use and modify as you like for this repository.
