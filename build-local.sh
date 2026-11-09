#!/usr/bin/env bash
# Build this checkout's tele and install it as ~/.local/bin/tele-v2, beside
# the v1 ~/.local/bin/tele, so the v2 line can be tried without replacing
# the working client. At cutover, set BIN back to tele.
set -euo pipefail
cd "$(dirname "$0")"
BIN="$HOME/.local/bin/tele-v2"
go build -o "$BIN" ./cmd/tele
echo "Installed: $("$BIN" --version 2>&1 || true) -> $BIN"
