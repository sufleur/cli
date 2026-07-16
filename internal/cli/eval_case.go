package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var evalCaseCmd = &cobra.Command{
	Use:           "case <run-id> <index>",
	Short:         "Show one case's drill-down for an eval run",
	Long:          "Fetches per-case detail for a run and prints one case's inputs, output, assertion and judge results. Use --prompts to also print the rendered candidate and judge prompts, or --json for the full structured case.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid case index %q: must be an integer", args[1])
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		detail, err := client.GetEvalRunDetail(cmd.Context(), args[0])
		if err != nil {
			return mapBearer(err)
		}

		matched, found := caseByIndex(detail, index)

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if !found {
				return caseNotFoundError(detail, index)
			}
			return printJSON(cmd, matched)
		}

		out := cmd.OutOrStdout()
		if len(detail.Cases) == 0 {
			fmt.Fprintln(out, evalDetailUnavailableMsg(&detail.EvalRun))
			return nil
		}
		if !found {
			return caseNotFoundError(detail, index)
		}

		prompts, _ := cmd.Flags().GetBool("prompts")
		writeCaseDetail(out, detail, matched, prompts)
		return nil
	},
}

func init() {
	evalCaseCmd.Flags().Bool("prompts", false, "Also print the rendered candidate and judge prompts")
}

// caseByIndex finds a case by its CaseIndex.
func caseByIndex(d *userapi.EvalRunDetail, index int) (*userapi.EvalRunCaseDetail, bool) {
	for i := range d.Cases {
		if d.Cases[i].CaseIndex == index {
			return &d.Cases[i], true
		}
	}
	return nil, false
}

// caseNotFoundError reports a missing case index, naming the valid indices
// when the run has any case detail, or explaining that the run has none.
func caseNotFoundError(d *userapi.EvalRunDetail, index int) error {
	if len(d.Cases) == 0 {
		return fmt.Errorf("%s", evalDetailUnavailableMsg(&d.EvalRun))
	}
	indices := make([]string, len(d.Cases))
	for i, c := range d.Cases {
		indices[i] = strconv.Itoa(c.CaseIndex)
	}
	return fmt.Errorf("case %d not found in run %s; valid indices: %s", index, d.ID, strings.Join(indices, ", "))
}
