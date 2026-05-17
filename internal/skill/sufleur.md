---
name: sufleur
description: Use this skill when the user wants to author, edit, push, render, or otherwise manage LLM prompts stored in Sufleur (a versioned prompt registry). Triggers on `@workspace/name` references, mentions of the `sufleur` CLI, requests to draft a new version of a prompt, or any task that touches files, metadata, or the output schema of a prompt in a remote workspace.
---

# Sufleur

You help the user author, edit, and test LLM prompts stored in Sufleur — a versioned prompt registry. The `sufleur` CLI is the primary interface; every command in this skill assumes it is installed and on the user's `PATH`.

The CLI is workspace-scoped. Every resource is addressed with a fully-qualified name; there is no "active workspace" context.

## Authentication

Authentication is one-time and **must be performed by the user**. Do not attempt to handle the device-code flow yourself.

If a command fails with `not logged in — run \`sufleur login\` first`, stop and ask the user to run:

```bash
sufleur login
```

Then verify on a fresh invocation:

```bash
sufleur me
```

`sufleur me` returns the authenticated user's email and id. If this works, every other command in this skill will work too.

## Naming conventions

Always use fully-qualified references:

| Form | Meaning |
|------|---------|
| `@workspace` | the workspace itself (used by `prompt list`) |
| `@workspace/name` | a prompt |
| `@workspace/name@<version>` | a specific version, where `<version>` is a semver string like `1.2.3` or the literal `draft` |

Never invent shorthand. Never assume the user wants the workspace from the previous command — always pass it explicitly.

## Draft-first workflow

Published versions are immutable from the CLI's perspective; the backend rejects writes to them. You operate **exclusively on draft versions**.

Before any edit:

```bash
sufleur version list @workspace/name
```

If no `DRAFT` row appears, create one:

```bash
sufleur version draft @workspace/name
```

This forks the latest published version into a fresh draft and prints the new version label (e.g. `0.1.3-draft.0`). From then on, target `@workspace/name@draft` (or the specific draft label) in every write command.

## Local iteration loop: dump → edit → render → push

For any non-trivial edit, prefer the local-first loop. It avoids round-tripping to the registry on every change and keeps the agent's working state on disk where it can be inspected.

### 1. Dump the current state

```bash
sufleur version dump @workspace/name@draft --to ./working
```

Produces:

```
./working/
  files/
    welcome.mustache           # one per file on the version
    partials.mustache
  output-schema.json           # absent if the version has no schema
  metadata.yaml                # flat key→value, "{}" when empty
```

### 2. Edit files locally

Use whatever file tools you already have. The `.mustache` files are plain text; `metadata.yaml` is flat scalar key→value (types inferred from YAML scalars); `output-schema.json` is a JSON Schema object.

### 3. Render to verify

```bash
sufleur prompt render ./working --entrypoint welcome --vars '{"user":{"name":"Tom"}}'
```

Notes:

* `--entrypoint` is **required** and names the file in `./working/files/` (the `.mustache` suffix is accepted but optional).
* `--vars` is an inline JSON object; use `--vars-file ./vars.json` for larger inputs.
* `{{@outputSchema}}` is substituted with the local `output-schema.json` (pretty-printed) before rendering, matching the codegen-time behaviour.
* `{{@type ...}}` and `{{@doc ...}}` directives render to empty strings — they exist as platform metadata, not output.

### 4. Push changes back

After the local render looks right, push individual changes:

```bash
# files
sufleur file create @workspace/name@draft --file ./working/files/welcome.mustache [--name override] [--entrypoint]
sufleur file update @workspace/name@draft --name welcome --file ./working/files/welcome.mustache
sufleur file update @workspace/name@draft --name welcome --rename greeting
sufleur file delete @workspace/name@draft --name old-file
sufleur file set-entrypoint @workspace/name@draft --name welcome
sufleur file set-entrypoint @workspace/name@draft --name welcome --clear

# metadata — full-replace sync from the YAML
sufleur version set-metadata @workspace/name@draft --from-file ./working/metadata.yaml

# metadata — single-key patch (additive, leaves other keys untouched)
sufleur version set-metadata @workspace/name@draft --string model=claude-sonnet-4-5 --float temperature=0.7
sufleur version delete-metadata @workspace/name@draft --key old-key

# output schema
sufleur version set-output-schema @workspace/name@draft --file ./working/output-schema.json
```

`set-metadata`'s two modes are mutually exclusive. Use `--from-file` when the YAML is the source of truth (it deletes any key not present in the file); use the typed flags for additive patches.

## What the CLI cannot do — hand back to the human

Two operations are intentionally human-only:

* **Publishing a draft** (promoting it to a stable version).
* **Changing prompt visibility** (PUBLIC ↔ PRIVATE).

When a draft is ready for either, stop and summarise what changed. Tell the user to publish via the web UI when they're ready. Do not look for or attempt to use a `publish` or `visibility` command — they intentionally do not exist on the CLI.

## File suffix convention

The registry stores file names without the `.mustache` extension. The CLI normalises both directions:

* When writing names (`--name`, `--rename`): pass either `welcome` or `welcome.mustache` — the suffix is stripped.
* When reading names from the registry (`list`, `get`): names are bare.
* When dumping to disk: `.mustache` is appended for editor ergonomics.

This means `dump → edit → push` round-trips cleanly without any name mangling.

## Machine-readable output

Every command in the agent surface (`prompt`, `version`, `file`, `me`) supports `--json`. Prefer it whenever you need to parse the output:

```bash
sufleur version get @workspace/name@draft --json | jq '.files[].name'
sufleur prompt list @workspace --json | jq '.data[] | {name, visibility}'
```

When `--json` is set, errors are emitted on **stderr** as `{"error": "<message>"}`. A non-zero exit always means the operation failed.

## Quick reference

| Task | Command |
|------|---------|
| Check identity | `sufleur me` |
| List prompts | `sufleur prompt list @workspace` |
| Inspect prompt | `sufleur prompt get @workspace/name` |
| Create prompt | `sufleur prompt create @workspace/name --description "..."` |
| Update description | `sufleur prompt update @workspace/name --description "..."` |
| Create draft | `sufleur version draft @workspace/name` |
| List versions | `sufleur version list @workspace/name [--status DRAFT\|PUBLISHED]` |
| Inspect version | `sufleur version get @workspace/name@version` |
| Dump version | `sufleur version dump @workspace/name@version --to ./dir` |
| Delete draft | `sufleur version delete @workspace/name@draft` |
| Set metadata (sync) | `sufleur version set-metadata @workspace/name@draft --from-file ./metadata.yaml` |
| Set metadata (patch) | `sufleur version set-metadata @workspace/name@draft --string KEY=VAL` |
| Delete metadata key | `sufleur version delete-metadata @workspace/name@draft --key KEY` |
| Set output schema | `sufleur version set-output-schema @workspace/name@draft --file ./schema.json` |
| List files | `sufleur file list @workspace/name@draft` |
| Create file | `sufleur file create @workspace/name@draft --file ./welcome.mustache` |
| Update file | `sufleur file update @workspace/name@draft --name welcome --file ./welcome.mustache` |
| Rename file | `sufleur file update @workspace/name@draft --name welcome --rename greeting` |
| Delete file | `sufleur file delete @workspace/name@draft --name welcome` |
| Mark/clear entrypoint | `sufleur file set-entrypoint @workspace/name@draft --name welcome [--clear]` |
| Render locally | `sufleur prompt render ./dir --entrypoint NAME --vars '{...}'` |

Run `sufleur <command> --help` for the full flag list of any command.
