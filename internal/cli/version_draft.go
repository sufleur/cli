package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionDraftCmd = &cobra.Command{
	Use:           "draft @workspace/name",
	Short:         "Create a new draft version of a prompt",
	Long:          "Forks the latest published version into a fresh draft. The new draft is returned with its assigned version label.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.CreatePromptVersion(cmd.Context(), ref.Workspace, ref.Name)
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
		fmt.Fprintf(cmd.OutOrStdout(), "Created draft @%s/%s@%s\n", ref.Workspace, ref.Name, v.Version)
		return nil
	},
}
