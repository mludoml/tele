#!/usr/bin/env bash
# Rebuilds the `mine` branch as `main` + a merge of the active topic
# branches below. Run this after rebasing a topic branch on `main`, or
# after adding/removing a topic from the list.
#
# To permanently drop a feature from your local build: comment out its
# line and re-run. The topic branch itself is untouched, so you can bring
# it back later just by uncommenting.
#
# This script assumes `main` and every topic branch are already up to
# date (see AGENTS.md for the upstream-sync steps). It does not fetch or
# rebase anything itself.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

TOPICS=(
  feat/qr-login
  feat/dev-workflow
  feat/ui
  feat/keybindings-remap
  feat/message-translation
  feat/rich-messages
)

if [[ ${#TOPICS[@]} -eq 0 ]]; then
  echo "no active topic branches — nothing to merge" >&2
fi

git checkout -B mine main
for topic in "${TOPICS[@]}"; do
  echo "merging $topic..."
  git merge --no-ff --no-edit "$topic"
done

echo
echo "mine rebuilt at $(git rev-parse --short mine). Review with:"
echo "  git log --oneline main..mine"
echo "Push it with:"
echo "  git push --force-with-lease origin mine"
