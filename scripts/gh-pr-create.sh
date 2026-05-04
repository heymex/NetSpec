#!/usr/bin/env bash
# Open a PR for a branch that already exists on origin, without checking it out
# locally. Use this on the dev host when NetSpec-dev has uncommitted edits and
# you still want gh to target the pushed feature branch.
#
# Usage:
#   scripts/gh-pr-create.sh <head-branch> [title] [body]
#
# Examples:
#   scripts/gh-pr-create.sh feature/apprise-alerting-debug
#   scripts/gh-pr-create.sh feature/foo "Short title" "Longer PR description."
#
# Environment:
#   NETSPEC_ROOT   — git clone (default: repo root when run from NetSpec, else ~/NetSpec-dev)
#   NETSPEC_GH_REPO — owner/repo (default: heymex/NetSpec)

set -euo pipefail

HEAD="${1:?usage: $0 <head-branch> [title] [body]}"
TITLE="${2:-$HEAD}"
BODY="${3:-PR from branch ${HEAD}.}"

if ROOT=$(git rev-parse --show-toplevel 2>/dev/null); then
	:
else
	ROOT="${NETSPEC_ROOT:-${HOME}/NetSpec-dev}"
fi
ROOT="${NETSPEC_ROOT:-$ROOT}"
REPO="${NETSPEC_GH_REPO:-heymex/NetSpec}"

cd "$ROOT"
git fetch origin

exec gh pr create --repo "$REPO" --base main --head "$HEAD" --title "$TITLE" --body "$BODY"
