package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetSetVisibilityCmd = &cobra.Command{
	Use:           "set-visibility @workspace/name <private|public>",
	Short:         "Change a dataset's visibility",
	Long:          "Sets a dataset to private or public. Requires the manage-visibility permission.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}
		visibility, err := normalizeVisibility(args[1])
		if err != nil {
			return err
		}
		if visibility == "" {
			return fmt.Errorf("visibility is required: private or public")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		d, err := client.UpdateDatasetVisibility(cmd.Context(), ref.Workspace, ref.Name, visibility)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, d)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "@%s/%s is now %s\n", ref.Workspace, d.Name, d.Visibility)
		return nil
	},
}
