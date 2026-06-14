# sufleur-cli

The CLI for [**Sufleur**](https://sufleur.com) — the registry where you author, version, and publish LLM prompts. This is the consumer side: it installs prompts from your Sufleur workspace into your project the way `pip` installs packages — declared in `sufleur.yaml`, locked to `sufleur-lock.yaml`, generated into one Python module with full types and runtime helpers.

Create a workspace and start authoring prompts at <https://sufleur.com>.

## What you call from your code

```python
from generated.prompts import get_prompt

review = get_prompt("@my-workspace/code-review")

rendered = review.render("en", {"diff": "...", "language": "go"})
prompt: str = rendered["prompt"]  # ready-to-send prompt string

result = review.parse_output(llm_response_text)
if result["success"]:
    result["data"]  # Pydantic model, validated against the prompt's output schema
else:
    result["error"]
```

`"@my-workspace/code-review"` is checked at type-check time (mypy / pyright): typos fail, the entrypoint name `"en"` is narrowed against the prompt's available entrypoints (via `@overload`), and the input is a `TypedDict` derived from the JSON Schema declared on that entrypoint. The version that resolves at codegen time is pinned in `sufleur-lock.yaml`.

## Install

```bash
pip install sufleur-cli
sufleur --help
```

Or with [pipx](https://pipx.pypa.io/) for an isolated install:

```bash
pipx install sufleur-cli
```

The wrapper ships the prebuilt binary inside a per-platform wheel — pip selects the right one via PEP 425 platform tags. There's no Python interpreter in the invocation hot path; `sufleur` is the native binary on your `PATH`.

## Quick start

```bash
mkdir my-app && cd my-app
sufleur init                                  # creates sufleur.yaml interactively
sufleur add @my-workspace/code-review ^1.0.0  # add + fetch + lock
sufleur generate                              # writes ./generated/prompts.py
```

The generated module imports two runtime peers. Install them with the `[generated]` extra:

```bash
pip install 'sufleur-cli[generated]'
```

…or add `chevron` (Mustache templating) and `pydantic` (output-schema validation, only needed when prompts have output schemas) directly to your project's dependencies. The CLI itself has no Python runtime deps; the `[generated]` extra exists so users who run `init`/`add`/`install` but never `generate` aren't forced to install code they don't use.

The generated code targets **Python 3.10+** (PEP 604 union syntax).

## What `sufleur generate` emits

A single `.py` module containing every prompt inlined (no runtime fetches). The public API is `get_prompt(name)`, which returns a result object with:

- **`render(entrypoint, input)` → `{"prompt": str}`** — Chevron renders the entrypoint template against `input`. The signature is narrowed via `@overload` per entrypoint, so type checkers reject the wrong input shape.
- **`metadata`** — a `TypedDict` containing `version`, your workspace's custom metadata, and (when applicable) `output_schema`.
- **`parse_output(raw)`** *(only present if the prompt has an output schema)* — strips ``` fences, JSON-parses, and validates with a Pydantic model generated from the prompt's JSON Schema. Returns `{"success": True, "data": <Model>}` or `{"success": False, "error": str}`.

Plus generated `TypedDict`s per entrypoint, with field docstrings for any schema property that has a `description`:

```python
class CodeReview_EnInput(TypedDict):
    diff: str
    """The unified diff to review."""
    language: str
```

Optional schema properties are wrapped in `typing.NotRequired[...]`, and `oneOf` schemas become PEP 604 unions (`X | Y`).

Prompts published with `DRAFT` status emit a `warnings.warn(...)` when their `get_prompt` is called.

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
  language: python
  file: ./generated/prompts.py
```

Constraints are npm-style semver ranges (`^`, `~`, `>=`, exact, `*`). The resolution is recorded in `sufleur-lock.yaml`. **Commit both files** — `sufleur.yaml` is the source of truth, `sufleur-lock.yaml` is the receipt.

## CI usage

```bash
sufleur install --frozen   # fail if lockfile is stale
sufleur generate
```

`--frozen` is the `pip-compile`-equivalent: refuses to update the lockfile, hard-errors if the manifest and lockfile disagree.

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

### Render before publishing

`sufleur prompt render <dir> --entrypoint <name> [--vars '{...}' | --vars-file path.json]` runs the same Mustache pipeline as the generated runtime — useful for previewing a draft locally before publishing, or for quick experimentation against a `version dump` directory. No auth required.

## Invocation modes

The `sufleur` command on your `PATH` is the Go binary itself. For tools that prefer module-style invocation:

```bash
python -m sufleur_cli --help
```

This goes through a tiny Python wrapper that locates the binary and `os.execvp`s it (POSIX) or `subprocess.run`s it (Windows). Slightly slower because Python boots first, but useful when invoking the CLI programmatically from a Python tool that wants to be sure it's calling the binary in the active environment.

The `find_sufleur_bin()` helper is also importable:

```python
from sufleur_cli import find_sufleur_bin
print(find_sufleur_bin())  # absolute path to the binary
```

## Supported platforms

| OS      | Architectures                                  |
| ------- | ---------------------------------------------- |
| macOS   | x86_64, arm64                                  |
| Linux   | x86_64, aarch64 (manylinux 2.17 / glibc 2.17+) |
| Windows | x86_64, arm64                                  |

Alpine / musl libc is currently unsupported (no musllinux wheel) — pip will refuse with "no matching distribution" rather than silently producing a broken install. There is no source distribution.

## Links

- **Sufleur platform** — author and manage prompts: <https://sufleur.com>
- **Source code, issues, release notes**: <https://github.com/sufleur/cli>

## License

MIT.
