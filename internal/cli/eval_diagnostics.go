package cli

import (
	"fmt"
	"io"

	"github.com/sufleur/cli/internal/userapi"
)

// evalDiagnosticCounts tallies a parse result into blocking errors (which stop a
// push), non-blocking errors (advisory: the eval saves but won't run cleanly
// until fixed), and warnings.
func evalDiagnosticCounts(res *userapi.EvalYamlParseResult) (blocking, nonBlocking, warnings int) {
	for _, e := range res.Errors {
		if e.IsBlocking() {
			blocking++
		} else {
			nonBlocking++
		}
	}
	return blocking, nonBlocking, len(res.Warnings)
}

// writeEvalDiagnostics prints a parse result in human-readable form: blocking
// errors first, then non-blocking notes, then warnings. Nothing is printed for a
// clean result.
func writeEvalDiagnostics(out io.Writer, res *userapi.EvalYamlParseResult) {
	var blocking, notes []userapi.EvalYamlIssue
	for _, e := range res.Errors {
		if e.IsBlocking() {
			blocking = append(blocking, e)
		} else {
			notes = append(notes, e)
		}
	}
	emit := func(label string, issues []userapi.EvalYamlIssue) {
		for _, i := range issues {
			fmt.Fprintf(out, "  %s %s: %s%s\n", label, evalIssueLocation(i), i.Message, evalIssueCode(i))
		}
	}
	emit("error", blocking)
	emit("note", notes)
	emit("warning", res.Warnings)
}

func evalIssueLocation(i userapi.EvalYamlIssue) string {
	if i.Path != "" {
		return i.Path
	}
	if i.Line > 0 {
		return fmt.Sprintf("line %d:%d", i.Line, i.Column)
	}
	return "(document)"
}

func evalIssueCode(i userapi.EvalYamlIssue) string {
	if i.Code != "" {
		return "  [" + i.Code + "]"
	}
	return ""
}
