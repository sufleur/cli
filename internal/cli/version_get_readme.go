package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionGetReadmeCmd = &cobra.Command{
	Use:   "get-readme @workspace/name@version",
	Short: "Print the README of a version to stdout",
	Long: `Fetches just the README markdown for a version and writes it to stdout.
Designed for agents and scripts that need the README without the full
contents of dump (files, schema, metadata).

A trailing newline is appended if the README doesn't end with one. Use
--json to get a structured {version, readme} object instead.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetPromptVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{
				"version": v.Version,
				"readme":  v.Readme,
			})
		}
		readme := v.Readme
		if !strings.HasSuffix(readme, "\n") {
			readme += "\n"
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), readme)
		return err
	},
}
