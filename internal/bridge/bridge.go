package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/johnsmith/hey-codex/internal/hotkey"
	"github.com/johnsmith/hey-codex/internal/paste"
	"github.com/johnsmith/hey-codex/internal/record"
	"github.com/johnsmith/hey-codex/internal/transcribe"
)

type Mode string

const (
	Tap  Mode = "tap"
	Push Mode = "push"
)

type Config struct {
	Mode    Mode
	Silence time.Duration
	Device  string
	APIKey  string
	Log     io.Writer
}

type Bridge struct {
	config     Config
	hotkey     hotkey.Listener
	recorder   record.Recorder
	transcribe transcribe.Client
	paste      paste.Paster

	mu        sync.Mutex
	recording bool
	cancel    context.CancelFunc
	session   uint64
}

func New(config Config) (*Bridge, error) {
	if config.Log == nil {
		config.Log = io.Discard
	}
	if config.Mode != Tap && config.Mode != Push {
		return nil, fmt.Errorf("unsupported mode %q", config.Mode)
	}
	if config.Silence <= 0 {
		return nil, errors.New("silence timeout must be positive")
	}
	return &Bridge{
		config:     config,
		hotkey:     hotkey.NewRightOption(),
		recorder:   record.NewFFmpeg(config.Device, config.Silence),
		transcribe: transcribe.NewOpenAI(config.APIKey),
		paste:      paste.NewMacOS(),
	}, nil
}

func (b *Bridge) Run(parent context.Context) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	events, err := b.hotkey.Start(ctx)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			b.stop(false)
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return errors.New("global hotkey listener stopped")
			}
			b.handle(event)
		}
	}
}

func (b *Bridge) handle(event hotkey.Event) {
	switch b.config.Mode {
	case Push:
		if event.Down {
			b.start(false)
		} else {
			b.stop(true)
		}
	case Tap:
		if event.Down {
			b.mu.Lock()
			running := b.recording
			b.mu.Unlock()
			if running {
				b.stop(true)
			} else {
				b.start(true)
			}
		}
	}
}

func (b *Bridge) start(autoStop bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.recording {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.session++
	session := b.session
	b.recording, b.cancel = true, cancel
	fmt.Fprintln(b.config.Log, "recording started")
	go func() {
		file, err := b.recorder.Record(ctx, autoStop)
		b.mu.Lock()
		discard := b.session != session
		if b.session == session {
			b.recording, b.cancel = false, nil
		}
		b.mu.Unlock()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				fmt.Fprintln(b.config.Log, "recording failed:", err)
			}
			return
		}
		defer os.Remove(file)
		if discard {
			return
		}
		fmt.Fprintln(b.config.Log, "transcribing")
		text, err := b.transcribe.Transcribe(context.Background(), file)
		if err != nil {
			fmt.Fprintln(b.config.Log, "transcription failed:", err)
			return
		}
		if err := b.paste.Paste(text); err != nil {
			fmt.Fprintln(b.config.Log, "paste failed:", err)
			return
		}
		fmt.Fprintln(b.config.Log, "transcription pasted; review it and press Enter yourself")
	}()
}

func (b *Bridge) stop(transcribe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.recording || b.cancel == nil {
		return
	}
	fmt.Fprintln(b.config.Log, "recording stopped")
	b.cancel()
	if !transcribe {
		b.session++
		b.recording, b.cancel = false, nil
	}
}
