#!/usr/bin/env bash
# Rebuilds the `mine-v2` branch as `main-v2` + a merge of the active topic
# branches below. Run this after rebasing a topic branch on `main-v2`, or
# after adding/removing a topic from the list.
#
# This is the v2 line (upstream 2.x), developed in the ../tele-cli-v2
# worktree alongside the v1 `main`/`mine` line. At cutover, set BASE=main,
# OUT=mine and drop the -v2 suffixes.
#
# To permanently drop a feature from your local build: comment out its
# line and re-run. The topic branch itself is untouched, so you can bring
# it back later just by uncommenting.
#
# This script assumes BASE and every topic branch are already up to
# date (see AGENTS.md for the upstream-sync steps). It does not fetch or
# rebase anything itself.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

BASE=main-v2
OUT=mine-v2

TOPICS=(
  feat/dev-workflow-v2
  feat/ui-v2
  feat/keybindings-remap-v2
  feat/message-translation-v2
  # feat/rich-messages-v2         # not yet ported to the v2 IPC surface
)

if [[ ${#TOPICS[@]} -eq 0 ]]; then
  echo "no active topic branches — nothing to merge" >&2
fi

git checkout -B "$OUT" "$BASE"
for topic in "${TOPICS[@]}"; do
  echo "merging $topic..."
  git merge --no-ff --no-edit "$topic"
done

echo
echo "$OUT rebuilt at $(git rev-parse --short "$OUT"). Review with:"
echo "  git log --oneline $BASE..$OUT"
echo "Push it with:"
echo "  git push --force-with-lease origin $OUT"
