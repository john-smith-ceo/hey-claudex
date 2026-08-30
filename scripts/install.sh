#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"
if ! command -v tmux >/dev/null 2>&1; then
  echo "tmux is required; install it first: brew install tmux" >&2
  exit 1
fi
go build -o bin/hey-codex ./cmd/hey-codex
./bin/hey-codex install
echo "Installed. Next: hey-codex setup-api-key, then run: hey-codex"
