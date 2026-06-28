package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetUpdateCmd = &cobra.Command{
	Use:           "update @workspace/name --description \"...\"",
	Short:         "Update a dataset's description",
	Long:          "Replaces the description on an existing dataset. Visibility is changed from the web app, not the CLI.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("description") {
			return fmt.Errorf("--description is required")
		}
		description, _ := cmd.Flags().GetString("description")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		d, err := client.UpdateDataset(cmd.Context(), ref.Workspace, ref.Name, description)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, d)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s/%s\n", ref.Workspace, d.Name)
		return nil
	},
}

func init() {
	datasetUpdateCmd.Flags().String("description", "", "New description (pass an empty string to clear)")
}
