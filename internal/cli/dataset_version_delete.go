package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionDeleteCmd = &cobra.Command{
	Use:           "delete @workspace/name@version",
	Short:         "Delete a draft version",
	Long:          "Deletes a dataset version. The backend only allows deleting a draft, and only when it is the latest version.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		ok, err := client.DeleteDatasetVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"deleted": ok})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}
