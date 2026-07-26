package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionGetCmd = &cobra.Command{
	Use:           "get @workspace/name@version",
	Short:         "Show details for a single version",
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

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.GetPromptVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
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

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Prompt:        @%s/%s\n", ref.Workspace, ref.Name)
		fmt.Fprintf(out, "Version:       %s\n", v.Version)
		fmt.Fprintf(out, "Status:        %s\n", v.Status)
		fmt.Fprintf(out, "Created:       %s\n", v.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(out, "Updated:       %s\n", v.UpdatedAt.Format(time.RFC3339))

		if len(v.Metadata) == 0 {
			fmt.Fprintln(out, "Metadata:      (none)")
		} else {
			keys := make([]string, 0, len(v.Metadata))
			for k := range v.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Fprintf(out, "Metadata:      %s\n", strings.Join(keys, ", "))
		}

		if v.OutputSchema == nil {
			fmt.Fprintln(out, "Output schema: (none)")
		} else {
			fmt.Fprintln(out, "Output schema: set")
		}

		fmt.Fprintf(out, "Model config:  %s\n", formatModelConfigValue(v.ModelConfig))

		fmt.Fprintf(out, "Files (%d):\n", len(v.Files))
		for _, f := range v.Files {
			star := ""
			if f.IsEntrypoint {
				star = " *"
			}
			fmt.Fprintf(out, "  %s%s\n", f.Name, star)
		}
		return nil
	},
}

// formatModelConfigValue renders the value half of the "Model config:" line
// in `version get`'s human output: "(none)" when no model config is set,
// otherwise "<lowercased provider> <model> <compact one-line params JSON>".
// Nil Parameters renders as "{}" rather than JSON's "null" so the line always
// shows a valid, empty-or-populated object.
func formatModelConfigValue(mc *userapi.ModelConfig) string {
	if mc == nil {
		return "(none)"
	}
	params := mc.Parameters
	if params == nil {
		params = map[string]any{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		paramsJSON = []byte("{}")
	}
	return fmt.Sprintf("%s %s %s", strings.ToLower(mc.Provider), mc.Model, string(paramsJSON))
}
