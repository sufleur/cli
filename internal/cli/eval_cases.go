package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var evalCasesCmd = &cobra.Command{
	Use:           "cases <run-id>",
	Short:         "List per-case results for an eval run",
	Long:          "Fetches per-case detail for a run and prints an overview table (pass/fail, assertion and judge counts). Use --failed to show only failing cases, or --json for the full structured detail.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		detail, err := client.GetEvalRunDetail(cmd.Context(), args[0])
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, detail)
		}

		out := cmd.OutOrStdout()
		if len(detail.Cases) == 0 {
			fmt.Fprintln(out, evalDetailUnavailableMsg(&detail.EvalRun))
			return nil
		}

		failed, _ := cmd.Flags().GetBool("failed")
		writeCasesTable(out, detail, failed)
		return nil
	},
}

func init() {
	evalCasesCmd.Flags().Bool("failed", false, "Show only failing cases (summary still counts all)")
}
