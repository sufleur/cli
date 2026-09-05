# sufleur-cli

Native Go CLI for [**Sufleur**](https://sufleur.com) — the registry where you author, version, and publish LLM prompts. It has two halves:

1. **Install published prompts into your project** the way `npm` / `pip` installs packages — declared in `sufleur.yaml`, locked to `sufleur-lock.yaml`, generated into a single typed file.
2. **Author prompts from the CLI** — full CRUD over workspace prompts, versions, files, and metadata. Designed so a coding agent (Claude Code, Cursor, etc.) can drive the authoring loop on your behalf.

## The payoff

Most codebases keep prompts as raw strings — hand-rolled interpolation, no versioning, and `JSON.parse` + finger-crossing on the model's output:

```ts
// before
const prompt = `You are a support triage engine. Classify this ticket:\n${text}`;
const llmResponse = await callLLM(prompt);
const triage = JSON.parse(llmResponse); // untyped, unvalidated 🤞
```

Sufleur turns a published prompt into a typed function in your own repo:

```ts
// after — everything below is generated code, autocomplete included
import { getPrompt } from './generated/prompts';

const triage = getPrompt('@sufleur/ticket-triage');

const user = triage.render('userPrompt', {
  text: 'Checkout has been down for an hour and we are losing orders!',
}); // input shape is typed from the template's variables

// …send user.prompt to any LLM SDK, then validate the response:
const result = triage.parseOutput(llmText); // zod-validated against the prompt's output schema
if (result.success) {
  result.data.priority; // 'urgent' | 'high' | 'medium' | 'low'
  result.data.category; // 'bug' | 'billing' | … | 'outage' | 'other'
}
```

The generated file inlines everything — no vendor SDK, no runtime fetches. Its only runtime deps are `mustache` and `zod` (or `chevron` and `pydantic` for Python).

### Tools

A prompt version can pin **tool contracts** — the wire name, the description that steers when the model reaches for a tool, and the JSON Schema of its arguments — and those come down with the prompt:

```ts
const brief = getPrompt('@acme/daily-brief');

// Provider-neutral { name, description, input_schema } for every pinned tool.
const res = await anthropic.messages.create({ ..., tools: brief.toolDefs() });

for (const block of res.content) {
  if (block.type !== 'tool_use') continue;
  // Validates what the model sent, then calls your implementation.
  const out = await brief.dispatchTool(block.name, block.input, { webSearch: mySearchFn });
  // -> { success: true, content } | { success: false, code: 'input-validation' | … }
}
```

The trust boundary runs the opposite way from prompt I/O: a tool's **arguments** are written by the model, so they are validated at runtime, while a tool's **result** comes from your own code, so it is typed statically. The bindings object is typed from the pins, so forgetting a tool — or changing one's shape — is a compile error rather than a runtime surprise. You still own the conversation loop; `dispatchTool` is a pure function.

## Quickstart

Zero to that typed call in under five minutes. Public prompts need no account or API key:

```bash
npm i -g @sufleur/cli                # or: pip install sufleur-cli

sufleur init                         # scaffold sufleur.yaml — accept the defaults
sufleur add @sufleur/ticket-triage   # resolve, fetch, and lock the prompt
sufleur generate                     # emit ./generated/prompts.ts (or .py)
npm i mustache zod                   # runtime deps of the generated file
                                     # (Python: pip install chevron pydantic)
```

`sufleur.yaml` declares what you depend on, `sufleur-lock.yaml` pins what you got, and the generated file is the only thing your code imports:

```yaml
# sufleur.yaml
prompts:
  "@sufleur/ticket-triage": "*"
output:
  language: typescript
  file: ./generated/prompts.ts
```

That's it — the `getPrompt(...)` call above now works, with autocomplete on prompt names, entrypoints, and inputs. Browse more public prompts at <https://sufleur.com/explore>.

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
| `sufleur add @ws/+collection` | Add **every** prompt in a collection (each under its own `@ws/name` key), then install |
| `sufleur install [--frozen]` | Resolve the manifest, fetch what's missing, refresh the lockfile |
| `sufleur update [@ws/name]` | Re-resolve one or all prompts |
| `sufleur generate` | Regenerate the typed `.ts` / `.py` file from the lockfile |

The generated file inlines every prompt (no runtime fetches) and exposes `getPrompt(name)` / `get_prompt(name)` with a typed `render(...)` plus an optional `parseOutput(...)` / `parse_output(...)` for prompts that declare an output schema.

**Authoring side** — login and CRUD:

| Group | Commands |
| ----- | -------- |
| Auth | `login`, `logout`, `me` |
| Workspaces | `workspace list` |
| Prompts | `prompt create / get / list / update` |
| Versions | `version draft / list / get / delete / set-metadata / delete-metadata / set-output-schema / set-model-config / set-readme / get-readme / dump` |
| Files | `file create / update / delete / list / set-entrypoint` |
| Evals | `eval get / validate / push / delete / run / runs / show / watch / cases / case` |
| Datasets | `dataset create / get / list / update / dump`, plus `dataset version / schema / cases` subgroups |
| Collections | `collection create / get / list-prompts / link / set-readme / set-description` |
| Local render | `prompt render <dir> --entrypoint <name> --vars '{...}'` |

Every authoring command accepts `--json` for machine-readable output. See the wrapper READMEs for the full table.

### Collections

A **collection** is a workspace-scoped group of prompts, referenced as `@workspace/+name` — the `+` marks it as a collection (prompt names can never contain `+`). Collections have no draft→publish workflow; every edit is applied immediately.

- `sufleur add @ws/+name` — install the collection: add each member prompt to `sufleur.yaml` under its own `@ws/prompt` key (constraint `*`) and resolve. Prompts already present are reported and skipped (use `--force` to reset them to `*`).
- `sufleur collection create @ws/+name [--description ...]` — create a new (private) collection.
- `sufleur collection list-prompts @ws/+name` — print the member prompts, one `@ws/name` per line (pipe into `version dump` / `add`).
- `sufleur collection link @ws/+name @ws/prompt [--force]` — add a prompt to a collection. A prompt belongs to at most one collection, so moving one already in another collection requires `--force`.
- `sufleur collection set-readme @ws/+name [--content ... | --file ...]` / `set-description ...` — document the collection.
- `sufleur collection get @ws/+name` — show metadata + README.

### Evals

An **eval** scores a prompt version against a dataset — it pins the dataset, the candidate's input mapping, optional LLM judges, [CEL](https://github.com/google/cel-spec) assertions over the output, and a passing threshold. (The provider, model, and parameters the candidate and judges run with come from each prompt version's model config — set with `sufleur version set-model-config` or in the web app.) Evals are authored as YAML on a draft version; the loop mirrors prompt editing:

- `sufleur eval get @ws/name@draft --file ./eval.yaml` — dump the current eval YAML (a ready-to-edit skeleton if none exists).
- `sufleur eval validate @ws/name@draft --file ./eval.yaml` — parse and type-check; saves nothing.
- `sufleur eval push @ws/name@draft --file ./eval.yaml` — validate, then save.
- `sufleur eval run @ws/name@draft [--watch]` — enqueue a run; with `--watch` it streams to completion and exits non-zero on a failing verdict, so it works as a CI gate.
- `sufleur eval runs / show <id> / watch <id>` — list, summarise, and follow runs.
- `sufleur eval cases <id> [--failed]` / `eval case <id> <index> [--prompts]` — drill into a succeeded run's per-case results: the pass/fail table, then a single case's inputs, output, assertions, and judges.

An eval pins a **dataset** version through `dataset.ref` — and those datasets are authored from the CLI too (see below). Full guide: <https://sufleur.com/docs/evals>.

### Datasets

A **dataset** is a workspace-scoped, versioned collection of **cases** (one JSON object per case) plus a JSON Schema describing their shape — the data an eval runs against. Datasets use the same draft→publish lifecycle as prompts, and — like prompts — **publishing a version and changing visibility are web-app only**. The CLI covers everything up to publish: create, draft, cases, schema, and validation.

- `sufleur dataset create @ws/name [--description ...]` — create a dataset and its initial draft (private).
- `sufleur dataset dump @ws/name@draft --to ./ds` — write `schema.json`, `cases.jsonl`, and `dataset.yaml` to a directory for local editing.
- `sufleur dataset cases push @ws/name@draft --file ./ds/cases.jsonl` — upload cases (JSONL, JSON array, or CSV; format detected from the extension). The schema is **inferred on the first upload**.
- `sufleur dataset schema set @ws/name@draft --file ./ds/schema.json` — refine the inferred JSON Schema.
- `sufleur dataset version validate @ws/name@draft` — check every case against the schema (exits non-zero on a violation). Publishing in the app is gated on this passing.
- `sufleur dataset version draft @ws/name` — open the next draft once a version has been published in the app.
- `sufleur dataset cases pull @ws/name@version --to cases.jsonl` — download a version's cases.

Reference a published version from an eval with `dataset.ref: "@ws/name@1.0.0"` (raw semver, no `v`; or the literal `draft` while iterating). Full guide: <https://sufleur.com/docs/datasets>.

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
