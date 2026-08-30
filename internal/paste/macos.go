package paste

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

type Paster interface {
	ActiveTarget() (string, error)
	Paste(text, target string) error
}

type macOS struct{}

func NewMacOS() Paster { return macOS{} }

func (macOS) ActiveTarget() (string, error) {
	cmd := exec.Command("osascript", "-e", `tell application "System Events" to get name of first application process whose frontmost is true`)
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("read active application (grant Accessibility permission): " + err.Error())
	}
	target := strings.TrimSpace(string(output))
	if target == "" {
		return "", errors.New("active application is empty")
	}
	return target, nil
}

func (macOS) Paste(text, target string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("refusing to paste empty transcription")
	}
	if strings.TrimSpace(target) == "" {
		return errors.New("refusing to paste without an active application target")
	}
	copy := exec.Command("pbcopy")
	copy.Stdin = strings.NewReader(text)
	if output, err := copy.CombinedOutput(); err != nil {
		return errors.New("copy transcription to clipboard: " + strings.TrimSpace(string(output)))
	}
	script := `tell application "System Events" to tell process ` + appleScriptString(target) + ` to keystroke "v" using command down`
	cmd := exec.Command("osascript", "-e", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New("paste transcription (grant Accessibility permission): " + strings.TrimSpace(string(bytes.TrimSpace(output))))
	}
	return nil
}

func appleScriptString(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}
