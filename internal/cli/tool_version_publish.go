package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionPublishCmd = &cobra.Command{
	Use:           "publish @workspace/name <version>",
	Short:         "Publish the draft version of a tool",
	Long:          "Publishes the current draft and assigns it the given semantic version.\n\n" + publishSettingNote,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
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

		v, err := client.PublishToolVersion(cmd.Context(), ref.Workspace, ref.Name, version)
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
