package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var datasetGetCmd = &cobra.Command{
	Use:           "get @workspace/name",
	Short:         "Show a dataset and its versions",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		d, err := client.GetDataset(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, d)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "@%s/%s\n", ref.Workspace, d.Name)
		if d.Description != "" {
			fmt.Fprintf(out, "description: %s\n", d.Description)
		}
		if d.Versions == nil || len(d.Versions.Data) == 0 {
			fmt.Fprintln(out, "\nNo versions.")
			return nil
		}
		fmt.Fprintln(out, "\nversions:")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, v := range d.Versions.Data {
			fmt.Fprintf(tw, "  %s\t%s\t%d cases\t%s\n", v.Version, v.Status, v.CaseCount, v.UpdatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		return nil
	},
}
