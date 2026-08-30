//go:build darwin

package statusbar

/*
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
void hey_codex_statusbar_run(void);
void hey_codex_statusbar_stop(void);
void hey_codex_statusbar_set(const char *state);
*/
import "C"

import "unsafe"

// Bar owns the menu-bar item and must run on the application's main thread.
type Bar struct{}

func New() *Bar { return &Bar{} }

func (b *Bar) Run() { C.hey_codex_statusbar_run() }

func (b *Bar) Stop() { C.hey_codex_statusbar_stop() }

func (b *Bar) Set(state string) {
	cstate := C.CString(state)
	defer C.free(unsafe.Pointer(cstate))
	C.hey_codex_statusbar_set(cstate)
}
