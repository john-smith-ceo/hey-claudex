package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/john-smith-ceo/hey-claudex/internal/hotkey"
	"github.com/john-smith-ceo/hey-claudex/internal/record"
	"github.com/john-smith-ceo/hey-claudex/internal/tmux"
	"github.com/john-smith-ceo/hey-claudex/internal/transcribe"
)

type Mode string

const (
	Tap  Mode = "tap"
	Push Mode = "push"
)

type Config struct {
	Mode       Mode
	Silence    time.Duration
	Device     string
	APIKey     string
	Log        io.Writer
	State      func(string)
	TmuxTarget string
	Key        hotkey.Key

	// BusyFile is touched while the microphone is recording. Anything that
	// speaks out loud can watch it and hold its tongue: a voice-over talking
	// into the same room ends up inside the recording.
	BusyFile string
	// OnRecord runs once when recording starts — to silence whatever is
	// already speaking, which a file alone cannot do.
	OnRecord string
}

type Bridge struct {
	config     Config
	hotkey     hotkey.Listener
	recorder   record.Recorder
	transcribe transcribe.Client
	sender     *tmux.Sender

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
	if config.Mode == Push && !config.Key.SupportsPush() {
		return nil, fmt.Errorf("%s is used while typing, so hold-to-talk cannot be told apart from ordinary use; run it with --mode tap", config.Key.Name)
	}
	listener, err := hotkey.New(config.Key)
	if err != nil {
		return nil, err
	}
	sender, err := tmux.New(config.TmuxTarget)
	if err != nil {
		return nil, err
	}
	if err := sender.Check(context.Background()); err != nil {
		return nil, err
	}
	return &Bridge{
		config:     config,
		hotkey:     listener,
		recorder:   record.NewFFmpeg(config.Device, config.Silence),
		transcribe: transcribe.NewOpenAI(config.APIKey),
		sender:     sender,
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
	b.state("recording")
	b.markBusy()
	go func() {
		startedAt := time.Now()
		file, err := b.recorder.Record(ctx, autoStop)
		recorded := time.Since(startedAt)
		b.clearBusy()
		b.mu.Lock()
		discard := b.session != session
		if b.session == session {
			b.recording, b.cancel = false, nil
		}
		b.mu.Unlock()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				b.fail("recording failed:", err)
			}
			return
		}
		defer os.Remove(file)
		if discard {
			b.state("idle")
			return
		}
		fmt.Fprintln(b.config.Log, "transcribing")
		b.state("transcribing")
		transcribeStart := time.Now()
		text, err := b.transcribe.Transcribe(context.Background(), file)
		transcribed := time.Since(transcribeStart)
		if err != nil {
			b.fail("transcription failed:", err)
			return
		}
		deliverStart := time.Now()
		if err := b.sender.Send(context.Background(), text); err != nil {
			b.fail("tmux delivery failed:", err)
			return
		}
		delivered := time.Since(deliverStart)
		// Where the wait actually goes is a question that gets asked often and
		// answered by feel; these three numbers answer it with facts.
		fmt.Fprintf(b.config.Log, "timing: record %s, transcribe %s, deliver %s, chars %d\n",
			round(recorded), round(transcribed), round(delivered), len(text))
		fmt.Fprintf(b.config.Log, "transcription delivered to tmux %s; review it and press Enter yourself\n", b.sender.Target())
		b.state("pasted")
		time.AfterFunc(1500*time.Millisecond, func() { b.state("idle") })
	}()
}

func (b *Bridge) stop(transcribe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.recording || b.cancel == nil {
		return
	}
	fmt.Fprintln(b.config.Log, "recording stopped")
	b.state("transcribing")
	b.cancel()
	if !transcribe {
		b.session++
		b.recording, b.cancel = false, nil
	}
}

// fail reports a failure and then lets the indicator settle back to idle. A
// status line borrowed from the user must not keep a red dot forever.
func (b *Bridge) fail(message string, err error) {
	fmt.Fprintln(b.config.Log, message, err)
	b.state("error")
	time.AfterFunc(4*time.Second, func() { b.state("idle") })
}

// markBusy raises the microphone flag and hushes anything already speaking.
// The flag carries no content: watchers read its presence and its age, and a
// recording never runs long enough to look abandoned.
func (b *Bridge) markBusy() {
	if b.config.BusyFile != "" {
		if err := os.MkdirAll(filepath.Dir(b.config.BusyFile), 0o755); err == nil {
			if file, err := os.Create(b.config.BusyFile); err == nil {
				_ = file.Close()
			}
		}
	}
	if b.config.OnRecord == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "sh", "-c", b.config.OnRecord).CombinedOutput(); err != nil {
		fmt.Fprintf(b.config.Log, "on-record command failed: %v: %s\n", err, strings.TrimSpace(string(output)))
	}
}

func (b *Bridge) clearBusy() {
	if b.config.BusyFile == "" {
		return
	}
	if err := os.Remove(b.config.BusyFile); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(b.config.Log, "clear microphone flag:", err)
	}
}

func round(d time.Duration) time.Duration { return d.Round(10 * time.Millisecond) }

func (b *Bridge) state(value string) {
	if b.config.State != nil {
		b.config.State(value)
	}
}
