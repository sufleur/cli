package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/render"
)

var promptRenderCmd = &cobra.Command{
	Use:   "render <dir> [--entrypoint NAME] [--vars '{...}' | --vars-file PATH] [--state JSON | --state-file PATH]",
	Short: "Render a local prompt directory with Mustache",
	Long: `Reads a dump-style directory and renders one of its entrypoints.

The directory must contain a "files/" subdirectory of .mustache templates;
output-schema.json (sibling to files/) is optional. ` + "`{{@outputSchema}}`" + ` is
substituted with the pretty-JSON output schema before Mustache rendering,
matching the codegen-time behaviour.

` + "`--vars`" + ` and ` + "`--vars-file`" + ` are mutually exclusive; both expect a JSON object.
Pass neither to render with an empty variable scope.

Decision (SYSTEM_ONE) prompts: when the directory has a decision.yaml and no
--entrypoint is given, the whole System-One request body is rendered instead:
{model, state, questions}. --vars then maps each question/state file name to
its inputs (e.g. {"state": {"ticket": {...}}}); prompts without a state file
take the raw state from --state (JSON, or plain text) or --state-file.
` + "`{{@field path}}`" + ` renders as a backtick-quoted state path.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		entrypoint, _ := cmd.Flags().GetString("entrypoint")
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
		if entrypoint == "" {
			if p.DecisionSpec == nil {
				return fmt.Errorf("--entrypoint is required")
			}
			return renderDecision(cmd, p, vars)
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
	promptRenderCmd.Flags().String("state", "", "Decision prompts without a state file: the raw state (JSON, or plain text)")
	promptRenderCmd.Flags().String("state-file", "", "Decision prompts without a state file: path to the raw state (JSON, or plain text)")
}

func renderDecision(cmd *cobra.Command, p *render.PromptDir, vars map[string]any) error {
	inlineState, _ := cmd.Flags().GetString("state")
	stateFile, _ := cmd.Flags().GetString("state-file")
	if inlineState != "" && stateFile != "" {
		return fmt.Errorf("--state and --state-file are mutually exclusive")
	}
	var state any
	rawState := inlineState
	if stateFile != "" {
		raw, err := os.ReadFile(stateFile)
		if err != nil {
			return fmt.Errorf("reading %s: %w", stateFile, err)
		}
		rawState = string(raw)
	}
	if rawState != "" {
		if err := json.Unmarshal([]byte(rawState), &state); err != nil {
			state = rawState
		}
	}

	inputsByFile := make(map[string]map[string]any, len(vars))
	for name, v := range vars {
		inputs, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("--vars for a decision prompt maps file names to input objects; %q is not an object", name)
		}
		inputsByFile[stripMustacheSuffix(name)] = inputs
	}

	out, err := p.RenderDecision(state, inputsByFile)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
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
