package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolCreateCmd = &cobra.Command{
	Use:   "create @workspace/name",
	Short: "Create a tool contract with an initial draft version",
	Long: `Creates a tool and opens its first draft version.

New tools are always private. Making one public is web-app-only, as it is for
prompts and datasets.

--description sets the catalog blurb used in listing and search. It is not the
text the model sees; set that on the version with
"sufleur tool version set-description".`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		tool, err := client.CreateTool(cmd.Context(), ref.Workspace, ref.Name, description)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, tool)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/%s (%s) with a draft version.\n",
			ref.Workspace, tool.Name, tool.Visibility)
		return nil
	},
}

func init() {
	toolCreateCmd.Flags().String("description", "", "Catalog blurb shown in listing and search (not sent to the model)")
}
