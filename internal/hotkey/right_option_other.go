//go:build !darwin

package hotkey

import (
	"context"
	"errors"
)

type unsupported struct{}

func GlobalSupported() bool { return false }

func NewRightOption() Listener { return unsupported{} }
func (unsupported) Start(context.Context) (<-chan Event, error) {
	return nil, errors.New("Right Option hotkey is supported only on macOS")
}
