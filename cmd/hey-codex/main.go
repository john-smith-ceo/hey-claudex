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
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/johnsmith/hey-codex/internal/bridge"
	"github.com/johnsmith/hey-codex/internal/hotkey"
	"github.com/johnsmith/hey-codex/internal/record"
	"github.com/johnsmith/hey-codex/internal/secret"
	"github.com/johnsmith/hey-codex/internal/tmux"
	"github.com/johnsmith/hey-codex/internal/transcribe"
)

const keychainService = "hey-codex.openai-api-key"

func main() {
	if len(os.Args) < 2 {
		os.Exit(start(nil))
	}

	switch os.Args[1] {
	case "--":
		// Everything after `--` belongs to Codex. This keeps hey-codex's
		// options separate from Codex's and avoids shell parsing entirely.
		os.Exit(start(os.Args[1:]))
	case "doctor":
		os.Exit(doctor(os.Args[2:], os.Stdout))
	case "setup-api-key":
		os.Exit(setupAPIKey(os.Args[2:], os.Stdin, os.Stdout))
	case "install":
		os.Exit(install(os.Stdout))
	case "start", "tmux":
		os.Exit(start(os.Args[2:]))
	case "stop":
		os.Exit(stop(os.Args[2:]))
	case "uninstall":
		os.Exit(uninstall(os.Args[2:]))
	case "run":
		os.Exit(run(os.Args[2:]))
	case "record":
		os.Exit(recordOnce(os.Args[2:]))
	case "help", "--help", "-h":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `hey-codex — голосовой ввод для Codex

Самый простой старт
  hey-codex
      Откроет Codex и включит голосовой ввод. Больше ничего помнить не нужно.

Как говорить
  1. Нажмите правый Option один раз.
  2. Скажите задачу.
  3. Нажмите правый Option ещё раз — или просто помолчите 2 секунды.
  4. Текст появится в Codex. Проверьте его и нажмите Enter сами.

Первый запуск
  hey-codex setup-api-key
      Сохранит ваш OpenAI API key в macOS Keychain.
  hey-codex doctor
      Проверит микрофон, tmux, ffmpeg и ключ.

Полезные команды
  hey-codex                 Открыть Codex с голосовым вводом.
  hey-codex start           То же самое.
  hey-codex start --mode push
                            Говорите, пока удерживаете правый Option.
  hey-codex -- --approve-for-me
                            Запустить Codex с его собственным флагом.
  hey-codex stop            Остановить Codex и голосовой ввод.
  hey-codex doctor          Проверить, всё ли готово.
  hey-codex doctor --verify-api
                            Проверить доступ к OpenAI без отправки аудио.
  hey-codex record          Linux: записать одну фразу и вставить текст в Codex.
  hey-codex uninstall       Убрать локальную dev-установку.
  hey-codex uninstall --purge-key
                            Убрать также ключ из Keychain.

Строка tmux
  Пастельно-синяя строка внизу терминала показывает mode:tap, rec…,
  transcribe…, done или error. Настраивается только сессия hey-codex:
  ваша глобальная тема tmux не меняется.

Безопасность
  hey-codex не нажимает Enter за вас. Голос доставляется только в вашу
  собственную сессию Codex, а не в случайное активное окно.

Флаги Codex
  После -- можно передать любые флаги самого Codex:
    hey-codex -- --approve-for-me
    hey-codex -- --model gpt-5.4
  Это действует при создании новой сессии. Если сессия уже идёт, сначала
  завершите её: hey-codex stop

Linux beta
  На Linux используется явная команда записи вместо глобальной горячей клавиши:
    hey-codex record
  Она записывает с default PipeWire/PulseAudio microphone, останавливается
  после двух секунд тишины и вставляет текст в hey-codex:0.0. Запускайте её
  из второго терминала, пока открыта tmux-сессия Codex.

`)
}

func doctor(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	verifyAPI := fs.Bool("verify-api", false, "verify API access without sending audio")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	failures := 0
	binaries := []string{"ffmpeg", "tmux"}
	if runtime.GOOS == "darwin" {
		binaries = append(binaries, "security")
	}
	for _, binary := range binaries {
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
	} else if runtime.GOOS == "darwin" {
		fmt.Fprintln(w, "fail OpenAI API key missing (run: hey-codex setup-api-key)")
		failures++
	} else {
		fmt.Fprintln(w, "fail OpenAI API key missing (export HEY_CODEX_OPENAI_API_KEY)")
		failures++
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(w, "note grant Microphone permission to the launcher application before run")
	} else if runtime.GOOS == "linux" {
		fmt.Fprintln(w, "note Linux uses `hey-codex record`; PipeWire or PulseAudio must expose the default microphone")
	}
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
	if runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "setup-api-key uses macOS Keychain; on Linux export HEY_CODEX_OPENAI_API_KEY in your shell profile")
		return 1
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
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "tmux is required; install it first: brew install tmux")
		return 1
	}
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
	device := fs.String("device", record.DefaultDevice(), "ffmpeg audio input device")
	tmuxTarget := fs.String("tmux-target", "hey-codex:0.0", "tmux pane receiving transcriptions")
	tmuxSession := fs.String("tmux-session", "hey-codex", "tmux session owning the hey-codex status line")
	codexFlags := fs.String("codex-flags", "", "Codex arguments to display in the tmux status line")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *mode != "tap" && *mode != "push" {
		fmt.Fprintln(os.Stderr, "--mode must be tap or push")
		return 2
	}
	key, err := openAIKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	status, err := tmux.NewStatus(*tmuxSession, strings.Fields(*codexFlags), *mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize tmux status:", err)
		return 1
	}
	if err := status.Configure(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "configure tmux status:", err)
		return 1
	}
	app, err := bridge.New(bridge.Config{Mode: bridge.Mode(*mode), Silence: *silence, Device: *device, APIKey: key, Log: os.Stderr, State: func(state string) {
		if err := status.Set(context.Background(), state); err != nil {
			fmt.Fprintln(os.Stderr, "update tmux status:", err)
		}
	}, TmuxTarget: *tmuxTarget})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "hey-codex ready: Right Option (%s mode), target %s; Ctrl+C stops the listener\n", *mode, *tmuxTarget)
	if err := app.Run(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "run:", err)
		return 1
	}
	return 0
}

// recordOnce is the portable Linux interaction: invoke it from a second
// terminal, speak, and let silence finish the recording. The transcript is
// pasted into one explicit tmux pane and is never submitted automatically.
func recordOnce(args []string) int {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	silence := fs.Duration("silence", 2*time.Second, "silence timeout before finishing the recording")
	device := fs.String("device", record.DefaultDevice(), "ffmpeg audio input device")
	tmuxTarget := fs.String("tmux-target", "hey-codex:0.0", "tmux pane receiving the transcription")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *silence <= 0 {
		fmt.Fprintln(os.Stderr, "--silence must be positive")
		return 2
	}
	key, err := openAIKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sender, err := tmux.New(*tmuxTarget)
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize tmux:", err)
		return 1
	}
	if err := sender.Check(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintln(os.Stderr, "recording: speak now; recording stops after silence")
	path, err := record.NewFFmpeg(*device, *silence).Record(ctx, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		return 1
	}
	defer os.Remove(path)
	fmt.Fprintln(os.Stderr, "transcribing")
	text, err := transcribe.NewOpenAI(key).Transcribe(context.Background(), path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "transcribe:", err)
		return 1
	}
	if err := sender.Send(context.Background(), text); err != nil {
		fmt.Fprintln(os.Stderr, "deliver transcription:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "transcription delivered to tmux %s; review it and press Enter yourself\n", sender.Target())
	return 0
}

func start(args []string) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	session := fs.String("session", "hey-codex", "tmux session name")
	mode := fs.String("mode", "tap", "voice mode: tap or push")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "tmux not found")
		return 1
	}
	if *mode != "tap" && *mode != "push" {
		fmt.Fprintln(os.Stderr, "--mode must be tap or push")
		return 2
	}
	codexArgs := fs.Args()
	_, sessionErr := exec.Command("tmux", "has-session", "-t", *session).CombinedOutput()
	if sessionErr == nil && len(codexArgs) > 0 {
		fmt.Fprintf(os.Stderr, "Codex flags apply only when creating a session; %q is already running. Run: hey-codex stop\n", *session)
		return 1
	}
	if sessionErr != nil {
		command := append([]string{"new-session", "-d", "-s", *session, "-n", "codex", "-c", mustGetwd(), "codex"}, codexArgs...)
		if output, err := exec.Command("tmux", command...).CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "start tmux Codex session:", strings.TrimSpace(string(output)))
			return 1
		}
	}
	status, err := tmux.NewStatus(*session, codexArgs, *mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize tmux status:", err)
		return 1
	}
	if err := status.Configure(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "configure tmux status:", err)
		return 1
	}
	if hotkey.GlobalSupported() && !tmuxWindowExists(*session, "voice") {
		executable, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "locate hey-codex executable:", err)
			return 1
		}
		target := *session + ":0.0"
		command := []string{"new-window", "-d", "-t", *session, "-n", "voice", "-c", mustGetwd(), executable, "run", "--mode", *mode, "--tmux-target", target, "--tmux-session", *session, "--codex-flags", strings.Join(codexArgs, " ")}
		if output, err := exec.Command("tmux", command...).CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "start voice listener:", strings.TrimSpace(string(output)))
			return 1
		}
	}
	if !hotkey.GlobalSupported() {
		fmt.Fprintf(os.Stderr, "Linux terminal mode: in a second terminal run: hey-codex record --tmux-target %s:0.0\n", *session)
	}
	attach := exec.Command("tmux", "attach-session", "-t", *session)
	attach.Stdin, attach.Stdout, attach.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := attach.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "attach tmux:", err)
		return 1
	}
	return 0
}

func stop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	session := fs.String("session", "hey-codex", "tmux session name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if output, err := exec.Command("tmux", "kill-session", "-t", *session).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "stop hey-codex:", strings.TrimSpace(string(output)))
		return 1
	}
	return 0
}

// uninstall removes the developer-installed command. Homebrew removes its own
// binary; --purge-key additionally removes the API key from the login Keychain.
func uninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purgeKey := fs.Bool("purge-key", false, "also remove the OpenAI API key from macOS Keychain")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_ = stop(nil)
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
	target := filepath.Join(home, ".local", "bin", "hey-codex")
	if current, err := filepath.EvalSymlinks(target); err == nil && current == executable {
		if err := os.Remove(target); err != nil {
			fmt.Fprintln(os.Stderr, "remove developer symlink:", err)
			return 1
		}
	}
	if *purgeKey {
		if runtime.GOOS != "darwin" {
			fmt.Fprintln(os.Stderr, "no Keychain key exists on this platform")
			return 1
		}
		if err := exec.Command("security", "delete-generic-password", "-s", keychainService).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "remove Keychain key:", err)
			return 1
		}
	}
	return 0
}

func openAIKey() (string, error) {
	if key := strings.TrimSpace(os.Getenv("HEY_CODEX_OPENAI_API_KEY")); key != "" {
		return key, nil
	}
	key, err := secret.Load(keychainService)
	if err == nil && strings.TrimSpace(key) != "" {
		return key, nil
	}
	if runtime.GOOS == "darwin" {
		return "", errors.New("OpenAI API key missing; run hey-codex setup-api-key")
	}
	return "", errors.New("OpenAI API key missing; export HEY_CODEX_OPENAI_API_KEY")
}

func tmuxWindowExists(session, name string) bool {
	output, err := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_name}").Output()
	if err != nil {
		return false
	}
	for _, window := range strings.Fields(string(output)) {
		if window == name {
			return true
		}
	}
	return false
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
