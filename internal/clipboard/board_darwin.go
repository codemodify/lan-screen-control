//go:build darwin && cgo

package clipboard

import (
	"errors"
)

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include "pasteboard_darwin.h"
*/
import "C"

var errWriteFailed = errors.New("NSPasteboard write failed")

type darwinBoard struct{}

// New returns the macOS general pasteboard (NSPasteboard).
// Setting text via AppKit does not require extra TCC beyond the Accessibility
// permission already needed for key injection.
func New() Board {
	return darwinBoard{}
}

func (darwinBoard) WriteText(s string) error {
	cs := C.CString(s)
	defer C.LSCPasteboardFree(cs)
	if C.LSCPasteboardWriteString(cs) == 0 {
		return errWriteFailed
	}
	return nil
}

func (darwinBoard) ReadText() (string, bool, error) {
	p := C.LSCPasteboardReadString()
	if p == nil {
		return "", false, nil
	}
	defer C.LSCPasteboardFree(p)
	return C.GoString(p), true, nil
}

func (darwinBoard) ChangeCount() int {
	return int(C.LSCPasteboardChangeCount())
}
