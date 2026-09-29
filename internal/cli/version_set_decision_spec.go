package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionSetDecisionSpecCmd = &cobra.Command{
	Use:   "set-decision-spec @workspace/name@version --from-file decision.yaml",
	Short: "Replace the questions of a decision (SYSTEM_ONE) prompt version",
	Long: `Reads a decision spec (YAML or JSON) and stores it on a draft version of a
decision prompt:

  questions:
    department:
      type: choice            # noul | choice | score
      criteria:               # string values are Mustache templates
        billing: Payments for {{{product}}}
        technical: null
    about:
      type: choice
      criteria:
        none: no listed concept fits
      optionCriteria:         # callers may add options, each described by this
        what: the belief is about "{{{name}}}"
    frustration:
      type: score
      criteria: [Calm, Frustrated, Very angry]
    isUrgent:
      type: noul

Each question is a template, rendered one at a time: its id is also the file
holding its instructions, and its criteria use the same inputs. Missing files
are created empty, files the spec no longer names become partials, and the
per-question answer schemas are re-derived. Question and option order is
kept. Use {{{ }}} to insert values without HTML escaping.`,
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
		path, _ := cmd.Flags().GetString("from-file")
		if path == "" {
			return fmt.Errorf("--from-file is required")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		// YAML is a superset of JSON; parse it order-preservingly so the
		// questions (and choice options) keep the order they were written in.
		tree, err := generator.ParseStructured(string(raw))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if tree.Kind != generator.TreeMap {
			return fmt.Errorf("%s must be a mapping with a questions key", path)
		}
		spec, err := tree.MarshalJSON()
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetPromptVersionDecisionSpec(cmd.Context(), ref.Workspace, ref.Name, ref.Version, spec)
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
		count := 0
		if v.DecisionSpec != nil {
			count = len(v.DecisionSpec.Questions)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated decision spec on @%s/%s@%s (%d question(s))\n", ref.Workspace, ref.Name, ref.Version, count)
		return nil
	},
}

func init() {
	versionSetDecisionSpecCmd.Flags().String("from-file", "", "Path to a YAML or JSON decision spec")
}
