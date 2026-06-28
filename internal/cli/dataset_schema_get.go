package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var datasetSchemaGetCmd = &cobra.Command{
	Use:           "get @workspace/name@version",
	Short:         "Print a dataset version's JSON Schema",
	Long:          "Writes the version's JSON Schema (pretty-printed) to stdout, or to --file when given. Prints `{}` when no schema is set.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("file")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetDatasetVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		schema := v.Schema
		if schema == nil {
			schema = map[string]any{}
		}
		raw, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding schema: %w", err)
		}
		raw = append(raw, '\n')

		if path == "" || path == "-" {
			_, err := cmd.OutOrStdout().Write(raw)
			return err
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Wrote schema to %s\n", path)
		return nil
	},
}

func init() {
	datasetSchemaGetCmd.Flags().String("file", "", "Write the schema to this path instead of stdout")
}
