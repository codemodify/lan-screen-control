#ifndef LSC_PASTEBOARD_DARWIN_H
#define LSC_PASTEBOARD_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

/* Current NSPasteboard changeCount (increments on every write). */
int LSCPasteboardChangeCount(void);

/* UTF-8 copy of NSPasteboardTypeString, or NULL if absent. Caller frees. */
char *LSCPasteboardReadString(void);

/* Replace the general pasteboard with a single plain-text item. 1 = ok. */
int LSCPasteboardWriteString(const char *utf8);

void LSCPasteboardFree(char *p);

#ifdef __cplusplus
}
#endif

#endif
