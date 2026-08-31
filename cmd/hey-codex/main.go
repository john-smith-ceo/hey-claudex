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
	"github.com/johnsmith/hey-codex/internal/hotkey"
	"github.com/johnsmith/hey-codex/internal/record"
	"github.com/johnsmith/hey-codex/internal/secret"
	"github.com/johnsmith/hey-codex/internal/tmux"
	"github.com/johnsmith/hey-codex/internal/transcribe"
)

const keychainService = "hey-codex.openai-api-key"

// voiceWindow is the hidden tmux window the listener runs in.
const voiceWindow = "hey-codex-voice"

func main() {
	if len(os.Args) < 2 {
		// Inside tmux the user already has a session and very likely a Codex
		// in it; creating a second one would be the wrong favour.
		if insideTmux() {
			os.Exit(listen(nil))
		}
		os.Exit(start(nil))
	}

	switch os.Args[1] {
	case "--":
		// Everything after `--` belongs to Codex. This keeps hey-codex's
		// options separate from Codex's and avoids shell parsing entirely.
		os.Exit(start(os.Args[1:]))
	case "listen", "join":
		os.Exit(listen(os.Args[2:]))
	case "keys":
		os.Exit(listKeys(os.Stdout))
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
      Если вы уже в tmux — подключит голосовой ввод к текущей панели.
      Если нет — откроет Codex в своей сессии и включит голосовой ввод.
      Больше ничего помнить не нужно.

Как говорить
  1. Нажмите правый Option один раз (клавиша меняется флагом --key).
  2. Скажите задачу.
  3. Нажмите правый Option ещё раз — или просто помолчите 5 секунд.
  4. Текст появится в Codex. Проверьте его и нажмите Enter сами.

Первый запуск
  hey-codex setup-api-key
      Сохранит ваш OpenAI API key: на macOS — в Keychain, на Linux —
      в ~/.config/hey-codex, доступный только вам.
  hey-codex doctor
      Проверит микрофон, tmux, ffmpeg и ключ.

Полезные команды
  hey-codex                 Открыть Codex с голосовым вводом.
  hey-codex start           То же самое.
  hey-codex listen          Подключить голос к панели, из которой вызвали.
  hey-codex keys            Показать клавиши, которые можно назначить.
  hey-codex start --key Alt_R
                            Назначить свою клавишу вместо правого Option.
  hey-codex start --mode push
                            Говорите, пока удерживаете правый Option.
  hey-codex -- --approve-for-me
                            Запустить Codex с его собственным флагом.
  hey-codex stop            Остановить голосовой ввод. Вашу сессию tmux
                            не трогает: убирает только своё окно и
                            возвращает строку состояния.
  hey-codex doctor          Проверить, всё ли готово.
  hey-codex doctor --verify-api
                            Проверить доступ к OpenAI без отправки аудио.
  hey-codex uninstall       Убрать локальную dev-установку.
  hey-codex uninstall --purge-key
                            Убрать также ключ из Keychain.

Строка tmux
  Состояние видно внизу терминала: mode:tap, rec…, transcribe…, done
  или error.
  В своей сессии hey-codex занимает строку целиком. В вашей — только
  дописывает значок справа, а прежнее содержимое возвращает при
  остановке. Ваше оформление остаётся вашим.

Безопасность
  hey-codex не нажимает Enter за вас. Голос доставляется только в вашу
  собственную сессию Codex, а не в случайное активное окно.

Флаги Codex
  После -- можно передать любые флаги самого Codex:
    hey-codex -- --approve-for-me
    hey-codex -- --model gpt-5.4
  Это действует при создании новой сессии. Если сессия уже идёт, сначала
  завершите её: hey-codex stop

`)
}

// listKeys prints the closed list of selectable hotkeys. The list is closed on
// purpose: a key that takes part in typing needs different handling, and an
// arbitrary key would silently get the wrong one.
func listKeys(w io.Writer) int {
	fmt.Fprintln(w, "Клавиши, которые можно назначить (--key):")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Свободные — в наборе текста не участвуют, работают оба режима:")
	for _, key := range hotkey.Names() {
		if key.Category != hotkey.Free {
			continue
		}
		fmt.Fprintln(w, "    "+describe(key))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Рабочие — нажимаются при наборе, поэтому только --mode tap")
	fmt.Fprintln(w, "  и только одиночное нажатие без других клавиш:")
	for _, key := range hotkey.Names() {
		if key.Category != hotkey.Typing {
			continue
		}
		fmt.Fprintln(w, "    "+describe(key))
	}
	fmt.Fprintf(w, "\nПо умолчанию: %s\n", hotkey.Default)
	return 0
}

func describe(key hotkey.Key) string {
	line := fmt.Sprintf("%-12s", key.Name)
	if len(key.Aliases) > 0 {
		line += " (" + strings.Join(key.Aliases, ", ") + ")"
	}
	if !key.AvailableOnDarwin() {
		line += " — на клавиатурах Apple такой клавиши нет"
	}
	return strings.TrimRight(line, " ")
}

func doctor(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	verifyAPI := fs.Bool("verify-api", false, "verify API access without sending audio")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	failures := 0
	required := []string{"ffmpeg", "tmux"}
	if runtime.GOOS == "darwin" {
		required = append(required, "security")
	}
	for _, binary := range required {
		if path, err := exec.LookPath(binary); err == nil {
			fmt.Fprintf(w, "ok   %-10s %s\n", binary, path)
		} else {
			fmt.Fprintf(w, "fail %-10s not found\n", binary)
			failures++
		}
	}
	if hotKey, err := hotkey.Lookup(hotkey.Default); err != nil {
		fmt.Fprintln(w, "fail hotkey table:", err)
		failures++
	} else if _, err := hotkey.New(hotKey); err != nil {
		fmt.Fprintln(w, "fail hotkey:", err)
		failures++
	} else {
		fmt.Fprintf(w, "ok   hotkey     %s available\n", hotKey.Name)
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
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(w, "note grant Microphone and Accessibility permission to the launcher application before run")
	} else {
		fmt.Fprintln(w, "note hey-codex needs an X11 session; Wayland is not supported yet")
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
	fmt.Fprintln(out, "saved:", secret.Location(keychainService))
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
		if !found || !dotenvKeyNames[strings.TrimSpace(name)] {
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
			return "", fmt.Errorf("%s is empty", strings.TrimSpace(name))
		}
		return value, nil
	}
	return "", errors.New("neither OPENAI_API_KEY nor OPEN_AI_API_KEY found")
}

// Both spellings occur in the wild, and a key file is not worth renaming just
// to satisfy a parser.
var dotenvKeyNames = map[string]bool{
	"OPENAI_API_KEY":  true,
	"OPEN_AI_API_KEY": true,
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
	silence := fs.Duration("silence", 5*time.Second, "tap-mode silence timeout")
	device := fs.String("device", record.DefaultDevice(), "audio input device")
	keyName := fs.String("key", hotkey.Default, "hotkey; run: hey-codex keys")
	tmuxTarget := fs.String("tmux-target", "hey-codex:0.0", "tmux pane receiving transcriptions")
	tmuxSession := fs.String("tmux-session", "hey-codex", "tmux session owning the hey-codex status line")
	codexFlags := fs.String("codex-flags", "", "Codex arguments to display in the tmux status line")
	attached := fs.Bool("attached", false, "the session belongs to the user: borrow the status line instead of taking it")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *mode != "tap" && *mode != "push" {
		fmt.Fprintln(os.Stderr, "--mode must be tap or push")
		return 2
	}
	hotKey, err := hotkey.Lookup(*keyName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := hotkey.Validate(hotKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	key, err := openAIKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	status, err := tmux.NewStatus(*tmuxSession, strings.Fields(*codexFlags), *mode, *attached)
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
	}, TmuxTarget: *tmuxTarget, Key: hotKey})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "hey-codex ready: %s (%s mode), target %s; Ctrl+C stops the listener\n", hotKey.Name, *mode, *tmuxTarget)
	runErr := app.Run(context.Background())
	if err := status.Restore(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "restore tmux status:", err)
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		fmt.Fprintln(os.Stderr, "run:", runErr)
		return 1
	}
	return 0
}

// listen attaches to the session the user is already in: the transcription goes
// to the pane it was called from, nothing is created, nothing is taken over.
func listen(args []string) int {
	fs := flag.NewFlagSet("listen", flag.ContinueOnError)
	mode := fs.String("mode", "tap", "voice mode: tap or push")
	keyName := fs.String("key", hotkey.Default, "hotkey; run: hey-codex keys")
	target := fs.String("target", "", "tmux pane receiving transcriptions (default: the pane you run this from)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !insideTmux() {
		fmt.Fprintln(os.Stderr, "hey-codex listen works inside tmux; outside it run: hey-codex")
		return 2
	}
	if *mode != "tap" && *mode != "push" {
		fmt.Fprintln(os.Stderr, "--mode must be tap or push")
		return 2
	}
	hotKey, err := hotkey.Lookup(*keyName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := hotkey.Validate(hotKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *mode == "push" && !hotKey.SupportsPush() {
		fmt.Fprintf(os.Stderr, "%s is pressed while typing, so hold-to-talk is impossible for it; use --mode tap\n", hotKey.Name)
		return 2
	}
	pane := *target
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	if pane == "" {
		fmt.Fprintln(os.Stderr, "cannot tell which pane to speak into; pass --target")
		return 1
	}
	session, err := currentSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if tmuxWindowExists(session, voiceWindow) {
		fmt.Fprintf(os.Stderr, "hey-codex is already listening in %q; stop it with: hey-codex stop\n", session)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate hey-codex executable:", err)
		return 1
	}
	command := []string{"new-window", "-d", "-t", session, "-n", voiceWindow, "-c", mustGetwd(), executable,
		"run", "--attached", "--mode", *mode, "--key", hotKey.Name, "--tmux-target", pane, "--tmux-session", session}
	if output, err := exec.Command("tmux", command...).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "start voice listener:", strings.TrimSpace(string(output)))
		return 1
	}
	fmt.Printf("Слушаю: %s (%s). Речь придёт в панель %s — проверьте текст и нажмите Enter сами.\n", hotKey.Name, *mode, pane)
	fmt.Println("Остановить: hey-codex stop")
	return 0
}

func start(args []string) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	session := fs.String("session", "hey-codex", "tmux session name")
	mode := fs.String("mode", "tap", "voice mode: tap or push")
	keyName := fs.String("key", hotkey.Default, "hotkey; run: hey-codex keys")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	hotKey, err := hotkey.Lookup(*keyName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := hotkey.Validate(hotKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *mode == "push" && !hotKey.SupportsPush() {
		fmt.Fprintf(os.Stderr, "%s is pressed while typing, so hold-to-talk is impossible for it; use --mode tap\n", hotKey.Name)
		return 2
	}
	if insideTmux() {
		fmt.Fprintln(os.Stderr, "you are already inside tmux, and hey-codex would start a second Codex in a session it cannot attach to.")
		fmt.Fprintln(os.Stderr, "run this instead: hey-codex listen")
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
	status, err := tmux.NewStatus(*session, codexArgs, *mode, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize tmux status:", err)
		return 1
	}
	if err := status.Configure(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "configure tmux status:", err)
		return 1
	}
	if !tmuxWindowExists(*session, voiceWindow) {
		executable, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "locate hey-codex executable:", err)
			return 1
		}
		target := *session + ":0.0"
		command := []string{"new-window", "-d", "-t", *session, "-n", voiceWindow, "-c", mustGetwd(), executable, "run", "--mode", *mode, "--key", hotKey.Name, "--tmux-target", target, "--tmux-session", *session, "--codex-flags", strings.Join(codexArgs, " ")}
		if output, err := exec.Command("tmux", command...).CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "start voice listener:", strings.TrimSpace(string(output)))
			return 1
		}
	}
	attach := exec.Command("tmux", "attach-session", "-t", *session)
	attach.Stdin, attach.Stdout, attach.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := attach.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "attach tmux:", err)
		return 1
	}
	return 0
}

// stop removes only what hey-codex started. A session that belonged to the
// user loses its listener window and gets its status line back; the session is
// never killed, because killing it would take the user's work with it.
func stop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	session := fs.String("session", "", "tmux session hey-codex created (default: hey-codex)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" && insideTmux() {
		if current, err := currentSession(); err == nil && tmuxWindowExists(current, voiceWindow) {
			if output, err := exec.Command("tmux", "kill-window", "-t", current+":"+voiceWindow).CombinedOutput(); err != nil {
				fmt.Fprintln(os.Stderr, "stop listener:", strings.TrimSpace(string(output)))
				return 1
			}
			if err := tmux.Restore(context.Background(), current); err != nil {
				fmt.Fprintln(os.Stderr, "restore tmux status:", err)
			}
			fmt.Printf("Слушатель остановлен, строка состояния сессии %q возвращена.\n", current)
			return 0
		}
	}
	owned := *session
	if owned == "" {
		owned = "hey-codex"
	}
	if _, err := exec.Command("tmux", "has-session", "-t", owned).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "nothing to stop: no hey-codex listener here and no", owned, "session")
		return 1
	}
	if output, err := exec.Command("tmux", "kill-session", "-t", owned).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "stop hey-codex:", strings.TrimSpace(string(output)))
		return 1
	}
	return 0
}

func insideTmux() bool { return strings.TrimSpace(os.Getenv("TMUX")) != "" }

func currentSession() (string, error) {
	output, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", fmt.Errorf("cannot tell which tmux session this is: %w", err)
	}
	name := strings.TrimSpace(string(output))
	if name == "" {
		return "", errors.New("cannot tell which tmux session this is")
	}
	return name, nil
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
	return "", errors.New("OpenAI API key missing; run hey-codex setup-api-key")
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
