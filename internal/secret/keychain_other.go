//go:build !darwin && !linux

package secret

import "errors"

func Load(string) (string, error) { return "", errors.New("macOS Keychain is unavailable") }
func Save(string, string) error   { return errors.New("macOS Keychain is unavailable") }

func Location(string) string { return "nowhere: no supported secret store" }
