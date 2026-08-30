//go:build darwin

package secret

import (
	"errors"
	"os/exec"
	"strings"
)

func Load(service string) (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return "", errors.New("key not found in login Keychain")
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", errors.New("empty key in login Keychain")
	}
	return key, nil
}

func Save(service, value string) error {
	if value == "" {
		return errors.New("API key is empty")
	}
	return exec.Command("security", "add-generic-password", "-U", "-s", service, "-a", "hey-codex", "-w", value).Run()
}
