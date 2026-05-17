package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/credentials"
	"github.com/sufleur/cli/internal/userapi"
)

// loadUserAPIClient reads stored credentials and constructs a userapi.Client
// wired with --verbose from the persistent flag. Returns a "not logged in"
// error if no credentials file is present.
func loadUserAPIClient(cmd *cobra.Command) (*userapi.Client, *credentials.Credentials, error) {
	creds, err := credentials.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("not logged in — run `sufleur login` first")
		}
		return nil, nil, err
	}
	verbose, _ := cmd.Flags().GetBool("verbose")
	return userapi.New(creds.APIBase, creds.APIKey, verbose), creds, nil
}
