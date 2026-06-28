package cli

import (
	"fmt"
	"io"

	"github.com/sufleur/cli/internal/userapi"
)

// writeDatasetValidation prints a validation report in human-readable form: a
// one-line summary plus one line per violation. Nothing beyond the summary is
// printed when the report is valid.
func writeDatasetValidation(out io.Writer, v *userapi.DatasetValidation) {
	if v == nil {
		return
	}
	if v.Valid {
		fmt.Fprintf(out, "validation: ok (%d cases)\n", v.CaseCount)
		return
	}
	fmt.Fprintf(out, "validation: %d violation(s) across %d cases\n", len(v.Violations), v.CaseCount)
	for _, vio := range v.Violations {
		fmt.Fprintf(out, "  case %d %s: %s\n", vio.CaseIndex, vio.Constraint, vio.Message)
	}
}
