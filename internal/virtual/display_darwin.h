#ifndef VIOR_VIRTUAL_DISPLAY_H
#define VIOR_VIRTUAL_DISPLAY_H

#ifdef __cplusplus
extern "C" {
#endif

// vior_vd_create creates a virtual display with the given pixel dimensions.
// Returns 0 on success, negative error code on failure.
// On success, outDisplayID is set to the CGDirectDisplayID of the new display.
int vior_vd_create(unsigned int width,
                   unsigned int height,
                   double refreshRate,
                   unsigned int *outDisplayID);

// vior_vd_create_hidpi creates a virtual HiDPI display at 2x scale.
// logicalWidth/Height are the logical (points) dimensions; pixel dims are 2x.
int vior_vd_create_hidpi(unsigned int logicalWidth,
                         unsigned int logicalHeight,
                         double refreshRate,
                         unsigned int *outDisplayID);

// vior_vd_destroy tears down the currently managed virtual display.
// The CGVirtualDisplay private API has no formal -invalidate method,
// so teardown = applying an empty settings + releasing the strong ref
// (requires ARC; the cgo CFLAGS enables -fobjc-arc). vior_vd_destroy
// returns the displayID that was torn down (0 if none was active) so
// the Go caller can poll capture.ListDisplays for the ghost.
unsigned int vior_vd_destroy(void);

// vior_vd_current_id returns the displayID of the most recently
// created virtual display, or 0 if none is currently held.
unsigned int vior_vd_current_id(void);

#ifdef __cplusplus
}
#endif

#endif
