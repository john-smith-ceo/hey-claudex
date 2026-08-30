//go:build !darwin

package secret

import "errors"

func Load(string) (string, error) { return "", errors.New("macOS Keychain is unavailable") }
func Save(string, string) error   { return errors.New("macOS Keychain is unavailable") }
