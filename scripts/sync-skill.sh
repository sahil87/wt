#!/usr/bin/env bash
# Copy the canonical docs/site/skill.md into src/ so it can be embedded via
# //go:embed. The Go module root is src/ and docs/site/ sits above it, so embed
# cannot reach the canonical file directly — this copy step bridges the gap
# (Constitution I: single binary, logic in scripts/). The copy is committed so a
# clean `go build ./...` (which does not run this script) compiles; the
# drift-guard test in skill_test.go keeps it byte-honest.
set -euo pipefail

# Run from the repo root regardless of caller CWD.
cd "$(dirname "$0")/.."

SRC="docs/site/skill.md"
DEST="src/cmd/wt/skill.md"

cp -f "$SRC" "$DEST"
echo "synced skill bundle: ${DEST} (from ${SRC})"
