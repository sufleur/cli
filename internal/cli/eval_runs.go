package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var evalRunsCmd = &cobra.Command{
	Use:           "runs @workspace/name@version",
	Short:         "List eval runs for a version (newest first)",
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
		take, _ := cmd.Flags().GetInt("take")
		skip, _ := cmd.Flags().GetInt("skip")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		page, err := client.ListEvalRuns(cmd.Context(), ref.Workspace, ref.Name, ref.Version, take, skip)
		if err != nil {
			if errors.Is(err, userapi.ErrNoEval) {
				return fmt.Errorf("no eval configured for @%s/%s@%s — create one with `sufleur eval push`", ref.Workspace, ref.Name, ref.Version)
			}
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, page)
		}

		out := cmd.OutOrStdout()
		if len(page.Data) == 0 {
			fmt.Fprintln(out, "No runs yet.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "RUN ID\tSTATUS\tVERDICT\tSCORE\tCASES\tMODEL\tCREATED")
		for _, r := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.2f\t%d\t%s\t%s\n",
				r.ID, r.Status, evalVerdictDisplay(r.Verdict), r.TotalScore, r.ProcessedCases,
				r.Model, r.CreatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		fmt.Fprintf(out, "\nShowing %d of %d.\n", len(page.Data), page.Total)
		return nil
	},
}

func init() {
	evalRunsCmd.Flags().Int("take", 20, "Maximum number of runs to return (1-100)")
	evalRunsCmd.Flags().Int("skip", 0, "Number of runs to skip (for paging)")
}
