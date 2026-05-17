package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show the user account associated with the stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		me, err := client.Me(cmd.Context())
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		if asJSON {
			return printJSON(cmd, me)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Email: %s\nID:    %s\n", me.Email, me.ID)
		return nil
	},
}

func init() {
	meCmd.Flags().Bool("json", false, "Output a single JSON object instead of human-readable text")
}
