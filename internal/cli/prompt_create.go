package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var promptCreateCmd = &cobra.Command{
	Use:           "create @workspace/name",
	Short:         "Create a new prompt",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		p, err := client.CreatePrompt(cmd.Context(), ref.Workspace, ref.Name, description)
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
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/%s\n", ref.Workspace, p.Name)
		return nil
	},
}

func init() {
	promptCreateCmd.Flags().String("description", "", "Optional description for the new prompt")
}
