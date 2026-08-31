//go:build !darwin && !linux

package hotkey

import (
	"context"
	"errors"
)

type unsupported struct{}

func GlobalSupported() bool { return false }

func New(Key) (Listener, error) { return unsupported{}, nil }

func (unsupported) Start(context.Context) (<-chan Event, error) {
	return nil, errors.New("global hotkeys are supported only on macOS and Linux")
}
