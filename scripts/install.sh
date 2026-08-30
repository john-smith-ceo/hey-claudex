#!/bin/sh
set -eu

repo=${HEY_CODEX_REPOSITORY:-john-smith-ceo/hey-codex}
version=${HEY_CODEX_VERSION:-latest}
install_dir=${HEY_CODEX_INSTALL_DIR:-"$HOME/.local/bin"}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "hey-codex installer requires $1" >&2
    exit 1
  fi
}

need curl
need tmux
need ffmpeg

if [ "$(uname -s)" != Darwin ]; then
  echo "hey-codex currently supports macOS only" >&2
  exit 1
fi
platform=darwin

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported CPU architecture: $(uname -m)" >&2; exit 1 ;;
esac

asset="hey-codex_${platform}_${arch}"
if [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/v$version"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT HUP INT TERM

curl --fail --location --silent --show-error "$base/$asset" -o "$workdir/$asset"
curl --fail --location --silent --show-error "$base/checksums.txt" -o "$workdir/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$workdir" && grep " $asset$" checksums.txt | sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  expected=$(grep " $asset$" "$workdir/checksums.txt" | awk '{print $1}')
  actual=$(shasum -a 256 "$workdir/$asset" | awk '{print $1}')
  [ "$expected" = "$actual" ] || { echo "checksum verification failed" >&2; exit 1; }
else
  echo "need sha256sum or shasum to verify the downloaded binary" >&2
  exit 1
fi

mkdir -p "$install_dir"
install -m 0755 "$workdir/$asset" "$install_dir/hey-codex"
echo "installed $install_dir/hey-codex"
echo "ensure $install_dir is on PATH, then run: hey-codex doctor"
