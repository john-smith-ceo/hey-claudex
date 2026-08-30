#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"
go build -o bin/hey-codex ./cmd/hey-codex
./bin/hey-codex install
echo "Installed. Next: hey-codex setup-api-key, then run: hey-codex"
