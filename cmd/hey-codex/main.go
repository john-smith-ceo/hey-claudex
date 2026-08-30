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
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/johnsmith/hey-codex/internal/bridge"
	"github.com/johnsmith/hey-codex/internal/secret"
	"github.com/johnsmith/hey-codex/internal/statusbar"
	"github.com/johnsmith/hey-codex/internal/transcribe"
)

const keychainService = "hey-codex.openai-api-key"

func main() {
	// AppKit creates NSStatusItem/NSWindow internals and requires macOS's original main thread.
	runtime.LockOSThread()
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "doctor":
		os.Exit(doctor(os.Args[2:], os.Stdout))
	case "setup-api-key":
		os.Exit(setupAPIKey(os.Args[2:], os.Stdin, os.Stdout))
	case "install":
		os.Exit(install(os.Stdout))
	case "tmux":
		os.Exit(tmuxStart(os.Args[2:]))
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
	fmt.Fprintln(w, "usage: hey-codex <doctor|setup-api-key|install|tmux|run>")
}

func doctor(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	verifyAPI := fs.Bool("verify-api", false, "verify API access without sending audio")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	failures := 0
	for _, binary := range []string{"ffmpeg", "pbcopy", "osascript", "security"} {
		if path, err := exec.LookPath(binary); err == nil {
			fmt.Fprintf(w, "ok   %-10s %s\n", binary, path)
		} else {
			fmt.Fprintf(w, "fail %-10s not found\n", binary)
			failures++
		}
	}
	key := os.Getenv("HEY_CODEX_OPENAI_API_KEY")
	if key == "" {
		key, _ = secret.Load(keychainService)
	}
	if key != "" {
		fmt.Fprintln(w, "ok   OpenAI API key available")
		if *verifyAPI {
			if err := transcribe.Verify(context.Background(), key); err != nil {
				fmt.Fprintln(w, "fail OpenAI API access:", err)
				failures++
			} else {
				fmt.Fprintln(w, "ok   OpenAI API access to gpt-transcribe")
			}
		}
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

func setupAPIKey(args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("setup-api-key", flag.ContinueOnError)
	envFile := fs.String("env-file", "", "read OPENAI_API_KEY from a dotenv file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var key string
	if *envFile != "" {
		loaded, err := loadDotenvKey(*envFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "load API key:", err)
			return 1
		}
		key = loaded
	} else {
		fmt.Fprint(out, "OpenAI API key: ")
		if file, ok := in.(*os.File); ok && file == os.Stdin {
			if err := exec.Command("stty", "-echo").Run(); err == nil {
				defer func() {
					_ = exec.Command("stty", "echo").Run()
					fmt.Fprintln(out)
				}()
			}
		}
		entered, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintln(os.Stderr, "read API key:", err)
			return 1
		}
		key = strings.TrimSpace(entered)
	}
	if err := secret.Save(keychainService, key); err != nil {
		fmt.Fprintln(os.Stderr, "save API key:", err)
		return 1
	}
	fmt.Fprintln(out, "saved in the macOS login Keychain")
	return 0
}

func loadDotenvKey(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		name, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(name) != "OPENAI_API_KEY" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			} else {
				value = value[1 : len(value)-1]
			}
		}
		if value == "" {
			return "", errors.New("OPENAI_API_KEY is empty")
		}
		return value, nil
	}
	return "", errors.New("OPENAI_API_KEY not found")
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
	device := fs.String("device", ":default", "AVFoundation audio device")
	tmuxTarget := fs.String("tmux-target", "hey-codex:0.0", "tmux pane receiving transcriptions")
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

	bar := statusbar.New()
	app, err := bridge.New(bridge.Config{Mode: bridge.Mode(*mode), Silence: *silence, Device: *device, APIKey: key, Log: os.Stderr, State: bar.Set, TmuxTarget: *tmuxTarget})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "hey-codex ready: Right Option (%s mode), target %s; use the menu-bar icon or Ctrl+C to stop\n", *mode, *tmuxTarget)
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(context.Background())
		bar.Stop()
	}()
	bar.Run()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "run:", err)
		return 1
	}
	return 0
}

func tmuxStart(args []string) int {
	fs := flag.NewFlagSet("tmux", flag.ContinueOnError)
	session := fs.String("session", "hey-codex", "tmux session name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "tmux not found")
		return 1
	}
	if output, err := exec.Command("tmux", "has-session", "-t", *session).CombinedOutput(); err == nil {
		_ = output
		fmt.Printf("tmux session %q already exists\n", *session)
	} else if output, err := exec.Command("tmux", "new-session", "-d", "-s", *session, "-c", mustGetwd(), "codex").CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "start tmux Codex session:", strings.TrimSpace(string(output)))
		return 1
	} else {
		fmt.Printf("started Codex in tmux session %q\n", *session)
	}
	fmt.Printf("attach: tmux attach -t %s\n", *session)
	fmt.Printf("listen: hey-codex run --tmux-target %s:0.0\n", *session)
	return 0
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
