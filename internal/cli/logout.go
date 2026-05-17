package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/auth"
	"github.com/sufleur/cli/internal/credentials"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke the stored user API key and remove local credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		force, _ := cmd.Flags().GetBool("force")

		exists, err := credentials.Exists()
		if err != nil {
			return err
		}
		if !exists {
			fmt.Fprintln(cmd.OutOrStdout(), "Not logged in.")
			return nil
		}

		creds, err := credentials.Load()
		if err != nil {
			return err
		}

		hc := newHTTPClient(verbose)
		revoked, err := auth.RevokeUserAPIKey(cmd.Context(), hc, creds.APIBase, creds.APIKey, creds.KeyID)

		out := cmd.OutOrStdout()
		switch {
		case err == nil && revoked:
			fmt.Fprintln(out, "Revoked stored user API key.")
		case err == nil && !revoked:
			fmt.Fprintln(out, "Stored key was already revoked on the server.")
		case errors.Is(err, auth.ErrBearerRejected):
			fmt.Fprintln(out, "Stored key was no longer valid on the server.")
		default:
			// Network / unexpected error — let the user decide whether to wipe local creds.
			fmt.Fprintf(out, "Could not contact the server to revoke the key: %v\n", err)
			if !force && !confirm(cmd.InOrStdin(), out, "Delete local credentials anyway?") {
				return fmt.Errorf("aborted; local credentials kept")
			}
		}

		if err := credentials.Delete(); err != nil {
			return err
		}
		fmt.Fprintln(out, "Local credentials removed.")
		return nil
	},
}

func init() {
	logoutCmd.Flags().Bool("force", false, "Delete local credentials even if the server cannot be reached")
}

func confirm(in io.Reader, out io.Writer, question string) bool {
	fmt.Fprintf(out, "%s [y/N]: ", question)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
