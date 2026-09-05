---
name: sufleur
description: Use this skill when the user wants to author, edit, push, render, or otherwise manage LLM prompts stored in Sufleur (a versioned prompt registry). Triggers on `@workspace/name` references, mentions of the `sufleur` CLI, requests to draft a new version of a prompt, or any task that touches files, metadata, or the output schema of a prompt in a remote workspace.
---

# Sufleur

You help the user author, edit, and test LLM prompts stored in Sufleur — a versioned prompt registry. The `sufleur` CLI is the primary interface; every command in this skill assumes it is installed and on the user's `PATH`.

**Skill version**: this skill is current for `sufleur` CLI version `${VERSION}`. Before relying on it, confirm the user's binary matches:

```bash
sufleur --version
```

If the version reported there does not match `${VERSION}`, warn the user that this skill may be out of date — command names, flags, or output shapes may have changed. Suggest they reinstall the skill via `sufleur skill > <path>` and retry.

**More documentation**: the platform publishes LLM-focused docs at:

- <https://sufleur.com/llms.txt> — index of available pages
- <https://sufleur.com/llms-full.txt> — all pages concatenated

Fetch whichever fits your context budget when you need deeper background on the platform (data model, workspace concepts, how publishing works, schema inference, etc.).

**Invocation**: in Python projects (where the `sufleur-cli` wheel is installed in the active venv), call it directly as `sufleur`. In Node projects with `@sufleur/cli` installed as a dependency, invoke through the project's package manager — e.g. `pnpm sufleur …`, `yarn sufleur …`, `npx sufleur …`, `bunx sufleur …`. All command examples below assume the binary is reachable as `sufleur`; substitute the right prefix for your project layout.

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
| `@workspace/+name` | a **collection** (a group of prompts) — the `+` marks it as a collection; prompt names can never contain `+` |

Never invent shorthand. Never assume the user wants the workspace from the previous command — always pass it explicitly.

If you don't know which `@workspace` to address, run `sufleur workspace list` to see the workspaces the authenticated user belongs to (with their role in each). Add `--json` to consume it programmatically.

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

### Brand-new prompts start with skeleton files

`sufleur prompt create` seeds the initial draft with two **empty** files, `systemPrompt` and `userPrompt`, both marked as entrypoints. They are placeholders, not requirements — before adding your own files, either reuse them (`file update --name systemPrompt --file ...`, optionally `--rename`) or delete them (`file delete @workspace/name@draft --name systemPrompt`). If you ignore them and create fresh files instead, the empty skeletons linger as entrypoints and pollute the version.

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
  README.md                    # always present (empty if never set)
  metadata.yaml                # flat key→value, "{}" when empty
  model-config.yaml            # provider/model/parameters, absent if unset
  eval.yaml                    # the version's eval config (skeleton if none)
```

### 2. Edit files locally

Use whatever file tools you already have. The `.mustache` files are plain text; `metadata.yaml` is flat scalar key→value (types inferred from YAML scalars); `output-schema.json` is a JSON Schema object; `model-config.yaml` (when present) has `provider`/`model`/`parameters` keys, `provider` lowercase.

#### Annotate every input — part of writing the template, not a follow-up

Sufleur infers each file's **input schema** from its template, and generated client code is only as typed as the template's annotations: a bare `{{variable}}` comes out as `unknown` in generated TypeScript and `Any` in generated Python. Annotate every input **as you write it**, by default, without being asked.

Every variable gets a `{{@type ...}}` immediately after it; every section opens with one. Add `{{@doc ...}}` when the name alone doesn't tell a developer what to pass, and `{{@optional}}` when the input may be omitted (inputs default to required).

```mustache
Hello {{user.name}}{{@type string}}{{@doc Display name shown in the greeting}}!
{{#topics}}{{@type array}}{{@doc Topics to cover in the summary}}
- {{.}}{{@type string}}
{{/topics}}
{{#verbose}}{{@type boolean}}{{@optional}}Include full detail.{{/verbose}}
```

Valid types: `string`, `integer`, `number`, `boolean`, `object`, `array` — all six work on sections too. A scalar type (`string`, `integer`, `number`) on a section makes it a **presence gate over an optional scalar**: the input infers as that scalar type and is automatically optional (no `{{@optional}}` needed), the body renders only when the value is present and truthy, and variables in the body resolve in the enclosing scope. Use it to wrap optional scalars whose surrounding text should disappear when they're omitted:

```mustache
{{#nickname}}{{@type string}}Nickname: {{nickname}}{{/nickname}}
{{^nickname}}No nickname given.{{/nickname}}
```

The directives render to empty strings, so they never leak into the prompt the model sees.

Before pushing a file, re-scan it: any `{{variable}}` or `{{#section}}` without a `{{@type ...}}` is unfinished work.

### 3. Render to verify

```bash
sufleur prompt render ./working --entrypoint welcome --vars '{"user":{"name":"Tom"}}'
```

Notes:

* `--entrypoint` is **required** and names the file in `./working/files/` (the `.mustache` suffix is accepted but optional).
* `--vars` is an inline JSON object; use `--vars-file ./vars.json` for larger inputs.
* `{{@outputSchema}}` is substituted with the local `output-schema.json` (pretty-printed) before rendering, matching the codegen-time behaviour.
* `{{@type ...}}`, `{{@doc ...}}`, and `{{@optional}}` directives render to empty strings — they are platform metadata (consumed by schema inference), not output; see "Annotate every input" above for how to write them. Important: `{{@doc ...}}` is **only** carried into generated code as a JSDoc comment / Python docstring on the corresponding input field — it does **not** become part of the rendered prompt the LLM sees. If you want the model itself to read guidance about a variable, write that guidance into the prompt template directly; `{{@doc ...}}` is for downstream developer ergonomics only.

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
sufleur version set-metadata @workspace/name@draft --string owner=payments-team --float weight=0.5
sufleur version delete-metadata @workspace/name@draft --key old-key

# output schema
sufleur version set-output-schema @workspace/name@draft --file ./working/output-schema.json

# model config — provider/model/parameters (required before an eval can run)
sufleur version set-model-config @workspace/name@draft --provider anthropic --model claude-sonnet-4-5 --params '{"temperature":0.7}'
sufleur version set-model-config @workspace/name@draft --from-file ./working/model-config.yaml

# readme — replace from a file, inline string, or stdin (mutually exclusive)
sufleur version set-readme @workspace/name@draft --file ./working/README.md
sufleur version set-readme @workspace/name@draft --content "# Title\n\nBody"
echo "# Piped" | sufleur version set-readme @workspace/name@draft --file -
```

`set-metadata`'s two modes are mutually exclusive. Use `--from-file` when the YAML is the source of truth (it deletes any key not present in the file); use the typed flags for additive patches.

`set-model-config`'s `--from-file` and `--provider`/`--model`/`--params` flags are likewise mutually exclusive — `--from-file` reads the same `provider`/`model`/`parameters` shape `version dump` writes to `model-config.yaml`, so the full dump → edit → push loop covers model config too.

To learn what a prompt does without dumping the whole version, fetch just the README:

```bash
sufleur version get-readme @workspace/name@draft
```

This prints the raw markdown to stdout — cheap to pipe into context.

## Datasets

A **dataset** is a workspace-scoped, versioned collection of **cases** — one JSON object per case — plus a JSON Schema describing their shape. Datasets are what evals run against: each case becomes one trial of the candidate prompt. Address a dataset as `@workspace/name` and a version as `@workspace/name@<version>`, where `<version>` is a semver (`1.0.0`), a constraint (`^1.0`), or the literal `draft`. The `+` collection marker is never valid on a dataset reference.

Datasets follow the **same draft → publish lifecycle as prompts** — and, exactly like prompts, **publishing a version and changing visibility are human-only**: do them in the web app, not from the CLI (see "What the CLI cannot do"). The CLI handles everything up to that point — creating, drafting, uploading cases, the schema, and validation. New datasets are created **private**. Dataset names follow npm conventions: lowercase, 5–214 chars, letters/digits/`-`/`_`/`.`.

### Create and inspect

```bash
sufleur dataset list @workspace [--search ... --limit ... --offset ...]
sufleur dataset get @workspace/name                          # the dataset and its versions
sufleur dataset create @workspace/name --description "..."
sufleur dataset update @workspace/name --description "..."   # pass "" to clear the description
```

`dataset create` also creates the initial draft version, so you can start uploading cases immediately — no separate `version draft` needed for a brand-new dataset.

### Local loop: dump → edit → push → validate

```bash
sufleur dataset dump @workspace/name@draft --to ./ds
```

Produces a complete working copy:

```
./ds/
  schema.json     the JSON Schema (pretty-printed; "{}" when unset)
  cases.jsonl     one JSON object per line (empty when the version has no cases)
  dataset.yaml    name, description, visibility, version, status, caseCount (read-only metadata)
```

Edit `cases.jsonl` and `schema.json` locally, then push each back to the **draft**:

```bash
sufleur dataset cases push @workspace/name@draft --file ./ds/cases.jsonl   # .jsonl / .json (array) / .csv
sufleur dataset schema set  @workspace/name@draft --file ./ds/schema.json
```

* **Cases push** replaces the draft's cases. Format is detected from the file extension; pass `--format jsonl|json|csv` to override, which is **required** when reading from stdin (`--file -`). On the **first** upload to a fresh draft the backend **infers the schema** and may suggest enums — you usually don't set a schema by hand at all.
* **Schema set** takes a JSON Schema object; `--file -` reads from stdin. You only need it to refine the inferred schema (tighten types, add an enum, mark fields required).

Then validate — every case must conform to the schema before the version can be published:

```bash
sufleur dataset version validate @workspace/name@draft   # exits non-zero on any case violation
```

When validation is clean, the draft is ready to **publish in the web app** — publishing is human-only (the backend hard-gates it on validation passing). Once a version is published there, open the next iteration with a fresh draft:

```bash
sufleur dataset version draft @workspace/name   # carries forward the latest published schema + cases; fails if a draft already exists
```

### Read-only access

```bash
sufleur dataset version list @workspace/name [--status DRAFT|PUBLISHED]
sufleur dataset version get  @workspace/name@version       # status, schema summary, validation report
sufleur dataset schema get   @workspace/name@version [--file out.json]
sufleur dataset cases pull   @workspace/name@version [--to cases.jsonl] [--force]
```

`cases pull` downloads a version's cases as JSONL; `dataset dump` does the same plus schema and metadata in one directory — use it to snapshot any published version. `version delete @workspace/name@draft` removes a draft (published versions are immutable).

Once a version is published (in the web app), point an eval at it via `dataset.ref` (see Evals below). Use the literal `draft` in a ref to run an eval against an unpublished version while you iterate.

## Evals

An **eval** is a YAML definition attached to a specific prompt **version** (addressed by the same `@workspace/name@version` ref). It pins a dataset, the candidate prompt's input mapping, optional LLM **judges**, **assertions** over the output, and a passing threshold. The provider, model, and parameters the candidate — and each judge — runs with come from each prompt **version's** model config (set with `sufleur version set-model-config` or in the web app), not from the eval. Running it executes the candidate over every dataset case and reports a pass-rate and a verdict. There is exactly one eval per version.

The backend is the source of truth for the schema — run `sufleur eval get @workspace/name@version` to print the current definition (a complete, editable skeleton when none exists yet) rather than authoring blind. The top-level shape:

```yaml
description: extraction quality
dataset:
  ref: "@workspace/dataset@2.0.0"   # dataset version to run against; null until set
prompt:
  inputMapping:                      # provider/model/params come from this version's model config, not the eval
    files:                           # each prompt file declares its own input schema,
      - file: systemPrompt           # so each file carries its own inputs
        role: system                 # user|system
        inputs:
          taxonomy: allowed_types    # CEL over the dataset case
      - file: userPrompt
        role: user
        inputs:
          text: case.text
judges:
  - alias: quality
    prompt: "@workspace/judge@1.0.0"   # the judge's provider/model/params come from ITS version's model config
    inputMapping:
      files:
        - file: userPrompt
          role: user
          inputs: { answer: output.answer }
assertions:
  - kind: schema                     # output conforms to the version's output schema
  - kind: expression                 # a CEL boolean
    expression: "judge.quality.score > 0.7"
verdict:
  passingThreshold: 0.8              # 0–1; omit/null for no gate
```

Provider, model, and parameters now live on each prompt **version's** model config (set via `sufleur version set-model-config` or the web app), not in the eval YAML — so before an eval can run, the candidate version **and every judge version** must have a model config set. An eval can only run against providers the workspace has configured; check with `sufleur workspace providers @workspace` — add `--models` to also list each provider's available models.

### Local loop: dump → edit → validate → push

```bash
sufleur eval get @workspace/name@draft                            # print the current eval YAML
sufleur eval get @workspace/name@draft --file ./eval.yaml         # …or write it to a file
sufleur eval validate @workspace/name@draft --file ./eval.yaml    # parse + type-check; nothing saved
sufleur eval push @workspace/name@draft --file ./eval.yaml        # validate, then save
sufleur eval delete @workspace/name@draft                         # remove the eval
```

`version dump … --to ./working` also writes `./working/eval.yaml`, so a dumped directory is a complete working copy you can edit and `eval push` from.

**Diagnostics.** Both `validate` and `push` report three severities:

* **error** — a blocking structural problem (bad syntax, unresolved ref, invalid value). `push` refuses and changes nothing; `validate` exits non-zero.
* **note** — a non-blocking issue (a failed type-check, an unavailable model). `push` still saves the eval, but it won't run cleanly until the note is resolved.
* **warning** — advisory only.

Run `eval validate` and clear any blocking errors before `eval push`.

### Running and inspecting

```bash
sufleur eval run @workspace/name@draft                # enqueue a run; prints the run id
sufleur eval run @workspace/name@draft --watch        # …and stream progress to completion
sufleur eval runs @workspace/name@draft               # list recent runs (newest first)
sufleur eval show <run-id>                            # one run's status, verdict, score, timing
sufleur eval watch <run-id>                           # follow an in-flight run to completion
sufleur eval cases <run-id>                           # per-case pass/fail table (--failed for failures only)
sufleur eval case <run-id> <index>                   # one case: inputs, output, assertions, judges (--prompts)
```

A run needs (1) a pinned `dataset.ref` and (2) the candidate and judge providers configured in the workspace; `eval run` errors clearly if either is missing. A run is an immutable snapshot of the eval config — to change what runs, edit `eval.yaml` and `eval push` again.

**Inspecting results.** After a run succeeds, `eval cases <run-id>` prints a per-case pass/fail overview (assertions passed/total, judge count, provider errors); add `--failed` to list only failing cases. `eval case <run-id> <index>` drills into a single case — resolved inputs, model output (parsed when available, else raw), each assertion's ✓/✗ with its message, and each judge's score — with `--prompts` to also dump the fully rendered candidate and judge prompts. Both accept `--json`. Per-case detail exists only once a run has `SUCCEEDED` and before it is retention-purged; while queued/running or after purge the commands say so.

**Exit codes (for CI).** `eval run --watch` and `eval watch` exit `0` when the run passes (or has no passing threshold), and non-zero when the verdict fails, the run errors, or the watch times out. Without `--watch`, `eval run` returns as soon as the run is enqueued.

## Collections

A **collection** is a workspace-scoped group of prompts, referenced as `@workspace/+name`. Unlike prompts, collections have **no draft workflow** — every edit applies immediately.

**Install a collection** — bring every prompt in it into the local project (only relevant when a `sufleur.yaml` manifest is in use):

```bash
sufleur add @workspace/+name
```

This adds each member prompt to `sufleur.yaml` under its own `@workspace/prompt` key (constraint `*`) and resolves them. Prompts already in the manifest are reported and skipped; pass `--force` to reset them to `*`.

**Edit a collection** (authoring, requires login):

```bash
sufleur collection create @workspace/+name --description "..."   # new (private) collection
sufleur collection get @workspace/+name                          # metadata + README
sufleur collection list-prompts @workspace/+name                 # one @workspace/name per line
sufleur collection link @workspace/+name @workspace/prompt       # add a prompt to the collection
sufleur collection set-readme @workspace/+name --file ./README.md
sufleur collection set-description @workspace/+name --content "..."
```

To work with the prompts inside a collection, list them and then use the normal prompt/version commands on each (`version dump`, etc.) — there is no bulk "dump collection".

A prompt belongs to **at most one** collection. Linking a prompt that is already in a different collection moves it out of that one, so `collection link` refuses unless you pass `--force`.

## Tool contracts in generated code

A prompt version can pin **tool contracts**: the wire name the model emits, the description that steers when a tool gets called, and the JSON Schema of its arguments. Pins are frozen into a published version alongside its files, so they arrive with the prompt — `sufleur install` fetches them, and `sufleur generate` turns them into typed bindings.

In this phase the CLI **consumes** pins; it cannot create tools or change what a prompt pins. Do that in the web app.

What the generated file gains, for each prompt that pins something:

* a validating schema and input type per contract (zod / pydantic),
* a static output type,
* a function type your implementation must satisfy,
* `toolDefs()` — provider-neutral `{name, description, input_schema}` to hand to an SDK,
* `dispatchTool(name, rawInput, tools)` — validates the model's arguments, calls your binding, returns `{success: true, content}` or `{success: false, code}` where code is `unknown-tool`, `input-validation` or `execution`.

The trust boundary runs the **opposite way** from prompt I/O, and it matters: a tool's *arguments* are written by the model, so they are validated at runtime; a tool's *result* comes from engineer code, so it is typed statically. `dispatchTool` is a pure function — there is no loop, no SDK call and no retry in it; call it once per tool-use block.

Only `ToolExecutionError` is reported back to the model as an `execution` failure. Anything else thrown by an implementation is a bug and propagates with its stack, rather than being quietly handed to the model as a tool result.

Two things worth telling the user about when they appear:

* `install` warns `pins draft tool "x"` when a pinned tool version is still a draft. The contract can change without the prompt's version moving, so regenerate before trusting the output.
* Generated tool type names come from the tool's registry ref, not the wire name — the same contract pinned by two prompts is one type. If a prompt pins two *versions* of one tool, every version's type gets a version suffix (`…ToolV1_2_0`), so names never depend on install order.

Pins the caller cannot read are silently omitted by the registry, so a prompt could in principle generate with a tool missing. Publish-time closure rules make this all but unreachable — a published public prompt cannot pin a non-public tool — but a draft prompt pinning a cross-workspace tool that has since been made private is the residual case.

## What the CLI cannot do — hand back to the human

These operations are intentionally human-only:

* **Publishing a draft** (promoting a prompt **or dataset** draft to a stable version).
* **Changing visibility** (PUBLIC ↔ PRIVATE) of a prompt, dataset, or collection.
* **Deleting a collection**, or **removing/unlinking a prompt from a collection** (these are destructive — only `link` is exposed, never an unlink).
* **Creating or editing tool contracts**, and **changing what a prompt version pins**. The CLI reads pins so `install`/`generate` can type them; authoring them is web-app-only for now.
* **Configuring AI provider credentials** (adding or removing API keys). The CLI can *list* a workspace's providers (`workspace providers @workspace`) so you know what an eval can run against, but never add or change them.

When a draft or collection is ready for any of these, stop and summarise what changed. Tell the user to act via the web UI when they're ready. Do not look for or attempt to use a `publish`, `visibility`, `delete`-collection, `unlink`, or provider-credential command — they intentionally do not exist on the CLI.

## File suffix convention

The registry stores file names without the `.mustache` extension. The CLI normalises both directions:

* When writing names (`--name`, `--rename`): pass either `welcome` or `welcome.mustache` — the suffix is stripped.
* When reading names from the registry (`list`, `get`): names are bare.
* When dumping to disk: `.mustache` is appended for editor ergonomics.

This means `dump → edit → push` round-trips cleanly without any name mangling.

## Machine-readable output

Every command in the agent surface (`prompt`, `version`, `file`, `eval`, `dataset`, `collection`, `workspace`, `me`) supports `--json`. Prefer it whenever you need to parse the output:

```bash
sufleur version get @workspace/name@draft --json | jq '.files[].name'
sufleur prompt list @workspace --json | jq '.data[] | {name, visibility}'
```

When `--json` is set, errors are emitted on **stderr** as `{"error": "<message>"}`. A non-zero exit always means the operation failed.

## Quick reference

| Task | Command |
|------|---------|
| Check identity | `sufleur me` |
| List your workspaces | `sufleur workspace list` |
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
| Set model config | `sufleur version set-model-config @workspace/name@draft --provider anthropic --model NAME [--params '{...}']` (or `--from-file ./model-config.yaml`) |
| Read README | `sufleur version get-readme @workspace/name@version` |
| Set README | `sufleur version set-readme @workspace/name@draft [--content STR \| --file PATH]` |
| List files | `sufleur file list @workspace/name@draft` |
| Create file | `sufleur file create @workspace/name@draft --file ./welcome.mustache` |
| Update file | `sufleur file update @workspace/name@draft --name welcome --file ./welcome.mustache` |
| Rename file | `sufleur file update @workspace/name@draft --name welcome --rename greeting` |
| Delete file | `sufleur file delete @workspace/name@draft --name welcome` |
| Mark/clear entrypoint | `sufleur file set-entrypoint @workspace/name@draft --name welcome [--clear]` |
| Render locally | `sufleur prompt render ./dir --entrypoint NAME --vars '{...}'` |
| Get eval YAML | `sufleur eval get @workspace/name@version [--file PATH]` |
| Validate eval | `sufleur eval validate @workspace/name@draft --file ./eval.yaml` |
| Push eval | `sufleur eval push @workspace/name@draft --file ./eval.yaml` |
| Delete eval | `sufleur eval delete @workspace/name@draft` |
| Run eval | `sufleur eval run @workspace/name@version [--watch]` |
| List eval runs | `sufleur eval runs @workspace/name@version [--take N --skip N]` |
| Show eval run | `sufleur eval show <run-id>` |
| Watch eval run | `sufleur eval watch <run-id>` |
| List run cases | `sufleur eval cases <run-id> [--failed]` |
| Inspect one case | `sufleur eval case <run-id> <index> [--prompts]` |
| List providers | `sufleur workspace providers @workspace [--models]` |
| Install a collection | `sufleur add @workspace/+name` |
| Create collection | `sufleur collection create @workspace/+name --description "..."` |
| Inspect collection | `sufleur collection get @workspace/+name` |
| List collection prompts | `sufleur collection list-prompts @workspace/+name` |
| Link prompt to collection | `sufleur collection link @workspace/+name @workspace/prompt [--force]` |
| Set collection README | `sufleur collection set-readme @workspace/+name [--content STR \| --file PATH]` |
| Set collection description | `sufleur collection set-description @workspace/+name [--content STR \| --file PATH]` |
| List datasets | `sufleur dataset list @workspace [--search ... --limit ... --offset ...]` |
| Inspect dataset | `sufleur dataset get @workspace/name` |
| Create dataset | `sufleur dataset create @workspace/name --description "..."` |
| Update dataset description | `sufleur dataset update @workspace/name --description "..."` |
| Dump dataset version | `sufleur dataset dump @workspace/name@version --to ./dir [--force]` |
| New dataset draft | `sufleur dataset version draft @workspace/name` |
| List dataset versions | `sufleur dataset version list @workspace/name [--status DRAFT\|PUBLISHED]` |
| Inspect dataset version | `sufleur dataset version get @workspace/name@version` |
| Validate dataset cases | `sufleur dataset version validate @workspace/name@draft` |
| Delete dataset draft | `sufleur dataset version delete @workspace/name@draft` |
| Get dataset schema | `sufleur dataset schema get @workspace/name@version [--file PATH]` |
| Set dataset schema | `sufleur dataset schema set @workspace/name@draft --file schema.json` |
| Push dataset cases | `sufleur dataset cases push @workspace/name@draft --file cases.jsonl [--format jsonl\|json\|csv]` |
| Pull dataset cases | `sufleur dataset cases pull @workspace/name@version [--to cases.jsonl] [--force]` |

Run `sufleur <command> --help` for the full flag list of any command.
