package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/toolschema"
	"github.com/sufleur/cli/internal/userapi"
)

var toolSchemaGetCmd = &cobra.Command{
	Use:   "get @workspace/name@version",
	Short: "Print a tool version's input or output schema",
	Long: `Prints one of a version's JSON Schemas to stdout, or writes it to --file.

Defaults to the input schema; pass --output for the result schema.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}
		wantOutput, _ := cmd.Flags().GetBool("output")
		path, _ := cmd.Flags().GetString("file")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetToolVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		schema := v.InputSchema
		if wantOutput {
			if v.OutputSchema == nil {
				return fmt.Errorf("@%s/%s@%s has no output schema", ref.Workspace, ref.Name, v.Version)
			}
			schema = v.OutputSchema
		}

		raw, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("rendering schema: %w", err)
		}
		raw = append(raw, '\n')

		if path != "" {
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s.\n", path)
			return nil
		}
		_, err = cmd.OutOrStdout().Write(raw)
		return err
	},
}

var toolSchemaSetCmd = &cobra.Command{
	Use:   "set @workspace/name@draft --file schema.json",
	Short: "Replace a draft version's input or output schema",
	Long: `Replaces one of a draft version's JSON Schemas from a file (- for stdin).

Defaults to the input schema; pass --output for the result schema, or
--output --clear to remove it.

The schema is checked locally first, against the subset the code generators can
express. The registry accepts more than that, but anything outside it silently
becomes "unknown" in generated TypeScript and "Any" in generated Python — a
failure you would otherwise only notice much later.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}
		wantOutput, _ := cmd.Flags().GetBool("output")
		clear, _ := cmd.Flags().GetBool("clear")
		path, _ := cmd.Flags().GetString("file")

		if clear && !wantOutput {
			return fmt.Errorf("--clear only applies to the output schema; a tool's input schema is required")
		}
		if clear && path != "" {
			return fmt.Errorf("--clear and --file are mutually exclusive")
		}
		if !clear && path == "" {
			return fmt.Errorf("--file is required (use - for stdin)")
		}

		// Read and check the schema before touching credentials or the network:
		// a round trip that succeeds and then generates `Any` is worse than a
		// refusal that names the property, and an unauthenticated user should
		// still learn their schema is wrong.
		var schema map[string]any
		if !clear {
			raw, err := readFileOrStdin(cmd, path)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				return fmt.Errorf("parsing schema as a JSON object: %w", err)
			}

			var issues []toolschema.Issue
			if wantOutput {
				issues = toolschema.ValidateOutput(schema)
			} else {
				issues = toolschema.ValidateInput(schema)
			}
			if len(issues) > 0 {
				var b strings.Builder
				fmt.Fprintf(&b, "%d schema issue(s); nothing was sent:\n", len(issues))
				for _, issue := range issues {
					fmt.Fprintf(&b, "%s\n", issue)
				}
				return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
			}
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		if clear {
			v, err := client.SetToolVersionOutputSchema(cmd.Context(), ref.Workspace, ref.Name, ref.Version, nil)
			if err != nil {
				return mapBearer(err)
			}
			return reportSchemaSet(cmd, ref.Workspace, ref.Name, v.Version, "Cleared the output schema on")
		}

		var v *userapi.ToolVersion
		if wantOutput {
			v, err = client.SetToolVersionOutputSchema(cmd.Context(), ref.Workspace, ref.Name, ref.Version, schema)
		} else {
			v, err = client.SetToolVersionInputSchema(cmd.Context(), ref.Workspace, ref.Name, ref.Version, schema)
		}
		if err != nil {
			return mapBearer(err)
		}

		verb := "Set the input schema on"
		if wantOutput {
			verb = "Set the output schema on"
		}
		return reportSchemaSet(cmd, ref.Workspace, ref.Name, v.Version, verb)
	},
}

func reportSchemaSet(cmd *cobra.Command, workspace, name, version, verb string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		return printJSON(cmd, map[string]any{"tool": "@" + workspace + "/" + name, "version": version})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s @%s/%s@%s.\n", verb, workspace, name, version)
	return nil
}

func init() {
	toolSchemaGetCmd.Flags().Bool("output", false, "Read the output schema instead of the input schema")
	toolSchemaGetCmd.Flags().String("file", "", "Write the schema to this path instead of stdout")

	toolSchemaSetCmd.Flags().Bool("output", false, "Write the output schema instead of the input schema")
	toolSchemaSetCmd.Flags().Bool("clear", false, "Remove the output schema (implies --output)")
	toolSchemaSetCmd.Flags().String("file", "", "Path to a JSON Schema file (- for stdin)")
}
