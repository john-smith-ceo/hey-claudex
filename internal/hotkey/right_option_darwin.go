//go:build darwin

package hotkey

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdbool.h>
#include <stdint.h>

extern void goRightOptionEvent(int down);
static CFMachPortRef globalRightOptionTap = NULL;

static CGEventRef rightOptionTap(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
	if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
		if (globalRightOptionTap != NULL) CGEventTapEnable(globalRightOptionTap, true);
		return event;
	}
	if (type != kCGEventFlagsChanged || CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode) != 61) {
		return event;
	}
	CGEventFlags flags = CGEventGetFlags(event);
	goRightOptionEvent((flags & kCGEventFlagMaskAlternate) != 0);
	return NULL;
}

static CFMachPortRef createRightOptionTap(void) {
	CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionDefault,
		CGEventMaskBit(kCGEventFlagsChanged), rightOptionTap, NULL);
	if (tap == NULL) return NULL;
	globalRightOptionTap = tap;
	return tap;
}

static int rightOptionTapIsNull(CFMachPortRef tap) { return tap == NULL; }

static void runRightOptionTap(CFMachPortRef tap) {
	CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
	CFRunLoopAddSource(CFRunLoopGetCurrent(), source, kCFRunLoopCommonModes);
	CGEventTapEnable(tap, true);
	CFRunLoopRun();
	CFRelease(source);
}

static void stopRightOptionTap(void) { CFRunLoopStop(CFRunLoopGetMain()); }
*/
import "C"

import (
	"context"
	"errors"
	"sync"
)

type rightOption struct{}

var (
	eventsMu sync.Mutex
	events   chan Event
)

func NewRightOption() Listener { return rightOption{} }

func (rightOption) Start(ctx context.Context) (<-chan Event, error) {
	tap := C.createRightOptionTap()
	if C.rightOptionTapIsNull(tap) != 0 {
		return nil, errors.New("cannot install Right Option event tap; grant Accessibility permission and retry")
	}
	ch := make(chan Event, 8)
	eventsMu.Lock()
	events = ch
	eventsMu.Unlock()
	go func() {
		<-ctx.Done()
		eventsMu.Lock()
		if events == ch {
			close(ch)
			events = nil
		}
		eventsMu.Unlock()
		// Stopping the CFRunLoop from another thread needs a runloop reference; process exit on Ctrl+C is safe.
	}()
	go func() { C.runRightOptionTap(tap) }()
	return ch, nil
}

//export goRightOptionEvent
func goRightOptionEvent(down C.int) {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	if events == nil {
		return
	}
	select {
	case events <- Event{Down: down != 0}:
	default:
	}
}
