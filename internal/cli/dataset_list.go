package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var datasetListCmd = &cobra.Command{
	Use:           "list @workspace",
	Short:         "List datasets in a workspace",
	Long:          "Lists datasets in a workspace. Use --search to filter by name or description.",
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

		page, err := client.ListDatasets(cmd.Context(), workspace, search, limit, offset)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, page)
		}

		out := cmd.OutOrStdout()
		if len(page.Data) == 0 {
			fmt.Fprintln(out, "No datasets found.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, d := range page.Data {
			fmt.Fprintf(tw, "@%s/%s\t%s\n", workspace, d.Name, d.UpdatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		fmt.Fprintf(out, "\nShowing %d of %d.\n", len(page.Data), page.Total)
		return nil
	},
}

func init() {
	datasetListCmd.Flags().String("search", "", "Filter datasets by name or description (case-insensitive substring)")
	datasetListCmd.Flags().Int("limit", 50, "Maximum number of datasets to return")
	datasetListCmd.Flags().Int("offset", 0, "Number of datasets to skip (for paging)")
}
