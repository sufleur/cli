package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

var evalDeleteCmd = &cobra.Command{
	Use:           "delete @workspace/name@version",
	Short:         "Remove the eval configured on a version",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if _, err := client.DeleteEval(cmd.Context(), ref.Workspace, ref.Name, ref.Version); err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"deleted": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted eval on @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}
