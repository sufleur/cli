package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var promptUpdateCmd = &cobra.Command{
	Use:           "update @workspace/name --description ...",
	Short:         "Update a prompt's description",
	Long:          "Replaces the description on an existing prompt. Visibility changes are intentionally not exposed; perform those in the web UI.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
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

		p, err := client.UpdatePromptDescription(cmd.Context(), ref.Workspace, ref.Name, description)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, p)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated @%s/%s\n", ref.Workspace, p.Name)
		return nil
	},
}

func init() {
	promptUpdateCmd.Flags().String("description", "", "New description (pass empty string to clear)")
}
