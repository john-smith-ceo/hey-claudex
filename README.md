# hey-codex

`hey-codex` is a macOS push-to-talk bridge for Codex in tmux. It records from the microphone, transcribes with OpenAI, and delivers the resulting text to one explicit tmux pane. It never presses Enter.

## Status

v0.1 is macOS-first and uses only the Go standard library plus system tools:

- global **Right Option** hotkey, intercepted before it reaches the focused application;
- `tap` mode: press once to start and again to stop; after speech has started, two seconds of silence also stops recording;
- `push` mode: hold Right Option to record and release it to stop;
- `ffmpeg` / AVFoundation records a temporary WAV file;
- `gpt-transcribe` transcribes it via the OpenAI Audio API;
- `tmux load-buffer` + `tmux paste-buffer` deliver text to one explicit pane, without GUI focus or virtual keystrokes.
- a menu-bar microphone shows the current state: ready, recording, transcribing, pasted, or error.

Audio is sent to OpenAI only after recording stops. Temporary audio is removed after a successful or failed transcription attempt.

## Install prerequisites

```bash
brew install ffmpeg
```

The application which launches `hey-codex` needs **Microphone** permission. It does not require Accessibility permission because delivery is handled by tmux, not GUI automation.

## Install from source

```bash
go build -o bin/hey-codex ./cmd/hey-codex
./bin/hey-codex doctor
./bin/hey-codex install
./bin/hey-codex setup-api-key --env-file /absolute/path/to/.env
./bin/hey-codex
```

`setup-api-key` stores the OpenAI API key in the macOS login Keychain under service `hey-codex.openai-api-key`. It can read `OPENAI_API_KEY` directly from a dotenv file without printing it. For one-off runs, `HEY_CODEX_OPENAI_API_KEY` takes precedence.

## Homebrew release

The release process publishes `packaging/homebrew/hey-codex.rb.tmpl` into the project's Homebrew tap after replacing `OWNER`, `VERSION`, and `SHA256` with the GitHub release values. The installed user experience is deliberately short:

```bash
brew install <owner>/tap/hey-codex
hey-codex setup-api-key
hey-codex
```

Removal is equally explicit:

```bash
hey-codex stop
brew uninstall hey-codex
```

The Keychain secret is preserved on normal uninstall; remove it only with `hey-codex uninstall --purge-key`.

## Commands

```text
hey-codex doctor
hey-codex doctor --verify-api
hey-codex install
hey-codex setup-api-key [--env-file /path/to/.env]
hey-codex [start] [--mode tap|push]
hey-codex stop
hey-codex uninstall [--purge-key]
hey-codex run [--mode tap|push] [--silence 2s] [--device :default]
```

Press `Ctrl+C` to stop the background listener.

For everyday work, run `hey-codex` with no arguments. It creates or resumes its own tmux session, starts Codex plus the voice listener, and attaches the terminal. `hey-codex stop` ends both deliberately.

`doctor --verify-api` checks authenticated model availability without recording or uploading audio.

## Safety

- Right Option is consumed by the event tap; it is not delivered to the terminal.
- No text is sent automatically. Review it in the Codex input, then press Enter yourself.
- The destination pane is explicit; no active-window or clipboard access is used.
- The recorder works only while `hey-codex run` is active.
