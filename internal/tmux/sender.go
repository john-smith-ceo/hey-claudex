package tmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Sender delivers text to one explicit tmux pane. It never sends Enter.
type Sender struct {
	target string
}

func New(target string) (*Sender, error) {
	if strings.TrimSpace(target) == "" {
		return nil, errors.New("tmux target is empty")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, errors.New("tmux not found")
	}
	return &Sender{target: target}, nil
}

func (s *Sender) Target() string { return s.target }

func (s *Sender) Check(ctx context.Context) error {
	output, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", s.target, "#{pane_id}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux target %q is unavailable: %s", s.target, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Sender) Send(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("refusing to send empty transcription")
	}
	buffer, err := bufferName()
	if err != nil {
		return err
	}
	load := exec.CommandContext(ctx, "tmux", "load-buffer", "-b", buffer, "-")
	load.Stdin = strings.NewReader(text)
	if output, err := load.CombinedOutput(); err != nil {
		return fmt.Errorf("load tmux buffer: %s", strings.TrimSpace(string(output)))
	}
	paste := exec.CommandContext(ctx, "tmux", "paste-buffer", "-d", "-b", buffer, "-t", s.target)
	if output, err := paste.CombinedOutput(); err != nil {
		return fmt.Errorf("paste into tmux target %q: %s", s.target, strings.TrimSpace(string(output)))
	}
	return nil
}

func bufferName() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "hey-codex-" + hex.EncodeToString(bytes), nil
}
