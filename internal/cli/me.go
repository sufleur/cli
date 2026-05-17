package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/auth"
	"github.com/sufleur/cli/internal/credentials"
)

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show the user account associated with the stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		asJSON, _ := cmd.Flags().GetBool("json")

		creds, err := credentials.Load()
		if err != nil {
			return fmt.Errorf("not logged in — run `sufleur login` first (%w)", err)
		}

		hc := newHTTPClient(verbose)
		me, err := auth.FetchMe(cmd.Context(), hc, creds.APIBase, creds.APIKey)
		if err != nil {
			if errors.Is(err, auth.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		out := cmd.OutOrStdout()
		if asJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(me)
		}
		fmt.Fprintf(out, "Email: %s\nID:    %s\n", me.Email, me.ID)
		return nil
	},
}

func init() {
	meCmd.Flags().Bool("json", false, "Output a single JSON object instead of human-readable text")
}
