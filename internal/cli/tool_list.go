package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var toolListCmd = &cobra.Command{
	Use:           "list @workspace",
	Short:         "List tool contracts in a workspace",
	Long:          "Lists tool contracts in a workspace. Use --search to filter by name or description.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace, err := parseWorkspaceRef(args[0])
		if err != nil {
			return err
		}
		search, _ := cmd.Flags().GetString("search")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		page, err := client.ListTools(cmd.Context(), workspace, search, limit, offset)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, page)
		}

		out := cmd.OutOrStdout()
		if len(page.Data) == 0 {
			fmt.Fprintln(out, "No tools found.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, t := range page.Data {
			fmt.Fprintf(tw, "@%s/%s\t%s\t%s\n", workspace, t.Name, t.Visibility, t.UpdatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		fmt.Fprintf(out, "\nShowing %d of %d.\n", len(page.Data), page.Total)
		return nil
	},
}

func init() {
	toolListCmd.Flags().String("search", "", "Filter tools by name or description (case-insensitive substring)")
	toolListCmd.Flags().Int("limit", 50, "Maximum number of tools to return")
	toolListCmd.Flags().Int("offset", 0, "Number of tools to skip")
}
