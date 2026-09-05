package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolUpdateCmd = &cobra.Command{
	Use:   "update @workspace/name --description \"...\"",
	Short: "Update a tool's catalog description",
	Long: `Replaces the catalog blurb used in listing and search. Pass an empty string to
clear it.

This is not the text the model sees. To change that, use
"sufleur tool version set-description" on a draft version.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
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

		tool, err := client.UpdateTool(cmd.Context(), ref.Workspace, ref.Name, description)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, tool)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s/%s.\n", ref.Workspace, tool.Name)
		return nil
	},
}

func init() {
	toolUpdateCmd.Flags().String("description", "", "Catalog blurb shown in listing and search (not sent to the model)")
}
