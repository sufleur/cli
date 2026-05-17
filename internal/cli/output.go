package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// printJSON encodes v as indented JSON to the command's stdout. Used by the
// agent command tree when `--json` is set.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
