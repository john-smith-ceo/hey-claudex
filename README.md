# hey-claudex

macOS voice input for [Codex](https://openai.com/codex/) in one explicit tmux
pane. `hey-claudex` records speech, sends audio to the OpenAI Transcription API,
and pastes the returned text into Codex. It never presses Enter: you review the
text before submitting it.

## Install

```sh
brew install ffmpeg tmux
curl -fsSL https://raw.githubusercontent.com/john-smith-ceo/hey-claudex/main/scripts/install.sh | sh
hey-claudex setup-api-key
hey-claudex doctor
hey-claudex
```

The installer downloads the correct macOS release binary, verifies its SHA-256
checksum, and writes `hey-claudex` to `~/.local/bin`. Ensure that directory is on
your `PATH`.

`setup-api-key` prompts for your OpenAI API key and stores it in the macOS login
Keychain. You can instead read it from a dotenv file:

```sh
hey-claudex setup-api-key --env-file /absolute/path/to/.env
```

Grant Microphone permission to the terminal application that launches
`hey-claudex`.

## Use

Run `hey-claudex`, then press Right Option once to start recording. Press it
again to stop, or pause for five seconds after speaking. The transcript appears
in Codex for review; you press Enter yourself.

For push-to-talk, hold Right Option while speaking:

```sh
hey-claudex start --mode push
```

## Commands

```text
hey-claudex [start] [--mode tap|push] [-- <Codex flags...>]
hey-claudex doctor [--verify-api]
hey-claudex setup-api-key [--env-file /absolute/path/to/.env]
hey-claudex stop
hey-claudex uninstall [--purge-key]
```

Pass Codex flags after `--`:

```sh
hey-claudex -- --approve-for-me
hey-claudex start --mode push -- --model gpt-5.4
```

## Privacy and safety

- Audio is uploaded to OpenAI only after recording stops.
- Temporary WAV files are removed after success or failure.
- The target tmux pane is explicit; no active-window lookup, clipboard access,
  GUI focus control, or virtual keystrokes are used.
- `hey-claudex` does not submit the pasted text.

## Development and releases

```sh
go test ./...
go build ./cmd/hey-claudex
```

GitHub Actions tests and builds macOS release binaries. Pushing a `v*` tag
creates a GitHub Release with Intel and Apple Silicon binaries plus
`checksums.txt`.

## License

[MIT](LICENSE)
