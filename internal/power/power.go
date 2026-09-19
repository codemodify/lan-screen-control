// Package power wakes the host display and holds it awake during a remote session.
//
// The real implementation is macOS + CGO (IOKit / CoreGraphics, with a
// caffeinate fallback). Other GOOS / CGO_ENABLED=0 builds are no-ops so
// Linux and cross-compiled Darwin binaries still compile.
package power
