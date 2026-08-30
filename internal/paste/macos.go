package paste

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

type Paster interface{ Paste(string) error }

type macOS struct{}

func NewMacOS() Paster { return macOS{} }

func (macOS) Paste(text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("refusing to paste empty transcription")
	}
	copy := exec.Command("pbcopy")
	copy.Stdin = strings.NewReader(text)
	if output, err := copy.CombinedOutput(); err != nil {
		return errors.New("copy transcription to clipboard: " + strings.TrimSpace(string(output)))
	}
	cmd := exec.Command("osascript", "-e", `tell application "System Events" to keystroke "v" using command down`)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New("paste transcription (grant Accessibility permission): " + strings.TrimSpace(string(bytes.TrimSpace(output))))
	}
	return nil
}
