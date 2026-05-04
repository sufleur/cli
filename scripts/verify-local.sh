#!/usr/bin/env bash
# Install the wheel matching the current host platform into a throwaway venv
# and confirm `sufleur --help` runs. Used by `make release-local`.
set -euo pipefail

VERSION="${1:?usage: $0 VERSION}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

case "$(uname -s):$(uname -m)" in
  Darwin:arm64)  TAG="macosx_11_0_arm64" ;;
  Darwin:x86_64) TAG="macosx_11_0_x86_64" ;;
  Linux:aarch64) TAG="manylinux_2_17_aarch64.manylinux2014_aarch64" ;;
  Linux:x86_64)  TAG="manylinux_2_17_x86_64.manylinux2014_x86_64" ;;
  *) echo "unsupported host: $(uname -sm)"; exit 1 ;;
esac

# Hatch normalizes hyphens in distribution names to underscores for wheel filenames.
WHEEL="$REPO_ROOT/wrappers/pip/dist/sufleur_cli-${VERSION}-py3-none-${TAG}.whl"
[ -f "$WHEEL" ] || { echo "missing $WHEEL"; ls "$REPO_ROOT/wrappers/pip/dist/" >&2; exit 1; }

VENV_DIR="$(mktemp -d)/sufleur-verify-venv"
trap 'rm -rf "$(dirname "$VENV_DIR")"' EXIT

python3 -m venv "$VENV_DIR"
"$VENV_DIR/bin/pip" install --quiet --disable-pip-version-check "$WHEEL"
"$VENV_DIR/bin/sufleur" --help > /dev/null
echo "::: pip wheel install verified: $(basename "$WHEEL") → sufleur --help ran cleanly"
