#!/usr/bin/env bash
# Build this branch's tele and install it as the active ~/.local/bin/tele.
# Run after every `git rebase main` to pick up the latest patch + upstream code.
set -euo pipefail
cd "$(dirname "$0")"
go build -o "$HOME/.local/bin/tele" ./cmd/tele
echo "Installed: $("$HOME/.local/bin/tele" --version 2>&1 || true) -> $HOME/.local/bin/tele"
