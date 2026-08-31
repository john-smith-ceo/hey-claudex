package hotkey

import "context"

type Event struct{ Down bool }

type Listener interface {
	Start(context.Context) (<-chan Event, error)
}

// Validate reports whether this machine can listen for the key at all, so a
// bad choice is refused before anything else is set up.
func Validate(key Key) error {
	_, err := New(key)
	return err
}
