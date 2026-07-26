package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

var evalRunCmd = &cobra.Command{
	Use:   "run @workspace/name@version",
	Short: "Trigger an eval run for a version",
	Long: `Enqueues an eval run for the version's eval and prints the new run id.

A run is a pure snapshot of the eval config (provider/model/params, dataset,
judges, assertions, threshold) as it stands now — there are no overrides; edit
and ` + "`sufleur eval push`" + ` to change what runs. The eval must have a dataset
pinned and the workspace must have the required providers configured.

Pass --watch to block and stream progress until the run finishes; the exit code
then reflects the verdict (usable as a CI gate).`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		ev, err := client.GetEval(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}
		if ev == nil {
			return fmt.Errorf("no eval configured for @%s/%s@%s — create one with `sufleur eval push`", ref.Workspace, ref.Name, ref.Version)
		}
		if ev.DatasetVersionID == "" {
			return fmt.Errorf("eval @%s/%s@%s has no dataset pinned — set `dataset.ref` in the eval YAML and `sufleur eval push` first", ref.Workspace, ref.Name, ref.Version)
		}

		run, err := client.RunEval(cmd.Context(), ref.Workspace, ev.ID)
		if err != nil {
			return mapBearer(err)
		}

		watch, _ := cmd.Flags().GetBool("watch")
		asJSON, _ := cmd.Flags().GetBool("json")
		if !watch {
			if asJSON {
				return printJSON(cmd, run)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Eval run started: %s (%s)\n", run.ID, run.Status)
			return nil
		}
		if !asJSON {
			fmt.Fprintf(cmd.OutOrStdout(), "Eval run started: %s\n", run.ID)
		}
		return watchEvalRun(cmd, client, run.ID)
	},
}

func init() {
	evalRunCmd.Flags().Bool("watch", false, "Poll the run until it finishes (exit code reflects the verdict)")
	addWatchFlags(evalRunCmd)
}
