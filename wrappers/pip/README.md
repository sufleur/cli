# sufleur-cli

pip wrapper for the [`sufleur`](https://github.com/WTomas/sufleur-cli) CLI — type-safe codegen for versioned LLM prompts.

The wrapper ships the prebuilt Go binary inside a per-platform wheel. `pip install` places `sufleur` directly on your `PATH` — no postinstall download, no first-run latency, no Python interpreter in the invocation hot path.

## Install

```bash
pip install sufleur-cli
sufleur --help
```

Or with [pipx](https://pipx.pypa.io/) for an isolated install:

```bash
pipx install sufleur-cli
```

If you also want the runtime dependencies that the **generated** Python code needs (see below):

```bash
pip install 'sufleur-cli[generated]'
```

## Supported platforms

| OS      | Architectures   |
| ------- | --------------- |
| macOS   | x86_64, arm64   |
| Linux   | x86_64, aarch64 (manylinux 2.17 / glibc 2.17+) |
| Windows | x86_64, arm64   |

Pip selects the right wheel automatically via PEP 425 platform tags. There is no source distribution — installing on an unsupported platform will fail with pip's "no matching distribution" error rather than silently producing a broken install.

## Generated code dependencies (`[generated]` extra)

The CLI itself has no Python runtime dependencies. However, the Python code emitted by `sufleur generate` imports [`chevron`](https://pypi.org/project/chevron/) (Mustache templating) and [`pydantic`](https://pypi.org/project/pydantic/) (model validation). Install them once with:

```bash
pip install 'sufleur-cli[generated]'
```

…or add `chevron` and `pydantic` directly to your project's dependencies. Either is fine — `[generated]` exists so users who run `sufleur init`/`install`/`update` but never `generate` aren't forced to install dependencies they don't need.

The generated code targets Python 3.10+ (PEP 604 union syntax).

## Invocation

The `sufleur` command on your `PATH` after install is the Go binary itself, not a Python shim. Cold start matches a native binary (~5 ms), and the process tree shows only the binary — no Python interpreter wrapping it.

A secondary entrypoint exists for tools that prefer module-style invocation:

```bash
python -m sufleur_cli --help
```

This goes through a tiny Python wrapper that locates the binary and `os.execvp`s it (POSIX) or `subprocess.run`s it (Windows). Slightly slower because Python boots first, but useful when invoking the CLI programmatically from a Python tool that wants to be sure it's calling the binary in the active environment.

The `find_sufleur_bin()` helper is also importable:

```python
from sufleur_cli import find_sufleur_bin
print(find_sufleur_bin())  # absolute path to the binary
```

## Source

Source code, issue tracker, and release notes: <https://github.com/WTomas/sufleur-cli>.

## License

MIT.
