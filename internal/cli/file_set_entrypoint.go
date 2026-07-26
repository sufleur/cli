package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var fileSetEntrypointCmd = &cobra.Command{
	Use:           "set-entrypoint @workspace/name@version --name NAME [--clear]",
	Short:         "Mark a file as an entrypoint (or clear it with --clear)",
	Long:          "By default sets the file as an entrypoint. Pass --clear to unset. The backend allows multiple entrypoints per version, so clearing one does not affect the others.",
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
		clear, _ := cmd.Flags().GetBool("clear")
		fileName := stripMustacheSuffix(nameFlag)
		isEntrypoint := !clear

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetPromptFileEntrypoint(cmd.Context(), ref.Workspace, ref.Name, ref.Version, fileName, isEntrypoint)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		verb := "Marked"
		if clear {
			verb = "Cleared"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s entrypoint %s on @%s/%s@%s\n", verb, fileName, ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	fileSetEntrypointCmd.Flags().String("name", "", "Registry name of the file")
	fileSetEntrypointCmd.Flags().Bool("clear", false, "Unset the entrypoint flag instead of setting it")
}
