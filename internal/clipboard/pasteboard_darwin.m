//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include "pasteboard_darwin.h"
#include <stdlib.h>
#include <string.h>

int LSCPasteboardChangeCount(void) {
	@autoreleasepool {
		return (int)[[NSPasteboard generalPasteboard] changeCount];
	}
}

char *LSCPasteboardReadString(void) {
	@autoreleasepool {
		NSString *s = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
		if (s == nil) {
			return NULL;
		}
		const char *utf8 = [s UTF8String];
		if (utf8 == NULL) {
			return NULL;
		}
		return strdup(utf8);
	}
}

int LSCPasteboardWriteString(const char *utf8) {
	@autoreleasepool {
		if (utf8 == NULL) {
			utf8 = "";
		}
		NSString *s = [NSString stringWithUTF8String:utf8];
		if (s == nil) {
			s = @"";
		}
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		BOOL ok = [pb setString:s forType:NSPasteboardTypeString];
		return ok ? 1 : 0;
	}
}

void LSCPasteboardFree(char *p) {
	free(p);
}
