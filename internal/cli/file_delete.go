package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var fileDeleteCmd = &cobra.Command{
	Use:           "delete @workspace/name@version --name NAME",
	Short:         "Delete a file from a draft version",
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
		nameFlag, _ := cmd.Flags().GetString("name")
		if nameFlag == "" {
			return fmt.Errorf("--name is required")
		}
		fileName := stripMustacheSuffix(nameFlag)

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		ok, err := client.DeletePromptFile(cmd.Context(), ref.Workspace, ref.Name, ref.Version, fileName)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"deleted": ok, "fileName": fileName})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s from @%s/%s@%s\n", fileName, ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	fileDeleteCmd.Flags().String("name", "", "Registry name of the file to delete")
}
