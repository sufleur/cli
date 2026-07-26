package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionValidateCmd = &cobra.Command{
	Use:   "validate @workspace/name@version",
	Short: "Validate a version's cases against its schema",
	Long: `Runs the live validation report for a dataset version: every case is checked
against the version's current schema. Publishing (done in the web app) is
hard-gated on this passing, so run it before you publish.

Exits non-zero when there is at least one violation.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetDatasetVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}
		val := v.Validation

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if err := printJSON(cmd, val); err != nil {
				return err
			}
		} else {
			writeDatasetValidation(cmd.OutOrStdout(), val)
		}

		if val != nil && !val.Valid {
			return fmt.Errorf("%d case violation(s)", len(val.Violations))
		}
		return nil
	},
}
