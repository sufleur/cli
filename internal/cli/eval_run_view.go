package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/sufleur/cli/internal/userapi"
)

// printRunSummary writes a key/value summary of an eval run, shared by
// `eval show` and the final frame of `eval watch` / `eval run --watch`.
func printRunSummary(out io.Writer, run *userapi.EvalRun) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Run:\t%s\n", run.ID)
	fmt.Fprintf(tw, "Eval:\t%s\n", run.EvalID)
	fmt.Fprintf(tw, "Status:\t%s\n", run.Status)
	fmt.Fprintf(tw, "Verdict:\t%s\n", evalVerdictDisplay(run.Verdict))
	fmt.Fprintf(tw, "Score:\t%s\n", evalScoreDisplay(run))
	fmt.Fprintf(tw, "Cases:\t%d\n", run.ProcessedCases)
	fmt.Fprintf(tw, "Model:\t%s / %s\n", run.Provider, run.Model)
	fmt.Fprintf(tw, "Created:\t%s\n", run.CreatedAt.Format(time.RFC3339))
	if run.StartedAt != nil {
		fmt.Fprintf(tw, "Started:\t%s\n", run.StartedAt.Format(time.RFC3339))
	}
	if run.FinishedAt != nil {
		fmt.Fprintf(tw, "Finished:\t%s\n", run.FinishedAt.Format(time.RFC3339))
	}
	if run.ErrorMessage != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", run.ErrorMessage)
	}
	if run.DetailAvailable {
		fmt.Fprintf(tw, "Detail:\tavailable\n")
	}
	_ = tw.Flush()
}

func evalVerdictDisplay(verdict string) string {
	if verdict == "" {
		return "—"
	}
	return verdict
}

func evalScoreDisplay(run *userapi.EvalRun) string {
	s := fmt.Sprintf("%.2f", run.TotalScore)
	if run.PassingThreshold != nil {
		return fmt.Sprintf("%s (threshold %.2f)", s, *run.PassingThreshold)
	}
	return s
}

// isTerminalRunStatus reports whether a run has reached a final state.
func isTerminalRunStatus(status string) bool {
	return status == "SUCCEEDED" || status == "FAILED"
}

// evalRunExitError maps a terminal run to a process exit error so that
// `eval run --watch` and `eval watch` can act as CI gates. Returns nil when the
// run passed or has no threshold to gate on.
func evalRunExitError(run *userapi.EvalRun) error {
	switch run.Status {
	case "SUCCEEDED":
		if run.Verdict == "FAILED" {
			if run.PassingThreshold != nil {
				return fmt.Errorf("eval run failed: score %.2f < threshold %.2f", run.TotalScore, *run.PassingThreshold)
			}
			return fmt.Errorf("eval run failed verdict")
		}
		return nil
	case "FAILED":
		if run.ErrorMessage != "" {
			return fmt.Errorf("eval run failed: %s", run.ErrorMessage)
		}
		return fmt.Errorf("eval run failed")
	default:
		return nil
	}
}
