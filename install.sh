#!/bin/sh
# Install costblame from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/abhilaksh-arora/costblame/main/install.sh | sh
#
# Override the install location with COSTBLAME_INSTALL_DIR (default ~/.local/bin).
set -eu

if [ -t 1 ]; then
  amber='\033[38;5;173m'
  dim='\033[2m'
  reset='\033[0m'
else
  amber=''
  dim=''
  reset=''
fi

printf '%b' "$amber"
cat <<'BANNER'
                   __  __    __
  _________  _____/ /_/ /_  / /___ _____ ___  ___
 / ___/ __ \/ ___/ __/ __ \/ / __ `/ __ `__ \/ _ \
/ /__/ /_/ (__  ) /_/ /_/ / / /_/ / / / / / /  __/
\___/\____/____/\__/_.___/_/\__,_/_/ /_/ /_/\___/
BANNER
printf '%b' "$reset"
printf '%battribute your AI coding spend — Claude Code, Codex, Gemini%b\n\n' "$dim" "$reset"

REPO="abhilaksh-arora/costblame"
BIN="costblame"
INSTALL_DIR="${COSTBLAME_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s)
arch=$(uname -m)

case "$os" in
  Darwin) os_name=macos ;;
  Linux)  os_name=linux ;;
  *)
    echo "costblame: unsupported OS '$os'. Grab a binary manually: https://github.com/$REPO/releases" >&2
    exit 1
    ;;
esac

case "$arch" in
  arm64|aarch64) arch_name=arm64 ;;
  x86_64|amd64)
    if [ "$os_name" = macos ]; then arch_name=intel; else arch_name=amd64; fi
    ;;
  *)
    echo "costblame: unsupported architecture '$arch'. Grab a binary manually: https://github.com/$REPO/releases" >&2
    exit 1
    ;;
esac

asset="${BIN}-${os_name}-${arch_name}"
url="https://github.com/${REPO}/releases/latest/download/${asset}.zip"

command -v curl  >/dev/null 2>&1 || { echo "costblame: curl is required" >&2; exit 1; }
command -v unzip >/dev/null 2>&1 || { echo "costblame: unzip is required" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading ${asset}..." >&2
if ! curl -fsSL "$url" -o "$tmp/${asset}.zip"; then
  echo "costblame: download failed — no release asset for ${os_name}/${arch_name} at $url" >&2
  echo "See https://github.com/${REPO}/releases for available builds." >&2
  exit 1
fi

unzip -qo "$tmp/${asset}.zip" -d "$tmp"

mkdir -p "$INSTALL_DIR"
mv "$tmp/${asset}" "$INSTALL_DIR/${BIN}"
chmod +x "$INSTALL_DIR/${BIN}"

echo "installed -> $INSTALL_DIR/$BIN" >&2

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "" >&2
    echo "note: $INSTALL_DIR is not on your PATH. Add this to your shell profile:" >&2
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" >&2
    ;;
esac

echo "" >&2
echo "run: costblame init   (pick your plan)" >&2
echo "  or costblame sync    (spend across every project)" >&2
echo "  or costblame serve   (dashboard)" >&2
