package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionDeleteCmd = &cobra.Command{
	Use:   "delete @workspace/name@draft",
	Short: "Delete a tool's draft version",
	Long: `Deletes a draft version. Published versions are immutable and cannot be
deleted; deleting a whole tool is web-app-only.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if err := client.DeleteToolVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version); err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"deleted": true, "version": ref.Version})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted @%s/%s@%s.\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}
