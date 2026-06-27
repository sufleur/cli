package userapi

import (
	"context"
	"errors"
	"time"
)

// ErrNoEval is returned by operations that need an existing eval (listing runs,
// running) when the version has no eval configured. Callers surface a friendly
// "create one with `sufleur eval push`" message.
var ErrNoEval = errors.New("no eval configured for this version")

// EvalYamlIssue is one diagnostic emitted by parseEvalYaml. Path/Line/Column
// are optional (zero means "no position"). Blocking is a pointer because it is
// nullable on the wire and absent on warnings — only errors carry it.
type EvalYamlIssue struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Code     string `json:"code"`
	Blocking *bool  `json:"blocking"`
}

// IsBlocking reports whether the issue blocks applying. A nil Blocking (warnings,
// or an older server) is treated as non-blocking.
func (i EvalYamlIssue) IsBlocking() bool { return i.Blocking != nil && *i.Blocking }

// EvalYamlParseResult is the response of parseEvalYaml. The CLI surfaces the
// diagnostics; the resolved config (an object the server can return) is not
// requested — it duplicates the user's own YAML and isn't needed here.
type EvalYamlParseResult struct {
	Errors   []EvalYamlIssue `json:"errors"`
	Warnings []EvalYamlIssue `json:"warnings"`
}

// Eval is the queryable subset of the GraphQL Eval type the CLI needs:
// enough to resolve the eval id (to trigger a run) and to pre-check that a
// dataset is pinned. DatasetVersionId is empty when no dataset is set.
type Eval struct {
	ID               string    `json:"id"`
	Description      string    `json:"description"`
	PromptVersionID  string    `json:"promptVersionId"`
	DatasetVersionID string    `json:"datasetVersionId"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	PassingThreshold *float64  `json:"passingThreshold"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// EvalRun is the queryable subset of an eval run. Status and Verdict arrive as
// the uppercase GraphQL enum names (QUEUED|RUNNING|SUCCEEDED|FAILED,
// PASSED|FAILED). Verdict is empty until the run completes.
type EvalRun struct {
	ID               string     `json:"id"`
	EvalID           string     `json:"evalId"`
	Status           string     `json:"status"`
	Verdict          string     `json:"verdict"`
	Provider         string     `json:"provider"`
	Model            string     `json:"model"`
	TotalScore       float64    `json:"totalScore"`
	PassingThreshold *float64   `json:"passingThreshold"`
	ProcessedCases   int        `json:"processedCases"`
	ErrorMessage     string     `json:"errorMessage"`
	DetailAvailable  bool       `json:"detailAvailable"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
}

// EvalRunsPage is the response shape of Eval.runs.
type EvalRunsPage struct {
	Data  []EvalRun `json:"data"`
	Total int       `json:"total"`
}

const evalFields = "id description promptVersionId datasetVersionId provider model passingThreshold createdAt updatedAt"
const evalRunFields = "id evalId status verdict provider model totalScore passingThreshold processedCases errorMessage detailAvailable createdAt startedAt finishedAt"
const evalIssueFields = "path line column message code blocking"

// ValidateEvalYaml parses and validates an eval YAML document against a prompt
// version without persisting it. Used by `eval validate` and the pre-check in
// `eval push`.
func (c *Client) ValidateEvalYaml(ctx context.Context, workspace, name, version, yaml string) (*EvalYamlParseResult, error) {
	var resp struct {
		Result *EvalYamlParseResult `json:"parseEvalYaml"`
	}
	err := c.Do(ctx, Request{
		Query: "query ParseEvalYaml($promptName: ID!, $version: ID!, $yaml: String!) { " +
			"parseEvalYaml(promptName: $promptName, version: $version, yaml: $yaml) { " +
			"errors { " + evalIssueFields + " } warnings { " + evalIssueFields + " } } }",
		Variables: map[string]any{"promptName": name, "version": version, "yaml": yaml},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Result == nil {
		return nil, errMissingData("parseEvalYaml")
	}
	return resp.Result, nil
}

// ApplyEvalYaml declaratively upserts the eval on a version from a YAML
// document. The backend rejects structurally invalid YAML as a GraphQL error.
func (c *Client) ApplyEvalYaml(ctx context.Context, workspace, name, version, yaml string) (*Eval, error) {
	var resp struct {
		Eval *Eval `json:"applyEvalYaml"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation ApplyEvalYaml($promptName: ID!, $version: ID!, $yaml: String!) { applyEvalYaml(promptName: $promptName, version: $version, yaml: $yaml) { " + evalFields + " } }",
		Variables: map[string]any{"promptName": name, "version": version, "yaml": yaml},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Eval == nil {
		return nil, errMissingData("applyEvalYaml")
	}
	return resp.Eval, nil
}

// GetEvalYaml returns the version's eval as a YAML document. The backend always
// returns a complete skeleton (never empty) when no eval is configured.
func (c *Client) GetEvalYaml(ctx context.Context, workspace, name, version string) (string, error) {
	var resp struct {
		Yaml string `json:"evalYaml"`
	}
	err := c.Do(ctx, Request{
		Query:     "query EvalYaml($promptName: ID!, $version: ID!) { evalYaml(promptName: $promptName, version: $version) }",
		Variables: map[string]any{"promptName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	return resp.Yaml, err
}

// DeleteEval removes the eval configured on a version. The backend errors if no
// eval exists.
func (c *Client) DeleteEval(ctx context.Context, workspace, name, version string) (bool, error) {
	var resp struct {
		Deleted bool `json:"deleteEval"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation DeleteEval($promptName: ID!, $version: ID!) { deleteEval(promptName: $promptName, version: $version) }",
		Variables: map[string]any{"promptName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	return resp.Deleted, err
}

// GetEval fetches the structured eval for a version. Returns (nil, nil) when the
// version has no eval configured (the backend returns null) — callers must check
// for nil rather than treating it as an error.
func (c *Client) GetEval(ctx context.Context, workspace, name, version string) (*Eval, error) {
	var resp struct {
		Eval *Eval `json:"eval"`
	}
	err := c.Do(ctx, Request{
		Query:     "query GetEval($promptName: ID!, $version: ID!) { eval(promptName: $promptName, version: $version) { " + evalFields + " } }",
		Variables: map[string]any{"promptName": name, "version": version},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	// null eval is legitimate ("no eval configured"); do not errMissingData.
	return resp.Eval, nil
}

// RunEval enqueues a run of the eval identified by evalID. The run is a pure
// snapshot of the eval config; there are no overrides. The backend errors if the
// eval has no dataset pinned or the required providers aren't configured.
func (c *Client) RunEval(ctx context.Context, workspace, evalID string) (*EvalRun, error) {
	var resp struct {
		Run *EvalRun `json:"runEval"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation RunEval($input: RunEvalInput!) { runEval(input: $input) { " + evalRunFields + " } }",
		Variables: map[string]any{"input": map[string]any{"evalId": evalID}},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Run == nil {
		return nil, errMissingData("runEval")
	}
	return resp.Run, nil
}

// GetEvalRun fetches a single run by id. Runs are addressed by their global UUID
// and authorised by the bearer subject, so no workspace header is required.
func (c *Client) GetEvalRun(ctx context.Context, runID string) (*EvalRun, error) {
	var resp struct {
		Run *EvalRun `json:"evalRun"`
	}
	err := c.Do(ctx, Request{
		Query:     "query EvalRun($id: ID!) { evalRun(id: $id) { " + evalRunFields + " } }",
		Variables: map[string]any{"id": runID},
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Run == nil {
		return nil, errMissingData("evalRun")
	}
	return resp.Run, nil
}

// ListEvalRuns paginates the runs of a version's eval (newest first). Returns
// ErrNoEval when the version has no eval configured.
func (c *Client) ListEvalRuns(ctx context.Context, workspace, name, version string, take, skip int) (*EvalRunsPage, error) {
	var resp struct {
		Eval *struct {
			Runs *EvalRunsPage `json:"runs"`
		} `json:"eval"`
	}
	err := c.Do(ctx, Request{
		Query: "query ListEvalRuns($promptName: ID!, $version: ID!, $pagination: PaginationArgs!) { " +
			"eval(promptName: $promptName, version: $version) { runs(pagination: $pagination) { data { " + evalRunFields + " } total } } }",
		Variables: map[string]any{"promptName": name, "version": version, "pagination": map[string]int{"take": take, "skip": skip}},
		Workspace: workspace,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Eval == nil {
		return nil, ErrNoEval
	}
	if resp.Eval.Runs == nil {
		return nil, errMissingData("runs")
	}
	return resp.Eval.Runs, nil
}
