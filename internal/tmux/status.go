package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// While hey-codex borrows the status line, the session's own state is parked in
// two user options. They live in tmux rather than in memory so that the value
// survives the listener being killed and can still be restored afterwards.
//
// Two options rather than one, because an empty status-right and an unset one
// look identical but behave differently: an unset option inherits the global
// value, an empty one overrides it with nothing. Restoring the wrong one would
// silently wipe the clock and the pane title.
const (
	wasOption  = "@hey-codex-status-was"
	baseOption = "@hey-codex-status-base"
)

// Status owns the status line of one tmux session. In a session hey-codex
// created it takes the whole line. In a session that already belonged to the
// user it only appends its indicator to the existing status-right and puts the
// original back on the way out: overwriting somebody's own status line is the
// same mistake as launching them a second Codex.
type Status struct {
	session  string
	flags    string
	mode     string
	attached bool
}

func NewStatus(session string, codexArgs []string, mode string, attached bool) (*Status, error) {
	if strings.TrimSpace(session) == "" {
		return nil, fmt.Errorf("tmux session is empty")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux not found")
	}
	return &Status{session: session, flags: cleanStatusText(strings.Join(codexArgs, " ")), mode: mode, attached: attached}, nil
}

func (s *Status) Configure(ctx context.Context) error {
	if s.attached {
		// Save once: a second listener must not overwrite the saved value with
		// a line that already contains an indicator.
		was, err := s.get(ctx, wasOption)
		if err != nil {
			return err
		}
		if was == "" {
			set, err := sessionOptionIsSet(ctx, s.session, "status-right")
			if err != nil {
				return err
			}
			// The base is what the user actually sees, which is the session
			// value when there is one and the global value otherwise.
			base, err := effective(ctx, s.session, "status-right", set)
			if err != nil {
				return err
			}
			state := "unset"
			if set {
				state = "set"
			}
			if err := s.set(ctx, wasOption, state); err != nil {
				return err
			}
			if err := s.set(ctx, baseOption, base); err != nil {
				return err
			}
		}
		return s.Set(ctx, "idle")
	}
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
	if s.attached {
		base, err := s.get(ctx, baseOption)
		if err != nil {
			return err
		}
		message := "#[default]" + indicatorWith(state, "#[default]") + "hey-codex " + s.label(state)
		if strings.TrimSpace(base) != "" {
			message = base + " " + message
		}
		return s.set(ctx, "status-right", message)
	}
	message := "<speech-to-text " + indicator(state) + s.label(state) + ">"
	left := "#[fg=#17324D,bold]hey-codex#[fg=#17324D]: codex"
	if s.flags != "" {
		left += " #[fg=#315B82]<" + s.flags + ">"
	}
	left += " #[fg=#315B82]" + message
	return s.set(ctx, "status-left", left)
}

// Restore puts the borrowed status line back. It is safe to call more than
// once and on a session that was never touched.
func (s *Status) Restore(ctx context.Context) error {
	if !s.attached {
		return nil
	}
	return Restore(ctx, s.session)
}

// Restore is also reachable without a Status, so that `hey-codex stop` can put
// the line back after killing a listener that had no chance to clean up.
func Restore(ctx context.Context, session string) error {
	was, err := get(ctx, session, wasOption)
	if err != nil || was == "" {
		return err
	}
	if was == "set" {
		base, err := get(ctx, session, baseOption)
		if err != nil {
			return err
		}
		if err := setOption(ctx, session, "status-right", base); err != nil {
			return err
		}
	} else if err := unsetOption(ctx, session, "status-right"); err != nil {
		return err
	}
	if err := unsetOption(ctx, session, wasOption); err != nil {
		return err
	}
	return unsetOption(ctx, session, baseOption)
}

// sessionOptionIsSet reports whether the option is set on the session itself
// rather than inherited from the global value.
func sessionOptionIsSet(ctx context.Context, session, option string) (bool, error) {
	output, err := exec.CommandContext(ctx, "tmux", "show-options", "-t", session).Output()
	if err != nil {
		return false, fmt.Errorf("read tmux options: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, option+" ") || line == option {
			return true, nil
		}
	}
	return false, nil
}

func effective(ctx context.Context, session, option string, sessionSet bool) (string, error) {
	if sessionSet {
		return get(ctx, session, option)
	}
	output, err := exec.CommandContext(ctx, "tmux", "show-option", "-gqv", option).Output()
	if err != nil {
		return "", fmt.Errorf("read global tmux %s: %w", option, err)
	}
	return strings.TrimRight(string(output), "\n"), nil
}

func unsetOption(ctx context.Context, session, option string) error {
	output, err := exec.CommandContext(ctx, "tmux", "set-option", "-t", session, "-u", option).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unset tmux %s: %s", option, strings.TrimSpace(string(output)))
	}
	return nil
}

// indicator renders a compact, coloured status light directly beside the
// speech-to-text label. The final foreground colour restores the neutral
// status text colour for the label that follows it.
func indicator(state string) string { return indicatorWith(state, "#[fg=#315B82]") }

func indicatorWith(state, after string) string {
	color := "#98C379" // idle and a successfully delivered transcription
	switch state {
	case "recording":
		color = "#E06C75"
	case "transcribing":
		color = "#E5C07B"
	case "error":
		color = "#E06C75"
	}
	return "#[fg=" + color + "]●" + after + " "
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
	return setOption(ctx, s.session, option, value)
}

func (s *Status) get(ctx context.Context, option string) (string, error) {
	return get(ctx, s.session, option)
}

func setOption(ctx context.Context, session, option, value string) error {
	output, err := exec.CommandContext(ctx, "tmux", "set-option", "-t", session, option, value).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set tmux %s: %s", option, strings.TrimSpace(string(output)))
	}
	return nil
}

func get(ctx context.Context, session, option string) (string, error) {
	output, err := exec.CommandContext(ctx, "tmux", "show-option", "-t", session, "-qv", option).Output()
	if err != nil {
		return "", fmt.Errorf("read tmux %s: %w", option, err)
	}
	return strings.TrimRight(string(output), "\n"), nil
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
