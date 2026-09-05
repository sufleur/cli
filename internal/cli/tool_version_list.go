package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var toolVersionListCmd = &cobra.Command{
	Use:           "list @workspace/name",
	Short:         "List a tool's versions",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
		if err != nil {
			return err
		}
		status, _ := cmd.Flags().GetString("status")
		status = strings.ToUpper(status)
		if status != "" && status != "DRAFT" && status != "PUBLISHED" {
			return fmt.Errorf("--status must be DRAFT or PUBLISHED")
		}
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		page, err := client.ListToolVersions(cmd.Context(), ref.Workspace, ref.Name, status, limit, offset)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, page)
		}

		out := cmd.OutOrStdout()
		if len(page.Data) == 0 {
			fmt.Fprintln(out, "No versions found.")
			return nil
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, v := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", v.Version, v.Status, v.UpdatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		fmt.Fprintf(out, "\nShowing %d of %d.\n", len(page.Data), page.Total)
		return nil
	},
}

func init() {
	toolVersionListCmd.Flags().String("status", "", "Filter by status (DRAFT or PUBLISHED)")
	toolVersionListCmd.Flags().Int("limit", 50, "Maximum number of versions to return")
	toolVersionListCmd.Flags().Int("offset", 0, "Number of versions to skip")
}
