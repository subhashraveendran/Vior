//go:build darwin

package capture

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation

#include <CoreGraphics/CoreGraphics.h>
#include <dlfcn.h>

// Function pointer type for the obsoleted CGDisplayCreateImage.
typedef CGImageRef (*CGDisplayCreateImageFunc)(CGDirectDisplayID);

// capturePixelRect captures a rect from a display using runtime-loaded CGDisplayCreateImage
// (obsoleted in macOS 15 SDK but still present in the framework).
static int capturePixelRect(CGDirectDisplayID displayID, CGRect targetRect,
                            unsigned char *outBuf, size_t outWidth, size_t outHeight, size_t stride) {

	static CGDisplayCreateImageFunc createImage = NULL;
	if (!createImage) {
		createImage = (CGDisplayCreateImageFunc)dlsym(RTLD_DEFAULT, "CGDisplayCreateImage");
	}
	if (!createImage) return -1;

	CGImageRef fullImg = createImage(displayID);
	if (!fullImg) return -1;

	size_t imgW = CGImageGetWidth(fullImg);
	size_t imgH = CGImageGetHeight(fullImg);

	CGFloat x = targetRect.origin.x; if (x < 0) x = 0;
	CGFloat y = targetRect.origin.y; if (y < 0) y = 0;
	CGFloat w = targetRect.size.width;
	CGFloat h = targetRect.size.height;
	if (x + w > (CGFloat)imgW) w = (CGFloat)imgW - x;
	if (y + h > (CGFloat)imgH) h = (CGFloat)imgH - y;
	if (w <= 0 || h <= 0) { CGImageRelease(fullImg); return -1; }

	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	if (!cs) { CGImageRelease(fullImg); return -1; }

	CGContextRef ctx = CGBitmapContextCreate(outBuf, outWidth, outHeight, 8, stride, cs,
	                                         (CGBitmapInfo)kCGImageAlphaNoneSkipFirst);
	CGColorSpaceRelease(cs);
	if (!ctx) { CGImageRelease(fullImg); return -1; }

	CGRect crop = CGRectMake(x, y, w, h);
	CGImageRef cropped = CGImageCreateWithImageInRect(fullImg, crop);
	if (cropped) {
		CGContextDrawImage(ctx, CGRectMake(0, 0, outWidth, outHeight), cropped);
		CGImageRelease(cropped);
	}

	CGContextRelease(ctx);
	CGImageRelease(fullImg);
	return 0;
}

// Forward declaration.
static CGDirectDisplayID getCGDisplayID(int displayIndex);

static void getDisplayPixelSize(int displayIndex, int *w, int *h) {
	*w = 0; *h = 0;
	CGDirectDisplayID id = getCGDisplayID(displayIndex);
	if (id == 0) return;
	CGDisplayModeRef mode = CGDisplayCopyDisplayMode(id);
	if (!mode) return;
	*w = (int)CGDisplayModeGetPixelWidth(mode);
	*h = (int)CGDisplayModeGetPixelHeight(mode);
	CGDisplayModeRelease(mode);
}

static CGDirectDisplayID getCGDisplayID(int displayIndex) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	if (count == 0) return 0;
	CGDirectDisplayID *ids = (CGDirectDisplayID *)malloc(sizeof(CGDirectDisplayID) * count);
	if (!ids) return 0;
	CGGetActiveDisplayList(count, ids, &count);
	CGDirectDisplayID result = 0;
	if ((uint32_t)displayIndex < count) {
		result = ids[displayIndex];
	}
	free(ids);
	return result;
}

// mirrorDisplay mirrors one display to another. master=0 to unmirror (extend).
static int mirrorDisplay(int sourceDisplayIndex, int targetDisplayIndex) {
	CGDirectDisplayID source = getCGDisplayID(sourceDisplayIndex);
	CGDirectDisplayID target = getCGDisplayID(targetDisplayIndex);
	if (source == 0 || target == 0) return -1;

	CGDisplayConfigRef config;
	if (CGBeginDisplayConfiguration(&config) != kCGErrorSuccess) return -1;

	CGConfigureDisplayMirrorOfDisplay(config, source, target);

	if (CGCompleteDisplayConfiguration(config, kCGConfigurePermanently) != kCGErrorSuccess) {
		CGCancelDisplayConfiguration(config);
		return -1;
	}
	return 0;
}

// unmirrorDisplay removes a display from any mirror set (puts it in extend mode).
static int unmirrorDisplay(int displayIndex) {
	CGDirectDisplayID display = getCGDisplayID(displayIndex);
	if (display == 0) return -1;

	CGDisplayConfigRef config;
	if (CGBeginDisplayConfiguration(&config) != kCGErrorSuccess) return -1;

	CGConfigureDisplayMirrorOfDisplay(config, display, kCGNullDirectDisplay);

	if (CGCompleteDisplayConfiguration(config, kCGConfigurePermanently) != kCGErrorSuccess) {
		CGCancelDisplayConfiguration(config);
		return -1;
	}
	return 0;
}

// isDisplayMirrored returns 1 if the display is mirroring another, 0 if extend.
static int isDisplayMirrored(int displayIndex) {
	CGDirectDisplayID id = getCGDisplayID(displayIndex);
	if (id == 0) return -1;
	return CGDisplayIsInMirrorSet(id) ? 1 : 0;
}
// checkScreenRecordingPermission tries to capture a tiny frame from the main display.
// Returns 0 if permission granted, -1 if denied.
static int checkScreenRecordingPermission(void) {
	static CGDisplayCreateImageFunc createImage = NULL;
	if (!createImage) {
		createImage = (CGDisplayCreateImageFunc)dlsym(RTLD_DEFAULT, "CGDisplayCreateImage");
	}
	if (!createImage) return -1;
	CGImageRef img = createImage(CGMainDisplayID());
	if (!img) return -1;
	CGImageRelease(img);
	return 0;
}

// findDisplayIndexByID returns the CGGetActiveDisplayList index for a CGDirectDisplayID.
// Returns -1 if not found.
static int findDisplayIndexByID(CGDirectDisplayID targetID) {
	uint32_t count = 0;
	CGGetActiveDisplayList(0, NULL, &count);
	if (count == 0) return -1;
	CGDirectDisplayID *ids = (CGDirectDisplayID *)malloc(sizeof(CGDirectDisplayID) * count);
	if (!ids) return -1;
	CGGetActiveDisplayList(count, ids, &count);

	for (uint32_t i = 0; i < count; i++) {
		if (ids[i] == targetID) { free(ids); return (int)i; }
	}

	free(ids);
	return -1;
}
*/
import "C"
import (
	"fmt"
	"image"
	"unsafe"

	"github.com/kbinani/screenshot"
)

func captureDisplayRaw(displayID uint32, bounds image.Rectangle) ([]byte, int, error) {
	w, h := bounds.Dx(), bounds.Dy()
	stride := w * 4
	buf := make([]byte, h*stride)
	rc := C.capturePixelRect(C.CGDirectDisplayID(displayID),
		C.CGRectMake(C.CGFloat(bounds.Min.X), C.CGFloat(bounds.Min.Y), C.CGFloat(w), C.CGFloat(h)),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.size_t(w), C.size_t(h), C.size_t(stride))
	if rc != 0 {
		return nil, 0, fmt.Errorf("capture failed for display %d", displayID)
	}
	return buf, stride, nil
}

func captureToRGBA(displayID uint32, bounds image.Rectangle) (*image.RGBA, error) {
	raw, stride, err := captureDisplayRaw(displayID, bounds)
	if err != nil {
		return nil, err
	}
	w, h := bounds.Dx(), bounds.Dy()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := y * stride
		dst := y * img.Stride
		for x := 0; x < w; x++ {
			s := src + x*4
			d := dst + x*4
			img.Pix[d] = raw[s+1]
			img.Pix[d+1] = raw[s+2]
			img.Pix[d+2] = raw[s+3]
			img.Pix[d+3] = 255
		}
	}
	return img, nil
}

func captureImagePlatform(displayIndex int, bounds image.Rectangle) (*image.RGBA, error) {
	id := C.getCGDisplayID(C.int(displayIndex))
	if id == 0 {
		return screenshot.CaptureRect(bounds)
	}
	return captureToRGBA(uint32(id), bounds)
}

func getPixelSize(displayIndex int) (int, int) {
	var w, h C.int
	C.getDisplayPixelSize(C.int(displayIndex), &w, &h)
	return int(w), int(h)
}

// MirrorDisplay mirrors sourceDisplay to targetDisplay (both are vior indices).
// Returns nil on success.
func MirrorDisplay(sourceDisplayIndex, targetDisplayIndex int) error {
	if C.mirrorDisplay(C.int(sourceDisplayIndex), C.int(targetDisplayIndex)) != 0 {
		return fmt.Errorf("mirror failed: display %d → %d", sourceDisplayIndex, targetDisplayIndex)
	}
	return nil
}

// UnmirrorDisplay puts a display back to extend mode.
func UnmirrorDisplay(displayIndex int) error {
	if C.unmirrorDisplay(C.int(displayIndex)) != 0 {
		return fmt.Errorf("unmirror failed for display %d", displayIndex)
	}
	return nil
}

// IsMirrored returns true if the display is currently mirroring another.
func IsMirrored(displayIndex int) (bool, error) {
	r := C.isDisplayMirrored(C.int(displayIndex))
	if r < 0 {
		return false, fmt.Errorf("failed to check mirror status for display %d", displayIndex)
	}
	return r == 1, nil
}

// CheckScreenRecordingPermission verifies macOS screen recording permission.
// Returns nil if granted, error if denied.
func CheckScreenRecordingPermission() error {
	if C.checkScreenRecordingPermission() != 0 {
		return fmt.Errorf("screen recording permission denied — open System Settings > Privacy & Security > Screen Recording and enable Vior")
	}
	return nil
}

// FindDisplayIndexByID returns the capture-compatible display index for a CGDirectDisplayID.
// Returns -1 if the display is not found.
func FindDisplayIndexByID(displayID uint32) int {
	return int(C.findDisplayIndexByID(C.CGDirectDisplayID(displayID)))
}
