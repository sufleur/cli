package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/sufleur/cli/internal/userapi"
)

// writeCasesTable renders the per-case overview. failedOnly drops passing
// cases from the listed rows, but the trailing summary always counts over
// ALL cases.
func writeCasesTable(w io.Writer, d *userapi.EvalRunDetail, failedOnly bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CASE\tPASSED\tASSERTIONS\tJUDGES\tNOTE")

	failed := 0
	for _, c := range d.Cases {
		if !c.Passed {
			failed++
		}
		if failedOnly && c.Passed {
			continue
		}

		passed := "yes"
		if !c.Passed {
			passed = "NO"
		}

		passedAssertions := 0
		for _, a := range c.Assertions {
			if a.Passed {
				passedAssertions++
			}
		}

		note := "—"
		if c.ProviderError != "" {
			note = "provider error"
		}

		fmt.Fprintf(tw, "%d\t%s\t%d/%d\t%d\t%s\n",
			c.CaseIndex, passed, passedAssertions, len(c.Assertions), len(c.Judges), note)
	}
	_ = tw.Flush()

	fmt.Fprintf(w, "\n%d cases, %d failed\n", len(d.Cases), failed)
}

// writeCaseDetail renders one case's drill-down. showPrompts appends
// rendered candidate + judge prompts.
func writeCaseDetail(w io.Writer, d *userapi.EvalRunDetail, c *userapi.EvalRunCaseDetail, showPrompts bool) {
	status := "PASSED"
	if !c.Passed {
		status = "FAILED"
	}
	fmt.Fprintf(w, "Case %d — %s\n\n", c.CaseIndex, status)

	fmt.Fprintln(w, "Inputs:")
	fmt.Fprintf(w, "  %s\n\n", indentJSON(c.ResolvedInputs))

	if c.ProviderError != "" {
		fmt.Fprintln(w, "Provider error:")
		fmt.Fprintf(w, "  %s\n\n", c.ProviderError)
	} else {
		fmt.Fprintln(w, "Output:")
		fmt.Fprintf(w, "  %s\n\n", formatCaseOutput(c))
	}

	fmt.Fprintln(w, "Assertions:")
	for _, ca := range c.Assertions {
		mark := "✓"
		if !ca.Passed {
			mark = "✗"
		}
		ra, ok := assertionByID(d, ca.EvalRunAssertionID)
		if !ok {
			fmt.Fprintf(w, "  %s %s\n", mark, ca.EvalRunAssertionID)
			continue
		}
		label := ra.Label
		if label == "" {
			label = ra.Definition
		}
		line := fmt.Sprintf("  %s [%d] %s (%s)", mark, ra.Ordinal, label, strings.ToLower(ra.Kind))
		if msg := firstNonEmpty(ca.Message, ca.Error); msg != "" {
			line += " — " + msg
		}
		fmt.Fprintln(w, line)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Judges:")
	for _, cj := range c.Judges {
		rj, ok := judgeByID(d, cj.EvalRunJudgeID)
		if !ok {
			fmt.Fprintf(w, "  %s\n", cj.EvalRunJudgeID)
			continue
		}
		line := "  " + rj.Alias
		if cj.Score != nil {
			line += fmt.Sprintf("  score %.2f", *cj.Score)
		}
		if cj.Error != "" {
			line += " — error: " + cj.Error
		}
		fmt.Fprintln(w, line)
	}

	hasPrompts := len(c.RenderedPrompts) > 0
	for _, cj := range c.Judges {
		if len(cj.RenderedPrompts) > 0 {
			hasPrompts = true
			break
		}
	}

	if showPrompts {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Rendered prompts:")
		for _, rp := range c.RenderedPrompts {
			fmt.Fprintf(w, "%s (%s):\n%s\n", rp.FileName, rp.Role, rp.RenderedPrompt)
		}
		for _, cj := range c.Judges {
			for _, rp := range cj.RenderedPrompts {
				fmt.Fprintf(w, "%s (%s):\n%s\n", rp.FileName, rp.Role, rp.RenderedPrompt)
			}
		}
	} else if hasPrompts {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "(use --prompts to see rendered prompts)")
	}
}

// assertionByID looks up a run-level assertion by id.
func assertionByID(d *userapi.EvalRunDetail, id string) (*userapi.EvalRunAssertion, bool) {
	for i := range d.Assertions {
		if d.Assertions[i].ID == id {
			return &d.Assertions[i], true
		}
	}
	return nil, false
}

// judgeByID looks up a run-level judge by id.
func judgeByID(d *userapi.EvalRunDetail, id string) (*userapi.EvalRunJudge, bool) {
	for i := range d.Judges {
		if d.Judges[i].ID == id {
			return &d.Judges[i], true
		}
	}
	return nil, false
}

// isRawJSONAbsent reports whether a json.RawMessage represents "no value" —
// either genuinely empty or the literal JSON null (which an explicit null
// field decodes to, not a nil slice).
func isRawJSONAbsent(raw json.RawMessage) bool {
	return len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null"
}

// formatCaseOutput prefers the pretty-printed parsed output when present and
// non-null, else falls back to the raw output verbatim.
func formatCaseOutput(c *userapi.EvalRunCaseDetail) string {
	if !isRawJSONAbsent(c.OutputParsed) {
		return indentJSON(c.OutputParsed)
	}
	return c.OutputRaw
}

// indentJSON pretty-prints a json.RawMessage with a two-space indent. Invalid
// or absent JSON is returned as-is (trimmed) rather than erroring, so
// rendering never panics on unexpected server data.
func indentJSON(raw json.RawMessage) string {
	if isRawJSONAbsent(raw) {
		return string(bytes.TrimSpace(raw))
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "  ", "  "); err != nil {
		return string(bytes.TrimSpace(raw))
	}
	return buf.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// evalDetailUnavailableMsg returns the informational line to print when a run
// has no per-case detail, keyed off run status. Not an error (exit 0).
func evalDetailUnavailableMsg(run *userapi.EvalRun) string {
	if !isTerminalRunStatus(run.Status) {
		return fmt.Sprintf("run %s is still %s — per-case detail is available once it succeeds", run.ID, run.Status)
	}
	return fmt.Sprintf("no per-case detail available for run %s (status %s)", run.ID, run.Status)
}
