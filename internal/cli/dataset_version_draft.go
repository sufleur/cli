package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionDraftCmd = &cobra.Command{
	Use:           "draft @workspace/name",
	Short:         "Create a new draft version of a dataset",
	Long:          "Creates a new draft version, carrying forward the schema and cases of the latest published version. Fails if a draft already exists.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.CreateDatasetVersion(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created draft of @%s/%s (%d cases carried forward)\n", ref.Workspace, ref.Name, v.CaseCount)
		return nil
	},
}
