package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionPublishCmd = &cobra.Command{
	Use:           "publish @workspace/name <version>",
	Short:         "Publish the draft version of a dataset",
	Long:          "Publishes the current draft and assigns it the given semantic version. The backend rejects the publish while the draft's cases violate its schema; run `sufleur dataset version validate` first.\n\n" + publishSettingNote,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}
		version, err := parsePublishVersion(args[1])
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.PublishDatasetVersion(cmd.Context(), ref.Workspace, ref.Name, version)
		if err != nil {
			return mapPublishError(err, ref.Workspace)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Published @%s/%s@%s\n", ref.Workspace, ref.Name, v.Version)
		return nil
	},
}
