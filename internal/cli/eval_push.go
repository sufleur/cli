package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

var evalPushCmd = &cobra.Command{
	Use:   "push @workspace/name@version --file PATH",
	Short: "Validate and save an eval definition against a version",
	Long: `Validates an eval YAML document, then saves it against a prompt version.

If validation produces any blocking errors the push is refused and the eval is
left unchanged. Non-blocking notes and warnings are printed but do not stop the
save (the eval is stored, but won't run cleanly until they are resolved). Run
` + "`sufleur eval validate`" + ` first to preview diagnostics without saving.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
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
		out := cmd.OutOrStdout()

		if blocking > 0 {
			if asJSON {
				_ = printJSON(cmd, map[string]any{
					"applied":  false,
					"errors":   res.Errors,
					"warnings": res.Warnings,
					"summary":  map[string]int{"blocking": blocking, "nonBlocking": nonBlocking, "warnings": warnings},
				})
			} else {
				fmt.Fprintf(out, "%d blocking error(s) — eval not applied:\n", blocking)
				writeEvalDiagnostics(out, res)
			}
			return fmt.Errorf("eval not applied: %d blocking error(s)", blocking)
		}

		ev, err := client.ApplyEvalYaml(cmd.Context(), ref.Workspace, ref.Name, ref.Version, yaml)
		if err != nil {
			return mapBearer(err)
		}

		if asJSON {
			return printJSON(cmd, map[string]any{
				"applied":  true,
				"eval":     ev,
				"errors":   res.Errors,
				"warnings": res.Warnings,
				"summary":  map[string]int{"blocking": blocking, "nonBlocking": nonBlocking, "warnings": warnings},
			})
		}

		if nonBlocking+warnings > 0 {
			writeEvalDiagnostics(out, res)
		}
		fmt.Fprintf(out, "Applied eval to @%s/%s@%s (eval %s)\n", ref.Workspace, ref.Name, ref.Version, ev.ID)
		return nil
	},
}

func init() {
	evalPushCmd.Flags().String("file", "", "Path to the eval YAML; use - to read from stdin")
}
