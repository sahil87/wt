#!/usr/bin/env bash
set -euo pipefail

./scripts/build.sh

DEST="${HOME}/.local/bin/wt"
mkdir -p "$(dirname "$DEST")"
cp -f ./bin/wt "$DEST"
echo "installed: $DEST"
