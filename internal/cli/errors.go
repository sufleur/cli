package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// handleError prints err to stderr in a format chosen by the matched command's
// --json flag. With --json set, output is a single-line JSON object so agent
// scripts can decode stderr; otherwise it's a plain "Error: ..." line that
// mirrors cobra's default style.
func handleError(cmd *cobra.Command, err error) {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		enc := json.NewEncoder(os.Stderr)
		payload := map[string]string{"error": err.Error()}
		var coded interface{ ErrorCode() string }
		if errors.As(err, &coded) {
			payload["code"] = coded.ErrorCode()
		}
		_ = enc.Encode(payload)
		return
	}
	fmt.Fprintln(os.Stderr, "Error:", err.Error())
}
