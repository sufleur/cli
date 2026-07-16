# Eval run inspection — implementation plan

Executes spec: `docs/superpowers/specs/2026-07-16-eval-run-inspection-design.md`.
Read the spec for full rationale; this plan is the task breakdown.

Two new CLI subcommands surface per-case detail from a finished eval run:
`sufleur eval cases <run-id>` (table) and `sufleur eval case <run-id> <index>`
(drill-down). Backend already exposes the data via `evalRun(id)` resolve fields
`assertions`, `judges`, `caseDetails`; this is pure CLI surfacing.

## Global Constraints

- **Repo is PUBLIC.** No PII, no internal ticket IDs, no personal emails in
  code, comments, commit messages, or test fixtures. Use neutral fixture data.
- Go module is `github.com/sufleur/cli`. Match existing conventions in
  `internal/cli/` and `internal/userapi/` exactly (error wrapping with `%w`,
  `tabwriter` for tables, `cobra` command structs, `SilenceErrors`/
  `SilenceUsage: true`, `RunE`).
- Every `eval` subcommand inherits the persistent `--json` flag from
  `evalCmd` (registered in `internal/cli/eval.go`). Under `--json`, emit exactly
  one JSON value via the existing `printJSON(cmd, v)` helper and nothing else.
- Convert rejected-bearer errors with the existing `mapBearer(err)` helper.
- `resolvedInputs`, `outputParsed`, judge `input`/`output` are arbitrary-shaped
  JSON on the wire (`GraphQLJSON`). Model them as `json.RawMessage`; never as
  typed Go maps. Pretty-print JSON blobs on render.
- After changes: `gofmt`/`go vet` clean, `go build ./...` and `go test ./...`
  pass. Do not add comments except where they clarify non-obvious intent, per
  repo style.
- Commit at the end of each task with a descriptive message ending in the
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer.

## Reference: backend GraphQL shape (already implemented, do not change)

`evalRun(id: ID!)` returns `EvalRun` with these resolve fields relevant here:

- `assertions: [EvalRunAssertion!]!` —
  `{ id, ordinal: Int!, label: String, kind: AssertionKind!, definition: String!,
     status, result, errorMessage: String }`
  (run-level, one set per run; `kind`/`result`/`status` are string enums).
- `judges: [EvalRunJudge!]!` —
  `{ id, alias: String!, promptVersionId: ID!, provider, model: String! }`.
- `caseDetails: [EvalRunCaseDetail!]!` —
  `{ caseIndex: Int!, passed: Boolean!, resolvedInputs: JSON!, outputRaw: String!,
     outputParsed: JSON, assertions: JSON!, judges: JSON!,
     renderedPrompts: [EvalRunRenderedPrompt!]!, providerError: String }`.
  - Each element of `assertions` (JSON array) is
    `{ evalRunAssertionId: string, passed: bool, error?: string, message?: string }`.
  - Each element of `judges` (JSON array) is
    `{ evalRunJudgeId: string, input: JSON, output?: JSON, score?: number|null,
       error?: string, renderedPrompts?: [{fileName, role, renderedPrompt}] }`.
  - `EvalRunRenderedPrompt` = `{ fileName: String!, role: String!, renderedPrompt: String! }`.

`caseDetails` is `[]` while `QUEUED`/`RUNNING` and after retention purge.
The existing `evalRunFields` const already selects the summary scalar fields
(`id evalId status verdict provider model totalScore passingThreshold
processedCases errorMessage detailAvailable createdAt startedAt finishedAt`).

---

## Task 1 — Data layer: `GetEvalRunDetail` + structs

**Files:** `internal/userapi/evals.go`, `internal/userapi/evals_test.go`.

Add a heavy per-run detail fetch, kept separate from the existing lean
`GetEvalRun` (which is used on the `watch`/`show` poll loop and must stay small).

### Structs to add (exported, in `evals.go`)

```go
// EvalRunAssertion is a run-level snapshotted assertion (one set per run).
type EvalRunAssertion struct {
    ID           string `json:"id"`
    Ordinal      int    `json:"ordinal"`
    Label        string `json:"label"`
    Kind         string `json:"kind"`
    Definition   string `json:"definition"`
    Status       string `json:"status"`
    Result       string `json:"result"`
    ErrorMessage string `json:"errorMessage"`
}

// EvalRunJudge is a run-level snapshotted judge.
type EvalRunJudge struct {
    ID              string `json:"id"`
    Alias           string `json:"alias"`
    PromptVersionID string `json:"promptVersionId"`
    Provider        string `json:"provider"`
    Model           string `json:"model"`
}

// RenderedPrompt is one entrypoint exactly as sent to the provider for a case.
type RenderedPrompt struct {
    FileName       string `json:"fileName"`
    Role           string `json:"role"`
    RenderedPrompt string `json:"renderedPrompt"`
}

// EvalCaseAssertion references a run-level assertion (by EvalRunAssertionID)
// with this case's pass/fail and optional error/message.
type EvalCaseAssertion struct {
    EvalRunAssertionID string `json:"evalRunAssertionId"`
    Passed             bool   `json:"passed"`
    Error              string `json:"error"`
    Message            string `json:"message"`
}

// EvalCaseJudge references a run-level judge (by EvalRunJudgeID) with this
// case's invocation result. Input/Output are arbitrary JSON.
type EvalCaseJudge struct {
    EvalRunJudgeID  string           `json:"evalRunJudgeId"`
    Input           json.RawMessage  `json:"input"`
    Output          json.RawMessage  `json:"output"`
    Score           *float64         `json:"score"`
    Error           string           `json:"error"`
    RenderedPrompts []RenderedPrompt `json:"renderedPrompts"`
}

// EvalRunCaseDetail is per-case verbose detail. ResolvedInputs and OutputParsed
// are arbitrary JSON (raw). Assertions/Judges reference the run-level rows.
type EvalRunCaseDetail struct {
    CaseIndex       int                 `json:"caseIndex"`
    Passed          bool                `json:"passed"`
    ResolvedInputs  json.RawMessage     `json:"resolvedInputs"`
    OutputRaw       string              `json:"outputRaw"`
    OutputParsed    json.RawMessage     `json:"outputParsed"`
    Assertions      []EvalCaseAssertion `json:"assertions"`
    Judges          []EvalCaseJudge     `json:"judges"`
    RenderedPrompts []RenderedPrompt    `json:"renderedPrompts"`
    ProviderError   string              `json:"providerError"`
}

// EvalRunDetail is a run plus its run-level assertions/judges and per-case
// detail, fetched in one query for `eval cases` / `eval case`.
type EvalRunDetail struct {
    EvalRun
    Assertions []EvalRunAssertion  `json:"-"` // filled from resolve field, see below
    Judges     []EvalRunJudge      `json:"-"`
    Cases      []EvalRunCaseDetail `json:"-"`
}
```

Note on embedding + JSON: because `evalRun` returns the summary scalars AND the
three resolve fields in one object, the cleanest decode is a single anonymous
response struct that mirrors the GraphQL selection, then map it into
`EvalRunDetail`. Do NOT rely on `json:"-"` + embedding to auto-decode the
resolve fields (name it `caseDetails`, not `cases`, on the wire). Concretely:
decode into a struct whose fields carry the wire tags
(`assertions`,`judges`,`caseDetails`) and copy across. Keep `EvalRunDetail`'s
public field names as above (`Assertions`,`Judges`,`Cases`) for the render layer;
the JSON tags shown with `-` are placeholders — implement whatever decode is
correct and clean, the render layer only reads the Go fields.

### Method

```go
func (c *Client) GetEvalRunDetail(ctx context.Context, runID string) (*EvalRunDetail, error)
```

- Query: `evalRun(id: $id) { <evalRunFields> assertions { id ordinal label kind
  definition status result errorMessage } judges { id alias promptVersionId
  provider model } caseDetails { caseIndex passed resolvedInputs outputRaw
  outputParsed assertions judges renderedPrompts { fileName role renderedPrompt }
  providerError } }`.
  - Note: the case-level `assertions`/`judges` are `GraphQLJSON` scalars on the
    backend, so they are selected as bare leaves (no sub-selection) — matching
    how `caseDetails.assertions` is `GraphQLJSON`. Verify against the query the
    test asserts.
- No workspace header (runs are addressed by global UUID, authorised by bearer
  subject) — mirror the existing `GetEvalRun`.
- Return `errMissingData("evalRun")` when the run object is null; otherwise map
  into `*EvalRunDetail`.

### Test (`evals_test.go`, httptest GraphQL mock, mirror `TestClient_ValidateEvalYaml`)

- Assert the outgoing query selects `caseDetails`, `renderedPrompts`,
  `assertions`, `judges`, and the nested `renderedPrompt` fields.
- Return a representative payload: 2 cases (one passed, one failed with a
  `providerError`), 2 run-level assertions, 1 run-level judge; one case with
  `outputParsed` present, one with it null; a judge with a numeric `score` and
  one case-judge with `score: null`.
- Assert the parsed `*EvalRunDetail`: counts, `Cases[i].Passed`,
  case-assertion `EvalRunAssertionID` linkage, nullable `OutputParsed`/`Score`
  decoded correctly (nil vs set), `ProviderError` populated.

**Done when:** `go build ./...` and `go test ./internal/userapi/...` pass; new
structs + method + test present; commit.

---

## Task 2 — Commands + rendering: `eval cases` / `eval case`

**Depends on Task 1** (uses `EvalRunDetail` and `GetEvalRunDetail`).
**Files:** `internal/cli/eval_case_view.go` (new, render helpers),
`internal/cli/eval_cases.go` (new), `internal/cli/eval_case.go` (new),
`internal/cli/eval.go` (register the two commands),
`internal/cli/eval_case_view_test.go` (new).

### Render helpers (`eval_case_view.go`) — pure, no cobra/client

```go
// writeCasesTable renders the per-case overview. failedOnly drops passing cases.
func writeCasesTable(w io.Writer, d *userapi.EvalRunDetail, failedOnly bool)

// writeCaseDetail renders one case's drill-down. showPrompts appends rendered
// candidate + judge prompts.
func writeCaseDetail(w io.Writer, d *userapi.EvalRunDetail, c *userapi.EvalRunCaseDetail, showPrompts bool)
```

Plus small unexported helpers:
- `assertionByID(d, id) (*userapi.EvalRunAssertion, bool)` and
  `judgeByID(d, id) (*userapi.EvalRunJudge, bool)` — id→row joiners; on miss the
  render must degrade gracefully (show the id, no panic).
- an output formatter: prefer `OutputParsed` (pretty-printed JSON) when present
  and non-null, else `OutputRaw` verbatim.
- a JSON pretty-printer for `json.RawMessage` (compact one-line for the inputs
  summary is acceptable; multi-line indent is fine too — pick one, be
  consistent, tested).

**`writeCasesTable` format** (tabwriter; header then rows; trailing summary):
```
CASE  PASSED  ASSERTIONS  JUDGES  NOTE
0     yes     3/3         1       —
1     NO      2/3         1       —
2     NO      0/0         0       provider error

3 cases, 2 failed
```
- `PASSED`: `yes` when `Passed`, `NO` otherwise.
- `ASSERTIONS`: `<passed>/<total>` counting the case's `Assertions` where
  `Passed` is true over the total.
- `JUDGES`: `len(case.Judges)`.
- `NOTE`: `provider error` when `ProviderError != ""`, else `—`.
- `failedOnly` true → include only cases with `Passed == false`; summary line
  still reports totals over ALL cases (`N cases, M failed`).

**`writeCaseDetail` format:**
```
Case 1 — FAILED

Inputs:
  <pretty resolvedInputs>

Output:
  <parsed-or-raw output>

Assertions:
  ✓ [0] length<=200        (cel)
  ✗ [1] contains 'foo'     (cel) — not found

Judges:
  quality  score 0.70

(use --prompts to see rendered prompts)
```
- Header: `Case <index> — PASSED` / `Case <index> — FAILED` from `c.Passed`.
- When `c.ProviderError != ""`, replace the `Output:` section with
  `Provider error:\n  <message>`.
- Assertions: for each `c.Assertions`, join to the run-level assertion via
  `EvalRunAssertionID`. Display `✓`/`✗` from case `Passed`, `[<ordinal>]`, the
  label if non-empty else the definition, `(<kind lowercased>)`, and
  ` — <message or error>` when either is set. On join miss, show the raw id.
- Judges: for each `c.Judges`, join to the run-level judge via `EvalRunJudgeID`;
  show the alias, ` score <x.xx>` when `Score != nil`, and ` — error: <error>`
  when `Error != ""`. On join miss, show the raw id.
- Print the `(use --prompts …)` hint ONLY when `showPrompts` is false AND the
  case (or any of its judges) actually has rendered prompts.
- When `showPrompts` is true, after the results print a `Rendered prompts:`
  section: the candidate's `c.RenderedPrompts` then each judge's
  `RenderedPrompts`, each as `<fileName> (<role>):` followed by the text.

### `eval cases` command (`eval_cases.go`)

- `Use: "cases <run-id>"`, `Args: cobra.ExactArgs(1)`, `--failed` bool flag.
- Load client, `GetEvalRunDetail(ctx, args[0])`, `mapBearer` on error.
- `--json` → `printJSON(cmd, detail)` (emit the whole detail; the render path is
  separate). Return.
- If `len(detail.Cases) == 0`, print the detail-unavailable message (see below)
  and return nil (exit 0).
- Else `writeCasesTable(out, detail, failed)`.

### `eval case` command (`eval_case.go`)

- `Use: "case <run-id> <index>"`, `Args: cobra.ExactArgs(2)`, `--prompts` bool.
- Parse `<index>` as int; bad int → error.
- Load client, `GetEvalRunDetail`, `mapBearer`.
- `--json`: find the matching case by `CaseIndex`; emit just that case
  (`printJSON(cmd, &case)`). If not found and cases exist → error naming the
  valid indices; if no cases → detail-unavailable message is fine, but under
  `--json` prefer emitting `null`/error consistently — choose: under `--json`,
  a missing case is an error (so scripts fail loudly). Non-JSON missing case
  with cases present → error listing valid `caseIndex` values.
- If `len(detail.Cases) == 0` (non-json) → detail-unavailable message, exit 0.
- Else `writeCaseDetail(out, detail, matchedCase, prompts)`.

### Detail-unavailable message helper (shared)

```go
// evalDetailUnavailableMsg returns the informational line to print when a run
// has no per-case detail, keyed off run status. Not an error (exit 0).
func evalDetailUnavailableMsg(run *userapi.EvalRun) string
```
- `QUEUED`/`RUNNING` (non-terminal): `run <id> is still <STATUS> — per-case
  detail is available once it succeeds`.
- otherwise (`FAILED`, or `SUCCEEDED` purged): `no per-case detail available for
  run <id> (status <STATUS>)`.
- Reuse `isTerminalRunStatus` from `eval_run_view.go` to branch.

### Register (`eval.go`)

Add `evalCasesCmd, evalCaseCmd` to the `evalCmd.AddCommand(...)` list.

### Tests (`eval_case_view_test.go`, table-driven, direct render calls)

Build `*userapi.EvalRunDetail` fixtures in-test (neutral data) and assert on
rendered strings via `bytes.Buffer` — mirror `eval_diagnostics_test.go`:
- cases table: mixed pass/fail; assertion `p/t` counts; judge counts;
  `providerError` → `provider error` NOTE; `--failed` filter drops passing rows
  but summary counts all; summary line `N cases, M failed`.
- case detail: PASSED vs FAILED header; inputs rendered; parsed output preferred,
  raw fallback when parsed absent/null; assertion join shows label (and
  definition fallback when label empty) + ✓/✗ + message/error; judge join shows
  alias + score, score omitted when nil, error shown when set; `providerError`
  replaces Output; `--prompts` true prints prompt bodies and omits the hint,
  `--prompts` false with prompts present prints the hint.
- join miss: a case assertion/judge whose id is absent from the run-level list
  renders the id without panicking.
- `evalDetailUnavailableMsg`: RUNNING vs FAILED vs SUCCEEDED-purged wording.

**Done when:** `go build ./...` and `go test ./...` pass; `sufleur eval --help`
lists `cases` and `case`; commit.
