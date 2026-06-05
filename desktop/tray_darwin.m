#import <Cocoa/Cocoa.h>
#include "tray_darwin.h"

// Callback exported from Go (see _cgo_export.h once cgo compiles).
extern void viorTrayMenuClicked(int tag);

@interface ViorTrayTarget : NSObject
@property (nonatomic, strong) NSStatusItem *item;
@property (nonatomic, strong) NSMenuItem   *statusEntry;
@property (nonatomic, strong) NSMenuItem   *showEntry;
@property (nonatomic, strong) NSMenuItem   *hideEntry;
@property (nonatomic, strong) NSMenuItem   *startEntry;
@property (nonatomic, strong) NSMenuItem   *stopEntry;
- (void)onMenu:(id)sender;
@end

@implementation ViorTrayTarget
- (void)onMenu:(id)sender {
    NSMenuItem *m = (NSMenuItem *)sender;
    viorTrayMenuClicked((int)m.tag);
}
@end

static ViorTrayTarget *_viorTray = nil;

static NSImage *buildTrayImage(void) {
    CGFloat side = 22.0;
    NSImage *img = [[NSImage alloc] initWithSize:NSMakeSize(side, side)];
    [img setTemplate:YES];
    [img lockFocus];
    [[NSColor blackColor] setStroke];
    [[NSColor blackColor] setFill];

    NSBezierPath *mon = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(2, 7, 14, 9) xRadius:1.5 yRadius:1.5];
    [mon setLineWidth:1.4];
    [mon stroke];

    NSRectFill(NSMakeRect(8, 5, 2, 2));
    NSBezierPath *base = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(5, 4, 8, 1.2) xRadius:0.6 yRadius:0.6];
    [base fill];

    NSBezierPath *phoneOuter = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(12, 8, 7, 11) xRadius:1.6 yRadius:1.6];
    [phoneOuter fill];
    [[NSColor whiteColor] setFill];
    NSBezierPath *phoneScreen = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(13, 10, 5, 7) xRadius:0.8 yRadius:0.8];
    [phoneScreen fill];

    [img unlockFocus];
    return img;
}

void viorTrayInstall(void) {
    if (_viorTray != nil) return;
    _viorTray = [[ViorTrayTarget alloc] init];

    dispatch_async(dispatch_get_main_queue(), ^{
        NSStatusBar *bar = [NSStatusBar systemStatusBar];
        _viorTray.item = [bar statusItemWithLength:NSVariableStatusItemLength];
        _viorTray.item.button.image = buildTrayImage();
        _viorTray.item.button.toolTip = @"Vior";

        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Vior"];

        _viorTray.statusEntry = [[NSMenuItem alloc] initWithTitle:@"Server: stopped" action:nil keyEquivalent:@""];
        _viorTray.statusEntry.tag = 6;
        _viorTray.statusEntry.enabled = NO;
        [menu addItem:_viorTray.statusEntry];
        [menu addItem:[NSMenuItem separatorItem]];

        _viorTray.showEntry = [[NSMenuItem alloc] initWithTitle:@"Show Vior" action:@selector(onMenu:) keyEquivalent:@""];
        _viorTray.showEntry.target = _viorTray; _viorTray.showEntry.tag = 1;
        [menu addItem:_viorTray.showEntry];

        _viorTray.hideEntry = [[NSMenuItem alloc] initWithTitle:@"Hide Window" action:@selector(onMenu:) keyEquivalent:@""];
        _viorTray.hideEntry.target = _viorTray; _viorTray.hideEntry.tag = 2;
        [menu addItem:_viorTray.hideEntry];

        [menu addItem:[NSMenuItem separatorItem]];

        _viorTray.startEntry = [[NSMenuItem alloc] initWithTitle:@"Start Server" action:@selector(onMenu:) keyEquivalent:@""];
        _viorTray.startEntry.target = _viorTray; _viorTray.startEntry.tag = 3;
        [menu addItem:_viorTray.startEntry];

        _viorTray.stopEntry = [[NSMenuItem alloc] initWithTitle:@"Stop Server" action:@selector(onMenu:) keyEquivalent:@""];
        _viorTray.stopEntry.target = _viorTray; _viorTray.stopEntry.tag = 4;
        [menu addItem:_viorTray.stopEntry];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *q = [[NSMenuItem alloc] initWithTitle:@"Quit Vior" action:@selector(onMenu:) keyEquivalent:@"q"];
        q.target = _viorTray; q.tag = 5;
        [menu addItem:q];

        _viorTray.item.menu = menu;
    });
}

void viorTrayUninstall(void) {
    if (_viorTray == nil) return;
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_viorTray.item != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:_viorTray.item];
            _viorTray.item = nil;
        }
        _viorTray = nil;
    });
}

void viorTraySetStatus(const char *text, int running) {
    if (_viorTray == nil) return;
    NSString *t = [NSString stringWithUTF8String:text];
    dispatch_async(dispatch_get_main_queue(), ^{
        _viorTray.statusEntry.title = t;
        _viorTray.startEntry.enabled = !running;
        _viorTray.stopEntry.enabled  =  running;
    });
}
