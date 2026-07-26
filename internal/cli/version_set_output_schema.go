package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionSetOutputSchemaCmd = &cobra.Command{
	Use:           "set-output-schema @workspace/name@version --file schema.json",
	Short:         "Replace the output schema of a version",
	Long:          "Reads a JSON object from --file and stores it as the version's output schema. Pass an empty JSON object `{}` to clear constraints without removing the schema.",
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
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			return fmt.Errorf("--file is required")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				// The JSON parsed fine but isn't an object (e.g. an array,
				// string, or number) — say so plainly instead of leaking Go's
				// "cannot unmarshal ... into Go value of type
				// map[string]interface {}" wording.
				return fmt.Errorf("%s must be a JSON object (got a JSON %s)", path, typeErr.Value)
			}
			return fmt.Errorf("parsing %s as JSON: %w", path, err)
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetPromptVersionOutputSchema(cmd.Context(), ref.Workspace, ref.Name, ref.Version, schema)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated output schema on @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	versionSetOutputSchemaCmd.Flags().String("file", "", "Path to a JSON file containing the output schema")
}
