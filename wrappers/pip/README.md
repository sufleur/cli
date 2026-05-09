# sufleur-cli

Type-safe codegen for versioned LLM prompts. Manage prompts the way `pip` manages packages — declare them in `sufleur.yaml`, lock to `sufleur-lock.yaml`, generate one Python module with full types and runtime helpers.

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

## Source

Source code, issue tracker, release notes: <https://github.com/sufleur/cli>.

## License

MIT.
