package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var promptListCmd = &cobra.Command{
	Use:           "list <workspace>",
	Short:         "List prompts in a workspace",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace := strings.TrimPrefix(args[0], "@")
		if strings.Contains(workspace, "/") {
			return fmt.Errorf("expected a workspace name like 'acme' (use `prompt get` for a single prompt)")
		}
		search, _ := cmd.Flags().GetString("search")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		page, err := client.ListPrompts(cmd.Context(), workspace, search, limit, offset)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, page)
		}

		out := cmd.OutOrStdout()
		if len(page.Data) == 0 {
			fmt.Fprintln(out, "No prompts found.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, p := range page.Data {
			fmt.Fprintf(tw, "@%s/%s\t%s\t%s\n", workspace, p.Name, p.Visibility, p.Description)
		}
		_ = tw.Flush()
		fmt.Fprintf(out, "\nShowing %d of %d.\n", len(page.Data), page.Total)
		return nil
	},
}

func init() {
	promptListCmd.Flags().String("search", "", "Filter prompts by name substring")
	promptListCmd.Flags().Int("limit", 50, "Maximum number of prompts to return")
	promptListCmd.Flags().Int("offset", 0, "Number of prompts to skip (for paging)")
}
