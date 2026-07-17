# Eval run inspection (per-case) — CLI design

**Date:** 2026-07-16
**Repo:** `cli` (public)
**Status:** approved, ready for implementation

## Problem

The CLI can trigger, watch, list, and summarise eval runs (`eval run`, `eval
watch`, `eval runs`, `eval show`) but cannot see *inside* a finished run. `eval
show` prints a bare `Detail: available` line and stops there. There is no way to
answer "which cases failed and why" from the CLI — the primary debugging
question after a run completes.

The backend already exposes everything needed; the CLI just never fetches it.

## Scope

Pure CLI surfacing of existing backend data. **No backend changes.**

The `evalRun(id)` GraphQL query already has these resolve fields:

- `assertions { id ordinal label kind definition status result errorMessage }`
  — the run's snapshotted assertions (run-level, one set for the whole run).
- `judges { id alias promptVersionId provider model }` — the run's snapshotted
  judges (run-level).
- `caseDetails { caseIndex passed resolvedInputs outputRaw outputParsed
  assertions judges renderedPrompts { fileName role renderedPrompt }
  providerError }` — per-case detail.

Per-case `assertions[]` are `{ evalRunAssertionId, passed, error?, message? }`
and reference the run-level `assertions` by id. Per-case `judges[]` are
`{ evalRunJudgeId, input, output?, score?, error?, renderedPrompts? }` and
reference the run-level `judges` by id. Rendering a case therefore requires the
run-level assertion/judge rows to resolve ids → human labels; all of it is
fetched in **one** query and joined client-side.

`caseDetails` is empty while the run is `QUEUED`/`RUNNING`, and after retention
purge (`detailPurgedAt` set). `detailAvailable` is true only when `SUCCEEDED`
and not purged.

## Commands

Two new subcommands under `eval`, mirroring the existing `runs` (list) / `show`
(single) split. Both take a `<run-id>` and support the inherited `--json` flag.

### `sufleur eval cases <run-id>`

Overview table of every case, newest data joined from the single detail fetch:

```
CASE  PASSED  ASSERTIONS  JUDGES  NOTE
0     yes     3/3         1       —
1     NO      2/3         1       —
2     NO      0/0         0       provider error
```

- `ASSERTIONS` = passed/total for that case. `JUDGES` = count of judges invoked.
- `NOTE` surfaces `providerError` presence (case counts as failed) or `—`.
- `--failed` flag: list only cases where `passed == false`. Common debug path.
- Trailing summary line: `N cases, M failed`.

### `sufleur eval case <run-id> <index>`

Single-case drill-down, results-focused by default:

```
Case 1 — FAILED

Inputs:
  <fileName>: { message: "…" }

Output:
  { "foo": … }          # outputParsed if present, else outputRaw

Assertions:
  ✓ [0] length<=200        (cel)
  ✗ [1] contains 'foo'     (cel) — <message or error, if any>

Judges:
  quality  score 0.70     # score omitted when null/absent; error shown if set

(use --prompts to see rendered prompts)
```

- `<index>` matches `caseDetails[].caseIndex`. Unknown index → friendly error
  listing the valid range.
- Assertion display line is joined from the run-level assertion row (label ||
  definition, kind, ordinal) + the per-case pass/fail/message.
- Judge display line joined from the run-level judge row (alias) + per-case
  score/output/error.
- `providerError`, when set, is shown as a prominent line in place of a normal
  Output section.
- `--prompts`: after the results, dump the candidate's `renderedPrompts` and
  each judge's `renderedPrompts` (`fileName`, `role`, `renderedPrompt` text).
  The `(use --prompts …)` hint is omitted when `--prompts` is passed or when no
  rendered prompts exist.

## Data layer (`internal/userapi/evals.go`)

Add a **separate** heavy fetch rather than fattening the existing lean
`GetEvalRun` (used by `watch`/`show` on a hot poll loop):

```go
func (c *Client) GetEvalRunDetail(ctx context.Context, runID string) (*EvalRunDetail, error)
```

`EvalRunDetail` embeds/reuses the summary `EvalRun` fields plus:

- `Assertions []EvalRunAssertion` — `{ ID, Ordinal, Label, Kind, Definition,
  Status, Result, ErrorMessage }`
- `Judges []EvalRunJudge` — `{ ID, Alias, PromptVersionID, Provider, Model }`
- `Cases []EvalRunCaseDetail` — `{ CaseIndex, Passed, ResolvedInputs (raw
  JSON), OutputRaw, OutputParsed (raw JSON, nullable), Assertions
  []EvalCaseAssertion, Judges []EvalCaseJudge, RenderedPrompts []RenderedPrompt,
  ProviderError string }`
- `EvalCaseAssertion` — `{ EvalRunAssertionID, Passed, Error, Message }`
- `EvalCaseJudge` — `{ EvalRunJudgeID, Input (raw JSON), Output (raw JSON),
  Score *float64, Error, RenderedPrompts []RenderedPrompt }`
- `RenderedPrompt` — `{ FileName, Role, RenderedPrompt }`

`resolvedInputs`, `outputParsed`, judge `input`/`output` are `GraphQLJSON` on the
wire — model them as `json.RawMessage` and pretty-print on render, never as typed
maps (avoids brittle coupling to arbitrary user schemas).

The query selects `evalRun(id: $id) { <summary fields> assertions { … } judges {
… } caseDetails { … } }`.

## Rendering (`internal/cli/eval_case_view.go`, new file)

Pure, client-agnostic helpers, unit-tested directly (matching the
`eval_diagnostics_test.go` / `eval_run_view.go` pattern):

- `writeCasesTable(w io.Writer, d *userapi.EvalRunDetail, failedOnly bool)`
- `writeCaseDetail(w io.Writer, d *userapi.EvalRunDetail, c *userapi.EvalRunCaseDetail, showPrompts bool)`
- small joiners: assertion-id → run-level assertion, judge-id → run-level judge;
  output formatter (parsed-or-raw, pretty JSON when the blob is JSON).

Command files `eval_cases.go` / `eval_case.go` are thin: parse ref/index, load
client, call `GetEvalRunDetail`, then either `printJSON` or the render helper.
Register both in `eval.go`'s `AddCommand` list.

## Detail-unavailable handling

When `GetEvalRunDetail` returns a run whose `Cases` is empty, the command prints
a friendly message keyed off status (non-`--json` mode):

- status not terminal (`QUEUED`/`RUNNING`) →
  `run <id> is still <STATUS> — per-case detail is available once it succeeds`
- terminal but empty (`FAILED`, or `SUCCEEDED` with `detailPurgedAt`) →
  `no per-case detail available for run <id> (status <STATUS>)`

These are informational (exit 0), not errors. `--json` always emits the
structured value (possibly with an empty `caseDetails`), no message.

For `eval case`, an out-of-range `<index>` when cases *do* exist is a real error
naming the valid range.

## Testing

`internal/cli/eval_case_view_test.go` (table-driven, direct render calls):

- cases table: mixed pass/fail rendering; assertion count `p/t`; judge count;
  `providerError` → `NOTE`; `--failed` filter; summary line.
- case detail: passed vs failed header; inputs pretty-print; parsed-output
  preferred over raw, raw fallback when parsed absent; assertion join shows
  label/definition + ✓/✗ + message/error; judge join shows alias + score, score
  omitted when nil, error shown when set; `providerError` replaces output;
  `--prompts` on/off (hint shown/omitted, prompt bodies present/absent).
- id-join edge: a case assertion/judge whose id is missing from the run-level
  list degrades gracefully (shows the id, no panic).

`internal/userapi/evals_test.go` (httptest GraphQL mock):

- `GetEvalRunDetail` selects `assertions`, `judges`, `caseDetails` and their
  nested fields; parses a representative payload into the structs; nullable
  `outputParsed`/`score`/`providerError` handled.

## Out of scope (YAGNI)

- No pretty diff/side-by-side of expected vs actual.
- No paging of `cases` (a run's case count is dataset-bounded and small; the
  whole detail blob is one fetch already).
- No new backend fields or resolvers.
