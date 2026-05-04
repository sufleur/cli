# @sufleur/cli

npm wrapper for the [`sufleur`](https://github.com/WTomas/sufleur-cli) CLI — type-safe codegen for versioned LLM prompts.

The wrapper downloads the matching prebuilt binary from GitHub Releases on `npm install` and exposes it as the `sufleur` command.

## Install

```bash
npm i -g @sufleur/cli
sufleur --help
```

Or run on demand:

```bash
npx sufleur --help
```

## Supported platforms

| OS      | Architectures   |
| ------- | --------------- |
| macOS   | x64, arm64      |
| Linux   | x64, arm64      |
| Windows | x64, arm64 (Windows 10 1803+) |

## Peer dependencies

This package declares `mustache` and `zod` as required peer dependencies. The CLI itself doesn't import them, but the TypeScript code emitted by `sufleur generate` does — so listing them as peers nudges your project to install them before runtime.

- npm 7+ installs peer dependencies automatically.
- npm 6 only warns. If you see a peer warning, run `npm i mustache zod`.

## Environment variables

| Variable | Purpose |
| -------- | ------- |
| `SUFLEUR_BINARY_MIRROR` | Override the binary download base URL. The installer fetches `${MIRROR}/v${version}/${archive}` and `${MIRROR}/v${version}/checksums.txt`. Default: `https://github.com/WTomas/sufleur-cli/releases/download`. |
| `SUFLEUR_SKIP_POSTINSTALL` | Set to `1` to skip the binary download. The `sufleur` command will print recovery instructions on first invocation. |

## Bypassing postinstall

If you install with `--ignore-scripts`, the binary won't be downloaded. Populate it manually:

```bash
node $(npm root -g)/@sufleur/cli/install.js
```

For an offline `npm install`, the postinstall detects `npm_config_offline=true` and exits cleanly. Once online:

```bash
npm rebuild @sufleur/cli
```

## How it works

1. `postinstall` runs `install.js`, which detects your platform/arch and computes the matching archive name (e.g., `sufleur_1.2.3_darwin_arm64.tar.gz`).
2. It downloads `checksums.txt` and the archive from GitHub Releases (or your configured mirror), verifying the SHA256 in-stream.
3. It extracts the binary into `bin/` and sets the executable bit.
4. `run.js` (the package's `bin` entrypoint) execs the binary with your arguments and inherits stdio.

## Source

Source code, issue tracker, and release notes: <https://github.com/WTomas/sufleur-cli>.

## License

MIT.
