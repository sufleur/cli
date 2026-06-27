package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/userapi"
)

func boolPtr(b bool) *bool        { return &b }
func floatPtr(f float64) *float64 { return &f }

func TestEvalDiagnosticCounts(t *testing.T) {
	res := &userapi.EvalYamlParseResult{
		Errors: []userapi.EvalYamlIssue{
			{Message: "structural", Blocking: boolPtr(true)},
			{Message: "type-check", Blocking: boolPtr(false)},
			{Message: "no-flag"}, // nil blocking → non-blocking
		},
		Warnings: []userapi.EvalYamlIssue{
			{Message: "w1"},
			{Message: "w2"},
		},
	}
	blocking, nonBlocking, warnings := evalDiagnosticCounts(res)
	if blocking != 1 || nonBlocking != 2 || warnings != 2 {
		t.Errorf("counts = (%d, %d, %d), want (1, 2, 2)", blocking, nonBlocking, warnings)
	}
}

func TestWriteEvalDiagnostics(t *testing.T) {
	res := &userapi.EvalYamlParseResult{
		Errors: []userapi.EvalYamlIssue{
			{Path: "prompt.model", Message: "bad model", Code: "MODEL", Blocking: boolPtr(true)},
			{Path: "assertions[0]", Message: "unknown var", Blocking: boolPtr(false)},
		},
		Warnings: []userapi.EvalYamlIssue{
			{Line: 4, Column: 2, Message: "heads up"},
		},
	}
	var buf bytes.Buffer
	writeEvalDiagnostics(&buf, res)
	got := buf.String()
	for _, want := range []string{
		"error prompt.model: bad model  [MODEL]",
		"note assertions[0]: unknown var",
		"warning line 4:2: heads up",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func TestEvalRunExitError(t *testing.T) {
	cases := []struct {
		name    string
		run     userapi.EvalRun
		wantErr bool
	}{
		{"passed", userapi.EvalRun{Status: "SUCCEEDED", Verdict: "PASSED"}, false},
		{"failed verdict with threshold", userapi.EvalRun{Status: "SUCCEEDED", Verdict: "FAILED", TotalScore: 0.6, PassingThreshold: floatPtr(0.8)}, true},
		{"failed verdict no threshold", userapi.EvalRun{Status: "SUCCEEDED", Verdict: "FAILED"}, true},
		{"no verdict (no gate)", userapi.EvalRun{Status: "SUCCEEDED", Verdict: ""}, false},
		{"run errored", userapi.EvalRun{Status: "FAILED", ErrorMessage: "provider key rejected"}, true},
		{"queued", userapi.EvalRun{Status: "QUEUED"}, false},
		{"running", userapi.EvalRun{Status: "RUNNING"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := evalRunExitError(&tc.run)
			if (err != nil) != tc.wantErr {
				t.Errorf("evalRunExitError = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseWorkspaceArg(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"@acme", "acme", false},
		{"acme", "", true},
		{"@acme/welcome", "", true},
		{"@", "", true},
	}
	for _, tc := range cases {
		got, err := parseWorkspaceArg(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("parseWorkspaceArg(%q) = (%q, %v), want (%q, wantErr=%v)", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}
