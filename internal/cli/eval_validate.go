package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

var evalValidateCmd = &cobra.Command{
	Use:   "validate @workspace/name@version --file PATH",
	Short: "Validate an eval YAML against a version without saving it",
	Long: `Parses and type-checks an eval YAML document against a prompt version and
prints any diagnostics. Nothing is persisted.

Diagnostics come in three flavours:
  error    blocking structural problem — a push would be refused
  note     non-blocking issue (e.g. type-check / model availability) — a push
           still saves the eval, but it won't run cleanly until fixed
  warning  advisory only

Exits non-zero when there is at least one blocking error.`,
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

		yaml, err := readEvalYamlFile(cmd)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		res, err := client.ValidateEvalYaml(cmd.Context(), ref.Workspace, ref.Name, ref.Version, yaml)
		if err != nil {
			return mapBearer(err)
		}

		blocking, nonBlocking, warnings := evalDiagnosticCounts(res)

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if err := printJSON(cmd, map[string]any{
				"errors":   res.Errors,
				"warnings": res.Warnings,
				"summary":  map[string]int{"blocking": blocking, "nonBlocking": nonBlocking, "warnings": warnings},
			}); err != nil {
				return err
			}
		} else {
			out := cmd.OutOrStdout()
			if blocking+nonBlocking+warnings == 0 {
				fmt.Fprintln(out, "No issues.")
			} else {
				writeEvalDiagnostics(out, res)
			}
		}

		if blocking > 0 {
			return fmt.Errorf("eval YAML has %d blocking error(s)", blocking)
		}
		return nil
	},
}

func init() {
	evalValidateCmd.Flags().String("file", "", "Path to the eval YAML; use - to read from stdin")
}
