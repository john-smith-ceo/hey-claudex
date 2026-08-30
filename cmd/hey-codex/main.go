package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnsmith/hey-codex/internal/bridge"
	"github.com/johnsmith/hey-codex/internal/secret"
)

const keychainService = "hey-codex.openai-api-key"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "doctor":
		os.Exit(doctor(os.Stdout))
	case "setup-api-key":
		os.Exit(setupAPIKey(os.Stdin, os.Stdout))
	case "install":
		os.Exit(install(os.Stdout))
	case "run":
		os.Exit(run(os.Args[2:]))
	case "help", "--help", "-h":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: hey-codex <doctor|setup-api-key|install|run>")
}

func doctor(w io.Writer) int {
	failures := 0
	for _, binary := range []string{"ffmpeg", "pbcopy", "osascript", "security"} {
		if path, err := exec.LookPath(binary); err == nil {
			fmt.Fprintf(w, "ok   %-10s %s\n", binary, path)
		} else {
			fmt.Fprintf(w, "fail %-10s not found\n", binary)
			failures++
		}
	}
	if _, err := secret.Load(keychainService); err == nil || os.Getenv("HEY_CODEX_OPENAI_API_KEY") != "" {
		fmt.Fprintln(w, "ok   OpenAI API key available")
	} else {
		fmt.Fprintln(w, "fail OpenAI API key missing (run: hey-codex setup-api-key)")
		failures++
	}
	fmt.Fprintln(w, "note grant Microphone and Accessibility permissions to the launcher application before run")
	if failures > 0 {
		return 1
	}
	return 0
}

func setupAPIKey(in io.Reader, out io.Writer) int {
	fmt.Fprint(out, "OpenAI API key: ")
	if file, ok := in.(*os.File); ok && file == os.Stdin {
		if err := exec.Command("stty", "-echo").Run(); err == nil {
			defer func() {
				_ = exec.Command("stty", "echo").Run()
				fmt.Fprintln(out)
			}()
		}
	}
	key, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, "read API key:", err)
		return 1
	}
	if err := secret.Save(keychainService, strings.TrimSpace(key)); err != nil {
		fmt.Fprintln(os.Stderr, "save API key:", err)
		return 1
	}
	fmt.Fprintln(out, "saved in the macOS login Keychain")
	return 0
}

func install(out io.Writer) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate executable:", err)
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate home directory:", err)
		return 1
	}
	targetDir := filepath.Join(home, ".local", "bin")
	target := filepath.Join(targetDir, "hey-codex")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create install directory:", err)
		return 1
	}
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			fmt.Fprintf(os.Stderr, "refusing to overwrite non-symlink %s\n", target)
			return 1
		}
		current, err := filepath.EvalSymlinks(target)
		if err != nil || current != executable {
			fmt.Fprintf(os.Stderr, "refusing to replace existing link %s\n", target)
			return 1
		}
		fmt.Fprintf(out, "already installed: %s\n", target)
		return 0
	}
	if err := os.Symlink(executable, target); err != nil {
		fmt.Fprintln(os.Stderr, "install symlink:", err)
		return 1
	}
	fmt.Fprintf(out, "installed: %s -> %s\n", target, executable)
	return 0
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	mode := fs.String("mode", "tap", "recording mode: tap or push")
	silence := fs.Duration("silence", 2*time.Second, "tap-mode silence timeout")
	device := fs.String("device", ":0", "AVFoundation audio device")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *mode != "tap" && *mode != "push" {
		fmt.Fprintln(os.Stderr, "--mode must be tap or push")
		return 2
	}
	key := os.Getenv("HEY_CODEX_OPENAI_API_KEY")
	if key == "" {
		var err error
		key, err = secret.Load(keychainService)
		if err != nil {
			fmt.Fprintln(os.Stderr, "OpenAI API key missing; run hey-codex setup-api-key")
			return 1
		}
	}

	app, err := bridge.New(bridge.Config{Mode: bridge.Mode(*mode), Silence: *silence, Device: *device, APIKey: key, Log: os.Stderr})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "hey-codex ready: Right Option (%s mode); Ctrl+C to stop\n", *mode)
	if err := app.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "run:", err)
		return 1
	}
	return 0
}
