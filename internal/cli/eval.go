package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

// evalCmd is the parent of `sufleur eval <action>`. Subcommands manage the eval
// attached to a prompt version (@workspace/name@version) and trigger or inspect
// eval runs.
var evalCmd = &cobra.Command{
	Use:           "eval",
	Short:         "Manage and run prompt evals",
	Long:          "Subcommands manage the eval attached to a prompt version (@workspace/name@version) and trigger or inspect eval runs.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

func init() {
	evalCmd.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")
	evalCmd.AddCommand(
		evalGetCmd,
		evalValidateCmd,
		evalPushCmd,
		evalDeleteCmd,
		evalRunCmd,
		evalRunsCmd,
		evalShowCmd,
		evalWatchCmd,
	)
}

// mapBearer converts a rejected-bearer error into the standard re-login message.
func mapBearer(err error) error {
	if errors.Is(err, userapi.ErrBearerRejected) {
		return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
	}
	return err
}

// readEvalYamlFile reads an eval YAML document from --file (or stdin when --file
// is "-").
func readEvalYamlFile(cmd *cobra.Command) (string, error) {
	path, _ := cmd.Flags().GetString("file")
	if path == "" {
		return "", fmt.Errorf("--file is required (path to the eval YAML, or - for stdin)")
	}
	if path == "-" {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(raw), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(raw), nil
}
