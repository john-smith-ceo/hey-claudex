# hey-claudex

Voice input for the tmux pane you are already working in. `hey-claudex` records
speech, sends the audio to the OpenAI Transcription API and pastes the text
into your pane. It starts nothing and launches nothing: your assistant is
already running, and the transcription goes to it.

macOS and Linux (X11).

## Names

The tool answers to three names, all symlinks to one binary:

| Name | Expects | Purpose |
|---|---|---|
| `hey-claude` | Claude Code in the pane | refuses if something else is running there |
| `hey-codex` | Codex in the pane | same check, other assistant |
| `hey-claudex` | anything | no check at all |

The check exists because a transcription landing in the wrong window is worse
than one that never arrived. `--any` overrides it.

## Install

### Codex plugin

После обычной установки бинарника добавьте marketplace из GitHub и включите
плагин:

```sh
codex plugin marketplace add john-smith-ceo/hey-claudex --ref main
codex plugin add hey-codex@hey-claudex
```

В панели tmux, где уже работает Codex, вызовите `/hey-codex`. Плагин включит
слушатель для этой же панели: не нужен отдельный target, соседняя панель или
другой assistant. Остановить: `hey-claudex stop`.

Плагин управляет уже установленным `hey-codex`; `ffmpeg`, `tmux`, доступ к
микрофону и сам бинарник всё ещё устанавливаются по инструкции ниже.

### macOS

```sh
brew install ffmpeg tmux
git clone https://github.com/john-smith-ceo/hey-claudex
cd hey-claudex && go build -o bin/hey-claudex ./cmd/hey-claudex && ./bin/hey-claudex install
```

Grant Microphone and Accessibility permission to the terminal application that
launches it — the key is captured through a CoreGraphics event tap.

### Linux

Needs an X11 session; Wayland is not supported yet. The key is captured through
XRecord, which requires the Xlib headers at build time:

```sh
sudo apt install ffmpeg tmux libx11-dev libxtst-dev
git clone https://github.com/john-smith-ceo/hey-claudex
cd hey-claudex && go build -o bin/hey-claudex ./cmd/hey-claudex && ./bin/hey-claudex install
```

`install` links all three names into `~/.local/bin`.

### First run

```sh
hey-claudex setup-api-key      # macOS Keychain, or ~/.config/hey-claudex on Linux
hey-claudex doctor             # ffmpeg, tmux, hotkey, key
```

`setup-api-key --env-file /path/to/.env` reads `OPENAI_API_KEY` or
`OPEN_AI_API_KEY` from a dotenv file instead of prompting.

## Use

From the pane where your assistant runs:

```sh
hey-claude
```

Press Right Alt (Right Option on macOS), speak, then press it again — or simply
pause. The text appears in the input line and **is not submitted**: you read it
and press Enter yourself.

```sh
hey-claude --submit              # send it as soon as the recording ends
hey-claude --mode push           # record only while the key is held
hey-claude --key Shift_L         # a different key
hey-claude --silence 2s          # a shorter pause ends the recording sooner
hey-claudex stop                 # stop listening
hey-claudex keys                 # every selectable key
```

`--submit` is deliberately independent of `--mode`: how a recording starts and
what happens to its result are separate questions, and holding a key, releasing
it and watching the text go is the most useful combination of the two.

Between the paste and the Enter there is a quarter-second pause. tmux
acknowledges the paste immediately, but the receiving program is still digesting
the bracketed-paste terminator, and an Enter arriving in the same burst of
events is swallowed as pasted text. `--submit-delay` widens it for a slow
application.

## Keys

Keys fall into two groups, and the group decides what is possible.

**Free** — `Alt_R`, `Super_R`, `Menu`, `Scroll_Lock`, `Pause`, `Caps_Lock`. They
do nothing on their own, so a bare press is unambiguous and both modes work.

**Typing** — `Shift_L/R`, `Control_L/R`, `Alt_L`, `Super_L`. These are pressed
constantly while writing, so only a solo press counts: down and up with no other
key in between and within 400 ms. A capital letter never starts a recording.
Hold-to-talk is impossible for them and is refused at startup — holding Shift
is indistinguishable from ordinary typing.

macOS keyboards have no `Menu`, `Scroll_Lock` or `Pause`; asking for one there
is refused with an explanation.

On macOS a free key is swallowed and never reaches applications, the way the
original Right Option tap worked. On Linux XRecord observes the stream without
consuming it, so the key keeps its normal function — which is why the default
is a key that has none.

## Status line

State is shown at the bottom of the terminal: `mode:tap`, `rec…`,
`transcribe…`, `done`, `error`, and `mode:tap auto` when submission is on.

In a session you already had, `hey-claudex` only appends its indicator to
`status-right` and parks the previous value, restoring it when it stops. Your
theme stays yours. It also raises `status-right-length`, which defaults to 40
characters — short enough to cut the indicator off entirely.

`stop` cleans up even when the listener window died on its own: the borrowed
status line still has to be given back.

## Silence while recording

If something reads answers out loud, it ends up inside the recording. Two flags
prevent that:

```sh
hey-claude --busy-file ~/.config/jarvis-voice/mic-busy \
           --on-record "jarvis-voice hush"
```

The file says "microphone busy" to anything that watches it; the command
interrupts what is already speaking, which a file cannot do. Both also come
from `HEY_CLAUDEX_BUSY_FILE` and `HEY_CLAUDEX_ON_RECORD`.

## Privacy and safety

- Audio is uploaded only after the recording stops.
- Temporary WAV files are removed after success or failure.
- The target pane is explicit: no active-window lookup, no clipboard, no GUI
  focus control, no synthetic keystrokes beyond the optional Enter.
- Without `--submit`, nothing is ever submitted for you.
- An empty transcription is never sent.
- `stop` removes only what `hey-claudex` started. It never kills your session.

## Development

```sh
go test ./...
go build ./cmd/hey-claudex
```

## License

[MIT](LICENSE)
