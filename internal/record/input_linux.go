//go:build linux

package record

// PipeWire exposes a PulseAudio-compatible server on current mainstream Linux
// desktops. This also works on distributions that run PulseAudio directly.
func defaultDevice() string { return "default" }

func inputArgs(device string) ([]string, error) {
	return []string{"-f", "pulse", "-i", device}, nil
}
