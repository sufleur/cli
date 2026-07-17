package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/userapi"
)

func rawJSON(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

func TestWriteCasesTable(t *testing.T) {
	d := &userapi.EvalRunDetail{
		Cases: []userapi.EvalRunCaseDetail{
			{
				CaseIndex:  0,
				Passed:     true,
				Assertions: []userapi.EvalCaseAssertion{{Passed: true}, {Passed: true}, {Passed: true}},
				Judges:     []userapi.EvalCaseJudge{{}},
			},
			{
				CaseIndex:  1,
				Passed:     false,
				Assertions: []userapi.EvalCaseAssertion{{Passed: true}, {Passed: true}, {Passed: false}},
				Judges:     []userapi.EvalCaseJudge{{}},
			},
			{
				CaseIndex:     2,
				Passed:        false,
				ProviderError: "rate limited",
			},
		},
	}

	t.Run("all cases", func(t *testing.T) {
		var buf bytes.Buffer
		writeCasesTable(&buf, d, false)
		got := buf.String()
		for _, want := range []string{
			"CASE",
			"PASSED",
			"ASSERTIONS",
			"JUDGES",
			"NOTE",
			"0",
			"yes",
			"3/3",
			"NO",
			"2/3",
			"0/0",
			"provider error",
			"3 cases, 2 failed",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
		// Case 0 line must not carry the "provider error" note.
		lines := strings.Split(got, "\n")
		for _, l := range lines {
			if strings.HasPrefix(l, "0 ") || strings.HasPrefix(l, "0\t") {
				if strings.Contains(l, "provider error") {
					t.Errorf("case 0 row unexpectedly flagged provider error: %q", l)
				}
			}
		}
	})

	t.Run("failed only drops passing rows but summary counts all", func(t *testing.T) {
		var buf bytes.Buffer
		writeCasesTable(&buf, d, true)
		got := buf.String()
		if strings.Contains(got, "yes") {
			t.Errorf("expected passing case row to be dropped, got:\n%s", got)
		}
		if !strings.Contains(got, "3 cases, 2 failed") {
			t.Errorf("expected summary to count all cases, got:\n%s", got)
		}
	})
}

func TestWriteCaseDetail(t *testing.T) {
	d := &userapi.EvalRunDetail{
		Assertions: []userapi.EvalRunAssertion{
			{ID: "a1", Ordinal: 0, Label: "length check", Kind: "CEL", Definition: "length<=200"},
			{ID: "a2", Ordinal: 1, Label: "", Kind: "CEL", Definition: "contains 'foo'"},
		},
		Judges: []userapi.EvalRunJudge{
			{ID: "j1", Alias: "quality"},
		},
	}

	t.Run("passed case, parsed output preferred", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      1,
			Passed:         true,
			ResolvedInputs: rawJSON(`{"topic":"widgets"}`),
			OutputRaw:      "raw fallback text",
			OutputParsed:   rawJSON(`{"summary":"ok"}`),
			Assertions: []userapi.EvalCaseAssertion{
				{EvalRunAssertionID: "a1", Passed: true},
				{EvalRunAssertionID: "a2", Passed: false, Message: "not found"},
			},
			Judges: []userapi.EvalCaseJudge{
				{EvalRunJudgeID: "j1", Score: floatPtr(0.7)},
			},
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		for _, want := range []string{
			"Case 1 — PASSED",
			"Inputs:",
			`"topic": "widgets"`,
			"Output:",
			`"summary": "ok"`,
			"✓ [0] length check (cel)",
			"✗ [1] contains 'foo' (cel) — not found",
			"quality  score 0.70",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "raw fallback text") {
			t.Errorf("expected raw output NOT present when parsed output is available:\n%s", got)
		}
	})

	t.Run("failed case, raw output fallback when parsed absent", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      2,
			Passed:         false,
			ResolvedInputs: rawJSON(`{"topic":"gadgets"}`),
			OutputRaw:      "not valid json",
			OutputParsed:   nil,
			Assertions: []userapi.EvalCaseAssertion{
				{EvalRunAssertionID: "a1", Passed: false, Error: "boom"},
			},
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if !strings.Contains(got, "Case 2 — FAILED") {
			t.Errorf("expected FAILED header, got:\n%s", got)
		}
		if !strings.Contains(got, "not valid json") {
			t.Errorf("expected raw output fallback, got:\n%s", got)
		}
		if !strings.Contains(got, "✗ [0] length check (cel) — boom") {
			t.Errorf("expected error message on assertion line, got:\n%s", got)
		}
	})

	t.Run("null parsed output falls back to raw", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      3,
			Passed:         true,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "verbatim",
			OutputParsed:   rawJSON(`null`),
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if !strings.Contains(got, "verbatim") {
			t.Errorf("expected raw output when parsed is JSON null, got:\n%s", got)
		}
	})

	t.Run("provider error replaces output section", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      4,
			Passed:         false,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "should not appear",
			ProviderError:  "connection reset",
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if !strings.Contains(got, "Provider error:") || !strings.Contains(got, "connection reset") {
			t.Errorf("expected provider error section, got:\n%s", got)
		}
		if strings.Contains(got, "Output:") {
			t.Errorf("expected Output: section to be replaced, got:\n%s", got)
		}
		if strings.Contains(got, "should not appear") {
			t.Errorf("expected raw output not to be printed alongside provider error, got:\n%s", got)
		}
	})

	t.Run("judge score omitted when nil, error shown when set", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      5,
			Passed:         false,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "x",
			Judges: []userapi.EvalCaseJudge{
				{EvalRunJudgeID: "j1", Score: nil, Error: "judge timed out"},
			},
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if strings.Contains(got, "score") {
			t.Errorf("expected no score text when Score is nil, got:\n%s", got)
		}
		if !strings.Contains(got, "quality — error: judge timed out") {
			t.Errorf("expected judge error line, got:\n%s", got)
		}
	})

	t.Run("prompts hint shown when prompts present and showPrompts false", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      6,
			Passed:         true,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "x",
			RenderedPrompts: []userapi.RenderedPrompt{
				{FileName: "system.md", Role: "system", RenderedPrompt: "You are a helper."},
			},
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if !strings.Contains(got, "(use --prompts to see rendered prompts)") {
			t.Errorf("expected prompts hint, got:\n%s", got)
		}
		if strings.Contains(got, "You are a helper.") {
			t.Errorf("expected prompt body not printed when showPrompts is false, got:\n%s", got)
		}
	})

	t.Run("no hint and no rendered-prompts section when no prompts exist", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      7,
			Passed:         true,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "x",
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, false)
		got := buf.String()
		if strings.Contains(got, "--prompts") {
			t.Errorf("expected no prompts hint when no prompts exist, got:\n%s", got)
		}
	})

	t.Run("showPrompts true prints candidate and judge prompt bodies, omits hint", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      8,
			Passed:         true,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "x",
			RenderedPrompts: []userapi.RenderedPrompt{
				{FileName: "system.md", Role: "system", RenderedPrompt: "You are a helper."},
			},
			Judges: []userapi.EvalCaseJudge{
				{
					EvalRunJudgeID: "j1",
					RenderedPrompts: []userapi.RenderedPrompt{
						{FileName: "judge.md", Role: "user", RenderedPrompt: "Score this output."},
					},
				},
			},
		}
		var buf bytes.Buffer
		writeCaseDetail(&buf, d, c, true)
		got := buf.String()
		for _, want := range []string{
			"Rendered prompts:",
			"system.md (system):",
			"You are a helper.",
			"judge.md (user):",
			"Score this output.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "(use --prompts") {
			t.Errorf("expected hint to be omitted when showPrompts is true, got:\n%s", got)
		}
	})

	t.Run("join miss renders raw id without panicking", func(t *testing.T) {
		c := &userapi.EvalRunCaseDetail{
			CaseIndex:      9,
			Passed:         false,
			ResolvedInputs: rawJSON(`{}`),
			OutputRaw:      "x",
			Assertions: []userapi.EvalCaseAssertion{
				{EvalRunAssertionID: "does-not-exist", Passed: false},
			},
			Judges: []userapi.EvalCaseJudge{
				{EvalRunJudgeID: "also-missing"},
			},
		}
		var buf bytes.Buffer
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("writeCaseDetail panicked on join miss: %v", r)
				}
			}()
			writeCaseDetail(&buf, d, c, false)
		}()
		got := buf.String()
		if !strings.Contains(got, "does-not-exist") {
			t.Errorf("expected raw assertion id on join miss, got:\n%s", got)
		}
		if !strings.Contains(got, "also-missing") {
			t.Errorf("expected raw judge id on join miss, got:\n%s", got)
		}
	})
}

func TestEvalDetailUnavailableMsg(t *testing.T) {
	cases := []struct {
		name string
		run  userapi.EvalRun
		want string
	}{
		{
			"running",
			userapi.EvalRun{ID: "run-1", Status: "RUNNING"},
			"run run-1 is still RUNNING — per-case detail is available once it succeeds",
		},
		{
			"queued",
			userapi.EvalRun{ID: "run-2", Status: "QUEUED"},
			"run run-2 is still QUEUED — per-case detail is available once it succeeds",
		},
		{
			"failed",
			userapi.EvalRun{ID: "run-3", Status: "FAILED"},
			"no per-case detail available for run run-3 (status FAILED)",
		},
		{
			"succeeded purged",
			userapi.EvalRun{ID: "run-4", Status: "SUCCEEDED"},
			"no per-case detail available for run run-4 (status SUCCEEDED)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalDetailUnavailableMsg(&tc.run)
			if got != tc.want {
				t.Errorf("evalDetailUnavailableMsg() = %q, want %q", got, tc.want)
			}
		})
	}
}
