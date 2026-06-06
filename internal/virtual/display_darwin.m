#include "display_darwin.h"
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>

// Private CGVirtualDisplay API — reverse-engineered, stable since macOS 13.
// See: github.com/w0lfschild/macOS_headers CoreGraphics/CGVirtualDisplay.h

@interface CGVirtualDisplayMode : NSObject
@property (readonly) unsigned int widthInPixels;
@property (readonly) unsigned int heightInPixels;
@property (readonly) double refreshRate;
- (instancetype)initWithWidth:(unsigned int)width
                       height:(unsigned int)height
                  refreshRate:(double)refreshRate;
@end

@interface CGVirtualDisplayDescriptor : NSObject
@property (assign) unsigned int vendorID;
@property (assign) unsigned int productID;
@property (assign) unsigned int serialNum;
@property (copy, nonatomic) NSString *name;
@property (assign) CGSize sizeInMillimeters;
@property (assign) unsigned int maxPixelsWide;
@property (assign) unsigned int maxPixelsHigh;
@property (copy, nonatomic) dispatch_queue_t queue;
@end

@interface CGVirtualDisplaySettings : NSObject
@property (copy, nonatomic) NSArray<CGVirtualDisplayMode *> *modes;
@end

@interface CGVirtualDisplay : NSObject
@property (readonly, nonatomic) unsigned int displayID;
- (instancetype)initWithDescriptor:(CGVirtualDisplayDescriptor *)descriptor;
- (BOOL)applySettings:(CGVirtualDisplaySettings *)settings;
@end

static CGVirtualDisplay *_viorDisplay = nil;
static unsigned int _viorDisplayID = 0;
static unsigned int _nextSerial = 100;

int vior_vd_create(unsigned int width, unsigned int height, double refreshRate, unsigned int *outDisplayID) {
    @autoreleasepool {
        CGVirtualDisplayDescriptor *desc = [[CGVirtualDisplayDescriptor alloc] init];
        if (!desc) return -1;

        desc.vendorID     = 0x1234;
        desc.productID    = 0x5678;
        desc.serialNum    = _nextSerial++;
        desc.name         = [NSString stringWithFormat:@"Vior Virtual %dx%d", width, height];
        desc.sizeInMillimeters = CGSizeMake(597, 336);
        desc.maxPixelsWide  = width;
        desc.maxPixelsHigh  = height;
        desc.queue          = dispatch_get_main_queue();

        CGVirtualDisplayMode *mode = [[CGVirtualDisplayMode alloc] initWithWidth:width
                                                                          height:height
                                                                     refreshRate:refreshRate];
        if (!mode) return -2;

        CGVirtualDisplaySettings *settings = [[CGVirtualDisplaySettings alloc] init];
        settings.modes = @[mode];

        CGVirtualDisplay *display = [[CGVirtualDisplay alloc] initWithDescriptor:desc];
        if (!display) return -3;

        BOOL ok = [display applySettings:settings];
        if (!ok) {
            return -4;
        }

        _viorDisplay = display;
        _viorDisplayID = display.displayID;
        *outDisplayID = _viorDisplayID;
        return 0;
    }
}

unsigned int vior_vd_destroy(void) {
    unsigned int gone = _viorDisplayID;
    @autoreleasepool {
        if (_viorDisplay != nil) {
            // Tell the system to drop all active modes BEFORE we
            // release the strong ref. Without this pass macOS often
            // keeps the display alive in CGGetActiveDisplayList for
            // several seconds after dealloc — the "ghost display"
            // users see when they re-pair quickly.
            CGVirtualDisplaySettings *empty = [[CGVirtualDisplaySettings alloc] init];
            empty.modes = @[];
            @try {
                [_viorDisplay applySettings:empty];
            } @catch (NSException *e) {
                // Some macOS versions raise on empty modes — fine, we
                // still release below and let the dealloc path do it.
            }
        }
        // ARC: assigning nil to the strong ref releases. The dispatch_after
        // gives the main runloop a tick to drain the CGDisplay reconfig
        // callbacks before the Go caller polls for the ghost.
        _viorDisplay = nil;
        _viorDisplayID = 0;
        _nextSerial = 100;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.05 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            // No-op: scheduling something on the main queue forces the
            // queue to wake up and process any pending display-removed
            // callbacks the system queued during release.
        });
    }
    return gone;
}

unsigned int vior_vd_current_id(void) {
    return _viorDisplayID;
}

int vior_vd_create_hidpi(unsigned int logicalWidth,
                         unsigned int logicalHeight,
                         double refreshRate,
                         unsigned int *outDisplayID) {
    unsigned int pxWidth  = logicalWidth  * 2;
    unsigned int pxHeight = logicalHeight * 2;
    return vior_vd_create(pxWidth, pxHeight, refreshRate, outDisplayID);
}
