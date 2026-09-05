package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionDraftCmd = &cobra.Command{
	Use:   "draft @workspace/name",
	Short: "Open a new draft version of a tool",
	Long: `Opens a new draft, carrying the latest published version's contract forward.

Rejected while a draft is already open — a tool has at most one at a time.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.CreateToolVersionDraft(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Opened a draft version of @%s/%s.\n", ref.Workspace, ref.Name)
		return nil
	},
}
