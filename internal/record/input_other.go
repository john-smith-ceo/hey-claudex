//go:build !darwin && !linux

package record

import "errors"

func defaultDevice() string { return "" }

func inputArgs(string) ([]string, error) {
	return nil, errors.New("audio capture is supported only on macOS and Linux")
}
