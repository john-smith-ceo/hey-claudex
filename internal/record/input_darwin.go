//go:build darwin

package record

func defaultDevice() string { return ":default" }

func inputArgs(device string) ([]string, error) {
	return []string{"-f", "avfoundation", "-i", device}, nil
}
