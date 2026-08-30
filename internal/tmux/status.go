package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Status owns only the status line of one hey-codex tmux session. It never
// reads or changes the user's global tmux configuration.
type Status struct {
	session string
	flags   string
	mode    string
}

func NewStatus(session string, codexArgs []string, mode string) (*Status, error) {
	if strings.TrimSpace(session) == "" {
		return nil, fmt.Errorf("tmux session is empty")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux not found")
	}
	return &Status{session: session, flags: cleanStatusText(strings.Join(codexArgs, " ")), mode: mode}, nil
}

func (s *Status) Configure(ctx context.Context) error {
	options := [][2]string{
		{"status", "on"},
		{"status-style", "bg=#BCD7EF,fg=#17324D"},
		{"status-left-length", "500"},
		{"status-right", ""},
		{"window-status-format", ""},
		{"window-status-current-format", ""},
		{"status-justify", "left"},
	}
	for _, option := range options {
		if err := s.set(ctx, option[0], option[1]); err != nil {
			return err
		}
	}
	return s.Set(ctx, "idle")
}

// Set renders the one meaningful status message instead of tmux's default
// window list, which would expose the internal voice listener window.
func (s *Status) Set(ctx context.Context, state string) error {
	message := "<speech-to-text " + s.label(state) + ">"
	left := "#[fg=#17324D,bold]hey-codex#[fg=#17324D]: codex"
	if s.flags != "" {
		left += " #[fg=#315B82]<" + s.flags + ">"
	}
	left += " #[fg=#315B82]" + message
	return s.set(ctx, "status-left", left)
}

func (s *Status) label(state string) string {
	switch state {
	case "recording":
		return "rec…"
	case "transcribing":
		return "transcribe…"
	case "pasted":
		return "done"
	case "error":
		return "error"
	default:
		return "mode:" + s.mode
	}
}

func (s *Status) set(ctx context.Context, option, value string) error {
	output, err := exec.CommandContext(ctx, "tmux", "set-option", "-t", s.session, option, value).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set tmux %s: %s", option, strings.TrimSpace(string(output)))
	}
	return nil
}

// tmux evaluates # constructs inside status strings. Keep command-line input
// visible but inert, even if somebody passes unusual Codex arguments.
func cleanStatusText(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		if r == '#' {
			return '＃'
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}
