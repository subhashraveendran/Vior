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
void vior_vd_destroy(void);

#ifdef __cplusplus
}
#endif

#endif
