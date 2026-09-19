//go:build darwin && cgo

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

void LSCRequestPerms(void) {
	if (&CGRequestScreenCaptureAccess != NULL) {
		CGRequestScreenCaptureAccess();
	}
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *values[] = { kCFBooleanTrue };
	CFDictionaryRef opts = CFDictionaryCreate(
		kCFAllocatorDefault, keys, values, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	AXIsProcessTrustedWithOptions(opts);
	if (opts) {
		CFRelease(opts);
	}
}
*/
import "C"

// RequestPermissions shows the system prompts for Screen Recording and Accessibility.
func RequestPermissions() {
	C.LSCRequestPerms()
}
