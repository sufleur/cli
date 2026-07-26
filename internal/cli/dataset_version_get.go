package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var datasetVersionGetCmd = &cobra.Command{
	Use:           "get @workspace/name@version",
	Short:         "Show a dataset version's status, schema, and validation",
	Long:          "Resolves a version by semver constraint or the literal \"draft\" and prints its status, case count, whether it has a schema, and its live validation report.",
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

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "@%s/%s@%s\n", ref.Workspace, ref.Name, v.Version)
		fmt.Fprintf(out, "status: %s\n", v.Status)
		fmt.Fprintf(out, "cases: %d (%d bytes gzipped)\n", v.CaseCount, int64(v.ByteSize))
		fmt.Fprintf(out, "schema: %s\n", schemaSummary(v.Schema))
		writeDatasetValidation(out, v.Validation)
		return nil
	},
}

// schemaSummary describes a schema map without dumping it: empty, or the count
// of top-level "properties".
func schemaSummary(schema map[string]any) string {
	if len(schema) == 0 {
		return "none"
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		return fmt.Sprintf("%d propert%s", len(props), plural(len(props), "y", "ies"))
	}
	return "set"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
