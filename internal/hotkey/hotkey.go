package hotkey

import "context"

type Event struct{ Down bool }

type Listener interface {
	Start(context.Context) (<-chan Event, error)
}
