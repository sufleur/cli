package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var evalWatchCmd = &cobra.Command{
	Use:   "watch <run-id>",
	Short: "Follow an eval run until it finishes",
	Long: `Polls an eval run by id until it reaches a terminal state, printing
progress as the status and processed-case count change.

Exit code reflects the outcome (usable as a CI gate): 0 when the run passed
(or has no passing threshold), non-zero when the verdict failed, the run
errored, or the watch timed out.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}
		return watchEvalRun(cmd, client, args[0])
	},
}

func init() {
	addWatchFlags(evalWatchCmd)
}

// addWatchFlags registers the polling flags shared by `eval watch` and
// `eval run --watch`.
func addWatchFlags(cmd *cobra.Command) {
	cmd.Flags().Duration("interval", 2*time.Second, "Polling interval while watching")
	cmd.Flags().Duration("timeout", 10*time.Minute, "Give up watching after this long (0 = no timeout)")
}

// watchEvalRun polls a run until it reaches a terminal state, printing a line
// each time the status or processed-case count changes. It honors --interval and
// --timeout, respects context cancellation (Ctrl-C), tolerates a few transient
// poll failures, and returns the CI-gate exit error for the final run. Under
// --json it emits exactly one JSON value: the terminal run.
func watchEvalRun(cmd *cobra.Command, client *userapi.Client, runID string) error {
	interval, _ := cmd.Flags().GetDuration("interval")
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeout, _ := cmd.Flags().GetDuration("timeout")
	asJSON, _ := cmd.Flags().GetBool("json")
	out := cmd.OutOrStdout()

	ctx := cmd.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	lastStatus := ""
	lastProcessed := -1
	failures := 0
	for {
		run, err := client.GetEvalRun(ctx, runID)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return mapBearer(err)
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return watchInterruptError(ctxErr, runID)
			}
			failures++
			if failures >= 3 {
				return err
			}
			if !asJSON {
				fmt.Fprintf(out, "  (poll failed: %v; retrying)\n", err)
			}
			if waitErr := sleepCtx(ctx, interval); waitErr != nil {
				return watchInterruptError(waitErr, runID)
			}
			continue
		}
		failures = 0

		changed := run.Status != lastStatus || run.ProcessedCases != lastProcessed
		if !asJSON && changed {
			fmt.Fprintf(out, "[%s] %d cases\n", run.Status, run.ProcessedCases)
		}
		lastStatus = run.Status
		lastProcessed = run.ProcessedCases

		if isTerminalRunStatus(run.Status) {
			if asJSON {
				return printJSON(cmd, run)
			}
			printRunSummary(out, run)
			return evalRunExitError(run)
		}

		if waitErr := sleepCtx(ctx, interval); waitErr != nil {
			return watchInterruptError(waitErr, runID)
		}
	}
}

// sleepCtx waits for d or until ctx is done, returning ctx.Err() if interrupted.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func watchInterruptError(cause error, runID string) error {
	if errors.Is(cause, context.DeadlineExceeded) {
		return fmt.Errorf("timed out waiting for run %s — check later with `sufleur eval show %s`", runID, runID)
	}
	return cause
}
