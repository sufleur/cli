# @sufleur/cli

The CLI for [**Sufleur**](https://sufleur.com) — the registry where you author, version, and publish LLM prompts. This is the consumer side: it installs prompts from your Sufleur workspace into your project the way `npm` installs packages — declared in `sufleur.yaml`, locked to `sufleur-lock.yaml`, generated into one TypeScript file with full types and runtime helpers.

Create a workspace and start authoring prompts at <https://sufleur.com>.

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

Optional schema properties are emitted with `?:`, and `oneOf` schemas become TypeScript union types.

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

## Authoring prompts from the CLI

The commands above install **published** prompts into your project. The CLI also exposes the full authoring side — designed so a coding agent (Claude Code, Cursor, etc.) can create, version, and edit prompts in your Sufleur workspace on your behalf.

### Hand it to your agent

`sufleur skill` prints a markdown skill description — when to use the CLI, FQ-name format, the full command surface, JSON flags. Pipe it wherever your agent loads skills from:

```bash
# Claude Code (each skill is a directory with a SKILL.md inside)
mkdir -p ~/.claude/skills/sufleur && sufleur skill > ~/.claude/skills/sufleur/SKILL.md

# Cursor
sufleur skill > .cursor/rules/sufleur.md
```

The skill ships inside the binary, so it always matches the `sufleur` version on your `PATH`.

### Log in

```bash
sufleur login    # device-code flow — opens a browser, polls until approved
sufleur me       # show the authenticated user
sufleur logout   # revoke the stored credential
```

Credentials land in `$XDG_CONFIG_HOME/sufleur/credentials.yaml` (or `~/.config/sufleur/credentials.yaml`). This user credential is separate from the workspace API keys referenced in `sufleur.yaml` — those stay machine-to-machine, this one identifies you as the author.

### Authoring commands

All accept `--json`. Prompts are addressed as `@workspace/name`, versions as `@workspace/name@version` (use the literal label `draft` while the version is unpublished).

| Command | What it does |
| ------- | ------------ |
| `workspace list` | List the workspaces you belong to, with your role |
| `prompt create @ws/name --description "..."` | Create a new prompt in a workspace |
| `prompt list @ws [--search ... --limit ... --offset ...]` | List prompts in a workspace |
| `prompt get @ws/name` | Show one prompt's details |
| `prompt update @ws/name --description "..."` | Update the description |
| `version draft @ws/name` | Fork the latest published version into a new draft |
| `version list @ws/name [--status DRAFT\|PUBLISHED]` | List versions of a prompt |
| `version get @ws/name@version` | Show one version's details |
| `version delete @ws/name@draft` | Delete a draft (published versions are immutable) |
| `version set-metadata @ws/name@draft --string K=V` (or `--from-file …`) | Patch or sync metadata |
| `version delete-metadata @ws/name@draft --key K` | Remove a metadata key |
| `version set-output-schema @ws/name@draft --file schema.json` | Replace the version's output schema |
| `version set-readme @ws/name@draft [--content STR \| --file PATH]` | Replace the version's README |
| `version get-readme @ws/name@version` | Print the version's README to stdout (raw markdown) |
| `version dump @ws/name@version --to ./dir` | Export files, output schema, README, and metadata to disk |
| `file list @ws/name@version` | List files in a version |
| `file create @ws/name@draft --file path.mustache [--entrypoint]` | Add a new file |
| `file update @ws/name@draft --name X [--file ...] [--rename Y]` | Replace content and/or rename |
| `file delete @ws/name@draft --name X` | Delete a file |
| `file set-entrypoint @ws/name@draft --name X [--clear]` | Mark (or unmark) a file as an entrypoint |
| `eval get @ws/name@version [--file PATH]` | Print the version's eval YAML (skeleton if none) |
| `eval validate @ws/name@version --file PATH` | Parse + type-check an eval YAML; saves nothing |
| `eval push @ws/name@version --file PATH` | Validate, then save the eval |
| `eval delete @ws/name@version` | Remove the eval from a version |
| `eval run @ws/name@version [--watch]` | Enqueue a run; `--watch` streams to completion (CI gate) |
| `eval runs @ws/name@version` | List recent runs, newest first |
| `eval show <run-id>` / `eval watch <run-id>` | Inspect or follow a single run |
| `dataset create @ws/name [--description ...]` | Create a dataset and its initial draft (private) |
| `dataset list @ws` / `dataset get @ws/name` | List datasets, or show one with its versions |
| `dataset update @ws/name --description "..."` | Edit the description |
| `dataset dump @ws/name@version --to ./dir` | Export `schema.json`, `cases.jsonl`, and `dataset.yaml` to a directory |
| `dataset cases push @ws/name@draft --file cases.jsonl` | Upload cases (JSONL/JSON/CSV); schema inferred on first upload |
| `dataset cases pull @ws/name@version [--to cases.jsonl]` | Download a version's cases as JSONL |
| `dataset schema get / set @ws/name@version [--file PATH]` | Read or refine a draft's JSON Schema |
| `dataset version draft @ws/name` | New draft, carrying forward the latest published schema + cases |
| `dataset version list @ws/name [--status DRAFT\|PUBLISHED]` | List a dataset's versions |
| `dataset version validate @ws/name@draft` | Check every case against the schema (non-zero exit on a violation) |

An **eval** scores a prompt version against a **dataset** — judges, CEL assertions, and a passing threshold. Datasets are versioned collections of test cases, authored from the CLI too (the `dataset …` commands above) and referenced from the eval YAML via `dataset.ref`. Like prompts, **publishing a dataset version and changing its visibility are done in the web app**. Full guides: <https://sufleur.com/docs/evals> and <https://sufleur.com/docs/datasets>.

### Render before publishing

`sufleur prompt render <dir> --entrypoint <name> [--vars '{...}' | --vars-file path.json]` runs the same Mustache pipeline as the generated runtime — useful for previewing a draft locally before publishing, or for quick experimentation against a `version dump` directory. No auth required.

## Supported platforms

| OS      | Architectures                  |
| ------- | ------------------------------ |
| macOS   | x64, arm64                     |
| Linux   | x64, arm64                     |
| Windows | x64, arm64 (Windows 10 1803+)  |

Alpine / musl libc is currently unsupported (no musllinux binary). Override the binary download URL with `SUFLEUR_BINARY_MIRROR`. Set `SUFLEUR_SKIP_POSTINSTALL=1` to defer the download (e.g. when building an image you'll rehydrate later with `npm rebuild @sufleur/cli`).

## Links

- **Sufleur platform** — author and manage prompts: <https://sufleur.com>
- **Source code, issues, release notes**: <https://github.com/sufleur/cli>

## License

MIT.
