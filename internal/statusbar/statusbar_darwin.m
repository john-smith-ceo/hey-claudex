#import <AppKit/AppKit.h>

static NSStatusItem *heyCodexItem = nil;

static void heyCodexApplyState(NSString *state) {
    if (heyCodexItem == nil) return;
    NSStatusBarButton *button = heyCodexItem.button;
    if ([state isEqualToString:@"recording"]) {
        button.title = @"●";
        button.toolTip = @"hey-codex: recording";
        button.contentTintColor = NSColor.systemRedColor;
    } else if ([state isEqualToString:@"transcribing"]) {
        button.title = @"◌";
        button.toolTip = @"hey-codex: transcribing";
        button.contentTintColor = NSColor.systemOrangeColor;
    } else if ([state isEqualToString:@"pasted"]) {
        button.title = @"✓";
        button.toolTip = @"hey-codex: transcription pasted";
        button.contentTintColor = NSColor.systemGreenColor;
    } else if ([state isEqualToString:@"error"]) {
        button.title = @"!";
        button.toolTip = @"hey-codex: see terminal for error";
        button.contentTintColor = NSColor.systemRedColor;
    } else {
        button.title = @"🎙";
        button.toolTip = @"hey-codex: ready (Right Option)";
        button.contentTintColor = nil;
    }
}

void hey_codex_statusbar_set(const char *value) {
    NSString *state = [NSString stringWithUTF8String:value];
    dispatch_async(dispatch_get_main_queue(), ^{ heyCodexApplyState(state); });
}

void hey_codex_statusbar_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{ [NSApp terminate:nil]; });
}

void hey_codex_statusbar_run(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        heyCodexItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"hey-codex"];
        NSMenuItem *title = [[NSMenuItem alloc] initWithTitle:@"hey-codex — Right Option" action:nil keyEquivalent:@""];
        [title setEnabled:NO];
        [menu addItem:title];
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit hey-codex" action:@selector(terminate:) keyEquivalent:@"q"];
        [menu addItem:quit];
        heyCodexItem.menu = menu;
        heyCodexApplyState(@"idle");
        [NSApp run];
    }
}
