package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionDeleteCmd = &cobra.Command{
	Use:           "delete @workspace/name@version",
	Short:         "Delete a draft version of a prompt",
	Long:          "Removes a draft version. Published versions cannot be deleted; the backend returns an error if you try.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
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

		ok, err := client.DeletePromptVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"deleted": ok, "ref": ref.Raw})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}
