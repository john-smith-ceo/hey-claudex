# hey-codex

`hey-codex` is a macOS push-to-talk bridge for terminal AI agents. It records from the microphone, transcribes with OpenAI, and pastes the resulting text into the active application. It never presses Enter.

## Status

v0.1 is macOS-first and uses only the Go standard library plus system tools:

- global **Right Option** hotkey, intercepted before it reaches the focused application;
- `tap` mode: press once to start and again to stop; after speech has started, two seconds of silence also stops recording;
- `push` mode: hold Right Option to record and release it to stop;
- `ffmpeg` / AVFoundation records a temporary WAV file;
- `gpt-transcribe` transcribes it via the OpenAI Audio API;
- `pbcopy` + macOS Accessibility paste the text, without sending it.

Audio is sent to OpenAI only after recording stops. Temporary audio is removed after a successful or failed transcription attempt.

## Install prerequisites

```bash
brew install ffmpeg
```

The application which launches `hey-codex` needs **Microphone** permission. `hey-codex` also needs **Accessibility** permission to paste into the active terminal.

## Build and run

```bash
go build -o bin/hey-codex ./cmd/hey-codex
./bin/hey-codex doctor
./bin/hey-codex install
./bin/hey-codex setup-api-key --env-file /absolute/path/to/.env
./bin/hey-codex run --mode tap --silence 2s
```

`setup-api-key` stores the OpenAI API key in the macOS login Keychain under service `hey-codex.openai-api-key`. It can read `OPENAI_API_KEY` directly from a dotenv file without printing it. For one-off runs, `HEY_CODEX_OPENAI_API_KEY` takes precedence.

## Commands

```text
hey-codex doctor
hey-codex doctor --verify-api
hey-codex install
hey-codex setup-api-key [--env-file /path/to/.env]
hey-codex run [--mode tap|push] [--silence 2s] [--device :0]
```

Press `Ctrl+C` to stop the background listener.

`doctor --verify-api` checks authenticated model availability without recording or uploading audio.

## Safety

- Right Option is consumed by the event tap; it is not delivered to the terminal.
- No text is sent automatically. Review it in the Codex composer, then press Enter yourself.
- Clipboard contents are replaced only after a successful transcription.
- The recorder works only while `hey-codex run` is active.
