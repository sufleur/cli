package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var workspaceListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List the workspaces you belong to",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		workspaces, err := client.ListWorkspaces(cmd.Context())
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, workspaces)
		}

		out := cmd.OutOrStdout()
		if len(workspaces) == 0 {
			fmt.Fprintln(out, "No workspaces found.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, ws := range workspaces {
			displayName := ws.DisplayName
			if displayName == "" {
				displayName = "—"
			}
			fmt.Fprintf(tw, "@%s\t%s\t%s\t%s\n", ws.Name, strings.ToLower(ws.Role), displayName, ws.Description)
		}
		_ = tw.Flush()
		return nil
	},
}
