#!/usr/bin/env bash
# Build this checkout's tele and install it as ~/.local/bin/tele-v2, beside
# the v1 ~/.local/bin/tele, so the v2 line can be tried without replacing
# the working client.
#
# APP is injected as main.appName, which names every default path the binary
# uses: ~/.config/$APP, $XDG_STATE_HOME/$APP (state and logs) and the host
# socket under $XDG_RUNTIME_DIR/$APP. Without it a tele-v2 binary would read
# and write v1's config, state and logs. At cutover, set APP back to tele.
set -euo pipefail
cd "$(dirname "$0")"
APP=tele-v2
BIN="$HOME/.local/bin/$APP"
go build -ldflags "-X main.appName=$APP" -o "$BIN" ./cmd/tele
echo "Installed: $("$BIN" --version 2>&1 || true) -> $BIN (appName=$APP)"
