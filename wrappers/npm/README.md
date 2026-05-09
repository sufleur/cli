# @sufleur/cli

Type-safe codegen for versioned LLM prompts. Manage prompts the way `npm` manages packages — declare them in `sufleur.yaml`, lock to `sufleur-lock.yaml`, generate one TypeScript file with full types and runtime helpers.

## What you call from your code

```ts
import { getPrompt } from './generated/prompts';

const review = getPrompt('@my-workspace/code-review');

const { prompt } = review.render('en', {
  diff: '...',
  language: 'go',
});
// → ready-to-send prompt string

const result = review.parseOutput(llmResponseText);
if (result.success) {
  result.data; // typed by the prompt's output schema (Zod-validated)
} else {
  result.error;
}
```

`'@my-workspace/code-review'` is checked at compile time: typos fail to type-check, the entrypoint name `'en'` is narrowed against the prompt's available entrypoints, and the input shape is the JSON Schema declared on that entrypoint. The version that resolves at codegen time is pinned in `sufleur-lock.yaml`.

## Install

```bash
npm i -g @sufleur/cli
sufleur --help
```

Or run on demand:

```bash
npx -p @sufleur/cli sufleur --help
```

The wrapper downloads the matching prebuilt binary on `npm install` and exposes it as `sufleur`. There's no JS in the hot path — the `sufleur` command is the native binary.

## Quick start

```bash
mkdir my-app && cd my-app
sufleur init                                  # creates sufleur.yaml interactively
sufleur add @my-workspace/code-review ^1.0.0  # add + fetch + lock
sufleur generate                              # writes ./generated/prompts.ts
```

The generated file imports two runtime peers — install them in your project:

```bash
npm i mustache
npm i -D @types/mustache
# only if any prompt has an output schema:
npm i zod
```

## What `sufleur generate` emits

A single `.ts` file containing every prompt inlined (no runtime fetches). The header documents what's exported; the public API is `getPrompt(name)`, which returns:

- **`render(entrypoint, input)` → `{ prompt: string }`** — Mustache renders the entrypoint template against `input`. The input type is narrowed by entrypoint name; entrypoints with no input schema take no second argument.
- **`metadata`** — `{ version, ...your custom workspace metadata, outputSchema? }`. The pinned version comes from the lockfile; the rest comes from whatever metadata your registry assigned to that prompt version.
- **`parseOutput(raw)`** *(only present if the prompt has an output schema)* — strips ``` fences, JSON-parses, and validates with a Zod schema generated from the prompt's JSON Schema. Returns `{ success: true, data }` or `{ success: false, error }`.

Plus exported types per entrypoint:

```ts
export type CodeReview_EnInput = { diff: string; language: string };
```

Prompts published with `DRAFT` status emit a runtime `console.warn` when their `getPrompt` is called.

## sufleur.yaml

The manifest. Looks like:

```yaml
api_keys:
  my-workspace: ${MY_WORKSPACE_API_KEY}

prompts:
  '@my-workspace/greeting': '*'
  '@my-workspace/code-review': '^2.0.0'
  # alias: keep two pinned versions side-by-side under different names
  '@my-workspace/code-review-strict': '@my-workspace/code-review@~1.4.0'

output:
  language: typescript
  file: ./generated/prompts.ts
```

Constraints are npm-style semver ranges (`^`, `~`, `>=`, exact, `*`). The resolution is recorded in `sufleur-lock.yaml`. **Commit both files** — `sufleur.yaml` is the source of truth, `sufleur-lock.yaml` is the receipt.

## CI usage

```bash
sufleur install --frozen   # fail if lockfile is stale
sufleur generate
```

`--frozen` is the npm-`ci` equivalent: refuses to update the lockfile, hard-errors if the manifest and lockfile disagree.

## Commands

| Command | Description |
| ------- | ----------- |
| `sufleur init` | Interactive scaffolding for `sufleur.yaml`. |
| `sufleur add @ws/name [range]` | Add a prompt, fetch it, update the lockfile. `--alias <name>` keeps multiple versions; `--force` overwrites an existing entry. |
| `sufleur remove @ws/name` | Remove a prompt from the manifest and prune its cache (kept if another alias still resolves to the same version). |
| `sufleur install` | Resolve the manifest, fetch what's missing, refresh the lockfile. `--frozen` for CI. |
| `sufleur update [@ws/name]` | Re-resolve constraints — one prompt or all. |
| `sufleur generate` | Regenerate the output file from the lockfile + cache. |

`-v` / `--verbose` enables HTTP request/response logs on any command. Variables in `.env` are loaded automatically; per-workspace API keys can be referenced as `${ENV_VAR_NAME}` in `sufleur.yaml`.

## Supported platforms

| OS      | Architectures                  |
| ------- | ------------------------------ |
| macOS   | x64, arm64                     |
| Linux   | x64, arm64                     |
| Windows | x64, arm64 (Windows 10 1803+)  |

Alpine / musl libc is currently unsupported (no musllinux binary). Override the binary download URL with `SUFLEUR_BINARY_MIRROR`. Set `SUFLEUR_SKIP_POSTINSTALL=1` to defer the download (e.g. when building an image you'll rehydrate later with `npm rebuild @sufleur/cli`).

## Source

Source code, issue tracker, release notes: <https://github.com/sufleur/cli>.

## License

MIT.
