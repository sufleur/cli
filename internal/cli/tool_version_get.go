package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionGetCmd = &cobra.Command{
	Use:   "get @workspace/name@version",
	Short: "Show a tool version's contract",
	Long: `Shows the model-facing description and the argument and result schemas of one
version. The version may be a semver, a constraint, or the literal "draft".`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetToolVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "@%s/%s@%s (%s)\n", ref.Workspace, ref.Name, v.Version, v.Status)
		fmt.Fprintf(out, "\nmodel description:\n%s\n", orPlaceholder(v.ModelDescription, "(not set)"))

		fmt.Fprintln(out, "\ninput schema:")
		fmt.Fprintln(out, indentJSONValue(v.InputSchema))
		if v.OutputSchema != nil {
			fmt.Fprintln(out, "\noutput schema:")
			fmt.Fprintln(out, indentJSONValue(v.OutputSchema))
		} else {
			fmt.Fprintln(out, "\noutput schema: (not set)")
		}
		if len(v.Metadata) > 0 {
			fmt.Fprintln(out, "\nmetadata:")
			fmt.Fprintln(out, indentJSONValue(v.Metadata))
		}
		return nil
	},
}

func orPlaceholder(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

// indentJSONValue renders a decoded JSON value for human output. A value that
// came off the wire as JSON always re-marshals, so the error path is unreachable
// in practice and falls back to Go's own rendering rather than failing a read.
func indentJSONValue(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}
