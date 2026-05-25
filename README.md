# sufleur-cli

Native Go CLI for [**Sufleur**](https://sufleur.com) — the registry where you author, version, and publish LLM prompts. It has two halves:

1. **Install published prompts into your project** the way `npm` / `pip` installs packages — declared in `sufleur.yaml`, locked to `sufleur-lock.yaml`, generated into a single typed file.
2. **Author prompts from the CLI** — full CRUD over workspace prompts, versions, files, and metadata. Designed so a coding agent (Claude Code, Cursor, etc.) can drive the authoring loop on your behalf.

## Install

Two prebuilt wrappers ship the same binary — pick the one that matches your project:

- **Node / TypeScript** → `npm i -g @sufleur/cli` · [wrapper README](wrappers/npm/README.md) · [npm](https://www.npmjs.com/package/@sufleur/cli)
- **Python** → `pip install sufleur-cli` · [wrapper README](wrappers/pip/README.md) · [PyPI](https://pypi.org/project/sufleur-cli/)

Or grab a binary directly from the GitHub releases page.

## What's in it

**Consumer side** — install and generate:

| Command | Purpose |
| ------- | ------- |
| `sufleur init` | Scaffold `sufleur.yaml` |
| `sufleur add @ws/name [range]` | Add a prompt, fetch it, update the lockfile |
| `sufleur install [--frozen]` | Resolve the manifest, fetch what's missing, refresh the lockfile |
| `sufleur update [@ws/name]` | Re-resolve one or all prompts |
| `sufleur generate` | Regenerate the typed `.ts` / `.py` file from the lockfile |

The generated file inlines every prompt (no runtime fetches) and exposes `getPrompt(name)` / `get_prompt(name)` with a typed `render(...)` plus an optional `parseOutput(...)` / `parse_output(...)` for prompts that declare an output schema.

**Authoring side** — login and CRUD:

| Group | Commands |
| ----- | -------- |
| Auth | `login`, `logout`, `me` |
| Prompts | `prompt create / get / list / update` |
| Versions | `version draft / list / get / delete / set-metadata / delete-metadata / set-output-schema / set-readme / get-readme / dump` |
| Files | `file create / update / delete / list / set-entrypoint` |
| Local render | `prompt render <dir> --entrypoint <name> --vars '{...}'` |

Every authoring command accepts `--json` for machine-readable output. See the wrapper READMEs for the full table.

## For coding agents

`sufleur skill` prints a markdown skill description — when to use the CLI, FQ-name format (`@workspace/name@version`), the full command surface, JSON flags — ready to drop into the agent of your choice. The skill ships inside the binary, so it always matches the version on your `PATH`.

```bash
# Claude Code (each skill is a directory with a SKILL.md inside)
mkdir -p ~/.claude/skills/sufleur && sufleur skill > ~/.claude/skills/sufleur/SKILL.md

# Cursor
sufleur skill > .cursor/rules/sufleur.md
```

Hand the agent the skill plus `sufleur login`, and it can drive the whole authoring loop — drafting versions, editing files, setting metadata, rendering locally — without you typing each command.

## Links

- **Platform** — author and manage prompts: <https://sufleur.com>
- **Node wrapper** — [wrappers/npm/README.md](wrappers/npm/README.md)
- **Python wrapper** — [wrappers/pip/README.md](wrappers/pip/README.md)

## License

MIT.
