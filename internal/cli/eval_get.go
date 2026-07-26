package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

var evalGetCmd = &cobra.Command{
	Use:   "get @workspace/name@version",
	Short: "Print a version's eval YAML (skeleton if none is configured)",
	Long: `Fetches the eval definition for a version as YAML and writes it to stdout.
The backend always returns a complete, editable skeleton when no eval exists
yet, so this is the place to start authoring from scratch.

Pass --file PATH to write the YAML to a file instead of stdout. Use --json to
get a structured {version, yaml} object.`,
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

		yaml, err := client.GetEvalYaml(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{"version": ref.Version, "yaml": yaml})
		}

		if path, _ := cmd.Flags().GetString("file"); path != "" {
			body := yaml
			if !strings.HasSuffix(body, "\n") {
				body += "\n"
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote eval YAML to %s\n", path)
			return nil
		}

		if !strings.HasSuffix(yaml, "\n") {
			yaml += "\n"
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), yaml)
		return err
	},
}

func init() {
	evalGetCmd.Flags().String("file", "", "Write the eval YAML to this path instead of stdout")
}
