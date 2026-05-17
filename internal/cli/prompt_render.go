package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/render"
)

var promptRenderCmd = &cobra.Command{
	Use:           "render <dir> --entrypoint NAME [--vars '{...}' | --vars-file PATH]",
	Short:         "Render a local prompt directory with Mustache",
	Long: `Reads a dump-style directory and renders one of its entrypoints.

The directory must contain a "files/" subdirectory of .mustache templates;
output-schema.json (sibling to files/) is optional. ` + "`{{@outputSchema}}`" + ` is
substituted with the pretty-JSON output schema before Mustache rendering,
matching the codegen-time behaviour.

` + "`--vars`" + ` and ` + "`--vars-file`" + ` are mutually exclusive; both expect a JSON object.
Pass neither to render with an empty variable scope.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
		if entrypoint == "" {
			return fmt.Errorf("--entrypoint is required")
		}
		entrypoint = stripMustacheSuffix(entrypoint)

		inlineVars, _ := cmd.Flags().GetString("vars")
		varsFile, _ := cmd.Flags().GetString("vars-file")
		if inlineVars != "" && varsFile != "" {
			return fmt.Errorf("--vars and --vars-file are mutually exclusive")
		}
		vars, err := loadVars(inlineVars, varsFile)
		if err != nil {
			return err
		}

		p, err := render.Load(dir)
		if err != nil {
			return err
		}
		out, err := p.Render(entrypoint, vars)
		if err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]string{"rendered": out})
		}
		fmt.Fprint(cmd.OutOrStdout(), out)
		return nil
	},
}

func init() {
	promptRenderCmd.Flags().String("entrypoint", "", "Required: name of the entrypoint file (e.g. \"welcome\" or \"welcome.mustache\")")
	promptRenderCmd.Flags().String("vars", "", "Inline JSON object of template variables")
	promptRenderCmd.Flags().String("vars-file", "", "Path to a JSON file containing the template variables")
}

func loadVars(inline, path string) (map[string]any, error) {
	var raw []byte
	switch {
	case inline != "":
		raw = []byte(inline)
	case path != "":
		r, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		raw = r
	default:
		return map[string]any{}, nil
	}
	var vars map[string]any
	if err := json.Unmarshal(raw, &vars); err != nil {
		return nil, fmt.Errorf("parsing vars as JSON object: %w", err)
	}
	return vars, nil
}
