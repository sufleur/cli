package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionPublishCmd = &cobra.Command{
	Use:   "publish @workspace/name@X.Y.Z",
	Short: "Publish the current draft, assigning it a semver",
	Long: `Promotes the dataset's current draft to a published version. The version
segment of the reference is the semver to assign — it must be greater than the
latest published version. Example:

  sufleur dataset version publish @acme/orders@1.0.0

Publishing is hard-gated: every case must validate against the version's final
schema. Run ` + "`dataset version validate @acme/orders@draft`" + ` first.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}
		if ref.Version == "draft" {
			return fmt.Errorf("publish needs a target semver, e.g. @%s/%s@1.0.0", ref.Workspace, ref.Name)
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.PublishDatasetVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Published @%s/%s@%s (%d cases)\n", ref.Workspace, ref.Name, v.Version, v.CaseCount)
		return nil
	},
}
