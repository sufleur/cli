#!/usr/bin/env bash
set -euo pipefail

TARGETS=(darwin_amd64 darwin_arm64 linux_amd64 linux_arm64 windows_amd64 windows_arm64)
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -d "$REPO_ROOT/dist" ]; then
  echo "error: $REPO_ROOT/dist does not exist; run goreleaser first" >&2
  exit 1
fi

for target in "${TARGETS[@]}"; do
  echo "::: building wheel for $target"
  (cd "$REPO_ROOT/wrappers/pip" && SUFLEUR_TARGET="$target" hatch build -t wheel)
done

echo "::: built $(ls -1 "$REPO_ROOT/wrappers/pip/dist/"*.whl | wc -l | tr -d ' ') wheels in wrappers/pip/dist/"
