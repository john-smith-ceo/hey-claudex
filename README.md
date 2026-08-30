# hey-codex

Voice input for [Codex](https://openai.com/codex/) in one explicit tmux pane.
`hey-codex` records speech, sends the audio to the OpenAI Transcription API,
and pastes the returned text into Codex. It never presses Enter: you review
the text before submitting it.

## Platform support

| Platform | Input | Credential storage | Status |
| --- | --- | --- | --- |
| macOS | Global Right Option hotkey | macOS Keychain | Supported |
| Linux | `hey-codex record` from a second terminal | `HEY_CODEX_OPENAI_API_KEY` environment variable | Beta |

Linux deliberately does not claim a global hotkey. Wayland and X11 have
incompatible security and input models; an explicit terminal command works on
both without root privileges or keylogging permissions.

## Install a release

Install `ffmpeg`, `tmux`, and `curl` first. On macOS:

```sh
brew install ffmpeg tmux
```

On Debian or Ubuntu:

```sh
sudo apt update && sudo apt install ffmpeg tmux curl
```

Then download the latest verified binary:

```sh
curl -fsSL https://raw.githubusercontent.com/john-smith-ceo/hey-codex/main/scripts/install.sh | sh
```

The installer downloads the correct release asset, verifies its SHA-256
checksum, and writes `hey-codex` to `~/.local/bin`. Ensure that directory is on
your `PATH`.

To install a specific version instead:

```sh
curl -fsSL https://raw.githubusercontent.com/john-smith-ceo/hey-codex/main/scripts/install.sh | HEY_CODEX_VERSION=0.1.0 sh
```

## macOS quick start

```sh
hey-codex setup-api-key
hey-codex doctor
hey-codex
```

The launcher needs Microphone permission. Press Right Option once to start
recording and once again to stop; after speech starts, two seconds of silence
also stops the recording. Use `--mode push` to record only while holding Right
Option.

```sh
hey-codex start --mode push
```

## Linux quick start

```sh
export HEY_CODEX_OPENAI_API_KEY='your-api-key'
hey-codex doctor
hey-codex
```

`hey-codex` opens the Codex tmux session. From a second terminal, record one
request and deliver it to the session:

```sh
hey-codex record
```

Recording uses the default PipeWire/PulseAudio microphone and ends after two
seconds of silence. To target another pane or adjust the silence timeout:

```sh
hey-codex record --tmux-target my-session:0.0 --silence 3s
```

## Commands

```text
hey-codex [start] [--mode tap|push] [-- <Codex flags...>]
hey-codex record [--silence 2s] [--device default] [--tmux-target hey-codex:0.0]
hey-codex doctor [--verify-api]
hey-codex setup-api-key [--env-file /absolute/path/to/.env]  # macOS
hey-codex stop
hey-codex uninstall [--purge-key]
```

Pass Codex flags after `--`, without shell interpolation:

```sh
hey-codex -- --approve-for-me
hey-codex start --mode push -- --model gpt-5.4
```

## Privacy and safety

- Audio is uploaded to OpenAI only after recording stops.
- Temporary WAV files are removed after success or failure.
- The target tmux pane is explicit; no active-window lookup, clipboard access,
  GUI focus control, or virtual keystrokes are used.
- `hey-codex` does not submit the pasted text.

## Development and releases

```sh
go test ./...
go build ./cmd/hey-codex
```

GitHub Actions tests on macOS and Linux. Pushing a `v*` tag creates a GitHub
Release with macOS and Linux binaries plus `checksums.txt`.

Homebrew is a macOS distribution channel, not the Linux installation path. A
tap formula is kept in `packaging/homebrew/`; publish it after the first GitHub
Release so its version and SHA-256 point at a real immutable tag.

## License

[MIT](LICENSE)
