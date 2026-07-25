package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetCreateCmd = &cobra.Command{
	Use:           "create @workspace/name",
	Short:         "Create a new dataset",
	Long:          "Creates a new dataset and its initial draft version. Names follow npm conventions (lowercase, 5–214 chars; letters, digits, '-', '_', '.'). Datasets are always workspace-scoped.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		d, err := client.CreateDataset(cmd.Context(), ref.Workspace, ref.Name, description)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, d)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/%s with an initial draft\n", ref.Workspace, d.Name)
		return nil
	},
}

func init() {
	datasetCreateCmd.Flags().String("description", "", "Optional description for the new dataset")
}
