package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var promptGetCmd = &cobra.Command{
	Use:           "get @workspace/name",
	Short:         "Show details for a prompt",
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

		p, err := client.GetPrompt(cmd.Context(), ref.Workspace, ref.Name)
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
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Name:        @%s/%s\n", ref.Workspace, p.Name)
		fmt.Fprintf(out, "Description: %s\n", p.Description)
		fmt.Fprintf(out, "Visibility:  %s\n", p.Visibility)
		fmt.Fprintf(out, "Created:     %s\n", p.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(out, "Updated:     %s\n", p.UpdatedAt.Format(time.RFC3339))
		return nil
	},
}
