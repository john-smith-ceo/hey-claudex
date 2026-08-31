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

	"github.com/john-smith-ceo/hey-claudex/internal/bridge"
	"github.com/john-smith-ceo/hey-claudex/internal/hotkey"
	"github.com/john-smith-ceo/hey-claudex/internal/record"
	"github.com/john-smith-ceo/hey-claudex/internal/secret"
	"github.com/john-smith-ceo/hey-claudex/internal/tmux"
	"github.com/john-smith-ceo/hey-claudex/internal/transcribe"
)

const keychainService = "hey-claudex.openai-api-key"

// voiceWindow is the hidden tmux window the listener runs in.
const voiceWindow = "hey-claudex-voice"

// installNames are the names the tool is reachable under.
var installNames = []string{"hey-claudex", "hey-claude", "hey-codex"}

// expectedApp reads the name the binary was called by. hey-claude expects to
// speak into Claude Code and hey-codex into Codex; the check exists so that a
// transcription never lands in the wrong pane. Called by any other name, the
// tool speaks into whatever is there.
func expectedApp() string {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	switch {
	case strings.Contains(name, "claudex"):
		return ""
	case strings.Contains(name, "codex"):
		return "codex"
	case strings.Contains(name, "claude"):
		return "claude"
	}
	return ""
}

// isOwnName reports whether the command is hey-claudex under any of its names.
func isOwnName(command string) bool {
	for _, name := range installNames {
		if command == name {
			return true
		}
	}
	return false
}

// paneCommand reports the program currently running in a pane.
func paneCommand(pane string) string {
	output, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{pane_current_command}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(listen(nil))
	}

	switch os.Args[1] {
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
	fmt.Fprint(w, `hey-claudex — голосовой ввод в панель, где вы уже работаете

Самый простой старт
  hey-claude
      Включит голосовой ввод в текущей панели tmux, где идёт Claude Code.
  hey-codex
      То же самое для панели с Codex.

Как говорить
  1. Нажмите правый Alt (на маке — правый Option) один раз.
  2. Скажите задачу.
  3. Нажмите ещё раз — или просто помолчите пять секунд.
  4. Текст появится в строке ввода. Проверьте его и нажмите Enter сами.

Первый запуск
  hey-claudex setup-api-key
      Сохранит ваш OpenAI API key: на macOS — в Keychain, на Linux —
      в ~/.config/hey-claudex, доступный только вам.
  hey-claudex doctor
      Проверит микрофон, tmux, ffmpeg, горячую клавишу и ключ.

Полезные команды
  hey-claude, hey-codex      Включить голос в текущей панели.
  hey-claudex listen         То же, но без проверки, что за приложение
                             в панели.
  hey-claudex stop           Остановить голосовой ввод.
  hey-claudex keys           Показать клавиши, которые можно назначить.
  hey-claudex listen --key Shift_L
                             Назначить свою клавишу.
  hey-claudex listen --mode push
                             Говорить, пока клавиша удерживается.
  hey-claudex doctor --verify-api
                             Проверить доступ к OpenAI без отправки аудио.

Имена вызова
  hey-claude ждёт в панели Claude Code, hey-codex — Codex. Если там
  работает другое, инструмент откажется и скажет, что именно видит:
  текст, ушедший не в то окно, хуже неуслышанного. Обойти — флаг --any.

Строка tmux
  Состояние видно внизу терминала: mode:tap, rec…, transcribe…, done
  или error. В вашей сессии hey-claudex только дописывает значок справа,
  а прежнее содержимое возвращает при остановке. Ваше оформление
  остаётся вашим.

Безопасность
  hey-claudex не нажимает Enter за вас и ничего не запускает: он
  работает только в панели, которая уже открыта, и говорит только
  в неё — без поиска активного окна и без нажатий за вас.

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
	key := os.Getenv("HEY_CLAUDEX_OPENAI_API_KEY")
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
		fmt.Fprintln(w, "fail OpenAI API key missing (run: hey-claudex setup-api-key)")
		failures++
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(w, "note grant Microphone and Accessibility permission to the launcher application before run")
	} else {
		fmt.Fprintln(w, "note hey-claudex needs an X11 session; Wayland is not supported yet")
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

// install links the built binary under all three names. The aliases are not
// decoration: the name states which assistant the pane is expected to run.
func install(out io.Writer) int {
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "tmux is required; install it first")
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
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create install directory:", err)
		return 1
	}
	for _, name := range installNames {
		target := filepath.Join(targetDir, name)
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
			continue
		}
		if err := os.Symlink(executable, target); err != nil {
			fmt.Fprintln(os.Stderr, "install symlink:", err)
			return 1
		}
		fmt.Fprintf(out, "installed: %s -> %s\n", target, executable)
	}
	return 0
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	mode := fs.String("mode", "tap", "recording mode: tap or push")
	silence := fs.Duration("silence", 5*time.Second, "tap-mode silence timeout")
	device := fs.String("device", record.DefaultDevice(), "audio input device")
	keyName := fs.String("key", hotkey.Default, "hotkey; run: hey-claudex keys")
	tmuxTarget := fs.String("tmux-target", "hey-claudex:0.0", "tmux pane receiving transcriptions")
	tmuxSession := fs.String("tmux-session", "hey-claudex", "tmux session owning the hey-claudex status line")

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

	// The status line shows what is actually running in the target pane rather
	// than what a flag claims, so it cannot drift from reality.
	status, err := tmux.NewStatus(*tmuxSession, paneCommand(*tmuxTarget), *mode, *attached)
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
	fmt.Fprintf(os.Stderr, "hey-claudex ready: %s (%s mode), target %s; Ctrl+C stops the listener\n", hotKey.Name, *mode, *tmuxTarget)
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
	keyName := fs.String("key", hotkey.Default, "hotkey; run: hey-claudex keys")
	target := fs.String("target", "", "tmux pane receiving transcriptions (default: the pane you run this from)")
	any := fs.Bool("any", false, "speak into the pane whatever runs in it")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !insideTmux() {
		fmt.Fprintln(os.Stderr, "hey-claudex speaks into a pane you are already working in, so it needs tmux.")
		fmt.Fprintln(os.Stderr, "open tmux, start your assistant there, and call this from that pane.")
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
	// The name the tool was called by states the intent. Speaking into a pane
	// running something else is almost always a mistake, and a transcription in
	// the wrong window is worse than no transcription.
	if want := expectedApp(); want != "" && !*any {
		running := paneCommand(pane)
		if isOwnName(running) {
			// The tool itself is the foreground process, which means the pane
			// holds a shell rather than an assistant.
			fmt.Fprintf(os.Stderr, "no %s is running in this pane.\n", want)
			fmt.Fprintf(os.Stderr, "call %s from the pane where %s works, or repeat with --any to speak into this one.\n", filepath.Base(os.Args[0]), want)
			return 1
		}
		if running != "" && running != want {
			fmt.Fprintf(os.Stderr, "%s expects %s in this pane, but %s is running there.\n", filepath.Base(os.Args[0]), want, running)
			fmt.Fprintf(os.Stderr, "call it from a %s pane, or repeat with --any to speak into this one anyway.\n", want)
			return 1
		}
	}
	session, err := currentSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if tmuxWindowExists(session, voiceWindow) {
		fmt.Fprintf(os.Stderr, "hey-claudex is already listening in %q; stop it with: hey-claudex stop\n", session)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "locate hey-claudex executable:", err)
		return 1
	}
	command := []string{"new-window", "-d", "-t", session, "-n", voiceWindow, "-c", mustGetwd(), executable,
		"run", "--attached", "--mode", *mode, "--key", hotKey.Name, "--tmux-target", pane, "--tmux-session", session}
	if output, err := exec.Command("tmux", command...).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "start voice listener:", strings.TrimSpace(string(output)))
		return 1
	}
	fmt.Printf("Слушаю: %s (%s). Речь придёт в панель %s — проверьте текст и нажмите Enter сами.\n", hotKey.Name, *mode, pane)
	fmt.Println("Остановить: hey-claudex stop")
	return 0
}

// stop removes the listener and puts the borrowed status line back. The user's
// session is never killed: hey-claudex did not create it and has no business
// taking it down.
func stop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !insideTmux() {
		fmt.Fprintln(os.Stderr, "run this from the tmux session where hey-claudex is listening")
		return 2
	}
	current, err := currentSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// The window may already be gone — a crash, or somebody closing it by hand.
	// The borrowed status line still has to be given back, so cleaning up runs
	// either way.
	if tmuxWindowExists(current, voiceWindow) {
		if output, err := exec.Command("tmux", "kill-window", "-t", current+":"+voiceWindow).CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "stop listener:", strings.TrimSpace(string(output)))
			return 1
		}
		restored, err := tmux.Restore(context.Background(), current)
		if err != nil {
			fmt.Fprintln(os.Stderr, "restore tmux status:", err)
		}
		if restored {
			fmt.Printf("Слушатель остановлен, строка состояния сессии %q возвращена.\n", current)
		} else {
			fmt.Println("Слушатель остановлен.")
		}
		return 0
	}
	restored, err := tmux.Restore(context.Background(), current)
	if err != nil {
		fmt.Fprintln(os.Stderr, "restore tmux status:", err)
		return 1
	}
	if restored {
		fmt.Printf("Слушателя уже не было, но строка состояния сессии %q осталась занятой — вернул.\n", current)
		return 0
	}
	fmt.Fprintf(os.Stderr, "nothing to stop: hey-claudex is not listening in %q\n", current)
	return 1
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
	for _, name := range installNames {
		target := filepath.Join(home, ".local", "bin", name)
		if current, err := filepath.EvalSymlinks(target); err == nil && current == executable {
			if err := os.Remove(target); err != nil {
				fmt.Fprintln(os.Stderr, "remove developer symlink:", err)
				return 1
			}
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
	if key := strings.TrimSpace(os.Getenv("HEY_CLAUDEX_OPENAI_API_KEY")); key != "" {
		return key, nil
	}
	key, err := secret.Load(keychainService)
	if err == nil && strings.TrimSpace(key) != "" {
		return key, nil
	}
	return "", errors.New("OpenAI API key missing; run hey-claudex setup-api-key")
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
