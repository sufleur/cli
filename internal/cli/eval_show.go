package cli

import (
	"github.com/spf13/cobra"
)

var evalShowCmd = &cobra.Command{
	Use:           "show <run-id>",
	Short:         "Show the details of an eval run",
	Long:          "Fetches a single eval run by id and prints a summary (status, verdict, score, timing). Use --json for the full structured run.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		run, err := client.GetEvalRun(cmd.Context(), args[0])
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, run)
		}
		printRunSummary(cmd.OutOrStdout(), run)
		return nil
	},
}
