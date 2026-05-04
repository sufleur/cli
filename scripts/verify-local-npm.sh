#!/usr/bin/env bash
# Exercise wrappers/npm/install.js end-to-end against a local HTTP mirror of
# dist/, without touching the real GitHub Releases or the user's global
# node_modules. Used by `make release-local`.
#
# 1. Stage dist/v$VERSION/ with the snapshot archives + checksums.txt at the
#    URL paths install.js expects.
# 2. Start `python3 -m http.server` in the background.
# 3. `npm pack` the wrapper, install the tarball into a sandbox prefix with
#    SUFLEUR_BINARY_MIRROR pointing at the local server.
# 4. Run the installed `sufleur --help` to prove the postinstall download +
#    sha256 verify + extract path worked.
# 5. Tear down: kill server, remove sandbox + staged mirror dir + .tgz.
set -euo pipefail

VERSION="${1:?usage: $0 VERSION}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$REPO_ROOT/dist"
NPM_DIR="$REPO_ROOT/wrappers/npm"
MIRROR_DIR="$DIST/v$VERSION"

[ -f "$DIST/checksums.txt" ] || { echo "missing $DIST/checksums.txt — run goreleaser snapshot first" >&2; exit 1; }

# Stage the mirror layout that install.js fetches: ${BASE}/v${VERSION}/${archive}
# and ${BASE}/v${VERSION}/checksums.txt. Symlinks keep this cheap and avoid
# duplicating ~60 MB of binaries.
rm -rf "$MIRROR_DIR"
mkdir -p "$MIRROR_DIR"
for f in "$DIST"/sufleur_${VERSION}_*.tar.gz "$DIST"/sufleur_${VERSION}_*.zip; do
  [ -f "$f" ] && ln -sf "$f" "$MIRROR_DIR/$(basename "$f")"
done
ln -sf "$DIST/checksums.txt" "$MIRROR_DIR/checksums.txt"

# Pick a free port so concurrent runs don't collide.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()')"

# Background HTTP server; killed by the trap below. `disown` keeps bash from
# printing a "Terminated" job-status line when the trap kills it on exit.
python3 -m http.server "$PORT" --directory "$DIST" --bind 127.0.0.1 > /dev/null 2>&1 &
SERVER_PID=$!
disown "$SERVER_PID" 2>/dev/null || true

SANDBOX="$(mktemp -d)/npm-sandbox"
mkdir -p "$SANDBOX"
TGZ=""

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -rf "$(dirname "$SANDBOX")" "$MIRROR_DIR"
  [ -n "$TGZ" ] && rm -f "$NPM_DIR/$TGZ" || true
}
trap cleanup EXIT

# Wait for the server to accept connections (typically <100ms).
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$PORT/checksums.txt" > /dev/null 2>&1; then break; fi
  sleep 0.1
done

# `npm pack` prints the .tgz filename on the last line of stdout.
TGZ="$(cd "$NPM_DIR" && npm pack --silent 2>/dev/null | tail -1)"
[ -f "$NPM_DIR/$TGZ" ] || { echo "npm pack did not produce a tarball" >&2; exit 1; }

# Install the tarball into the sandbox. Setting the cache to a sandboxed path
# avoids polluting ~/.npm with the staged tarball.
SUFLEUR_BINARY_MIRROR="http://127.0.0.1:$PORT" \
  npm install --prefix "$SANDBOX" --cache "$SANDBOX/.npm-cache" --silent --no-audit --no-fund "$NPM_DIR/$TGZ"

# The bin shim ends up at $SANDBOX/node_modules/.bin/sufleur for non-global installs.
BIN="$SANDBOX/node_modules/.bin/sufleur"
[ -x "$BIN" ] || { echo "missing $BIN after npm install" >&2; ls -la "$SANDBOX/node_modules/.bin/" >&2 || true; exit 1; }

"$BIN" --help > /dev/null
echo "::: npm tarball install verified: postinstall fetched binary from local mirror and sufleur --help ran cleanly"
