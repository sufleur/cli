package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var datasetSchemaSetCmd = &cobra.Command{
	Use:           "set @workspace/name@version --file schema.json",
	Short:         "Replace a draft version's JSON Schema",
	Long:          "Reads a JSON Schema (draft-07) object from --file, or stdin when --file is \"-\", and stores it on a draft version. Prints the refreshed validation report.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			return fmt.Errorf("--file is required (path to a JSON Schema, or - for stdin)")
		}
		raw, err := readFileOrStdin(cmd, path)
		if err != nil {
			return err
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			return fmt.Errorf("parsing schema as a JSON object: %w", err)
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetDatasetSchema(cmd.Context(), ref.Workspace, ref.Name, ref.Version, schema)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Updated schema on @%s/%s@%s\n", ref.Workspace, ref.Name, v.Version)
		writeDatasetValidation(out, v.Validation)
		return nil
	},
}

func init() {
	datasetSchemaSetCmd.Flags().String("file", "", "Path to a JSON Schema file; use - to read from stdin")
}
