//go:build !darwin || !cgo

package input

// RequestPermissions is a no-op off macOS / without CGO.
func RequestPermissions() {}
