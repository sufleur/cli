package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionListCmd = &cobra.Command{
	Use:           "list @workspace/name",
	Short:         "List versions of a prompt",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}
		status, _ := cmd.Flags().GetString("status")
		if status != "" {
			status = strings.ToUpper(status)
			if status != "DRAFT" && status != "PUBLISHED" {
				return fmt.Errorf("--status must be DRAFT or PUBLISHED")
			}
		}
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		page, err := client.ListPromptVersions(cmd.Context(), ref.Workspace, ref.Name, status, limit, offset)
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
	versionListCmd.Flags().String("status", "", "Filter by status: DRAFT or PUBLISHED")
	versionListCmd.Flags().Int("limit", 50, "Maximum number of versions to return")
	versionListCmd.Flags().Int("offset", 0, "Number of versions to skip (for paging)")
}
