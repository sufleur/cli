package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var toolGetCmd = &cobra.Command{
	Use:   "get @workspace/name",
	Short: "Show a tool contract and its versions",
	Long: `Shows a tool's catalog metadata, how many published prompt versions depend on
it, and its versions.

The description shown here is the catalog blurb — it is never sent to the model.
For the model-facing text, read a version: sufleur tool version get @ws/name@version`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], false)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		tool, err := client.GetTool(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, tool)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "@%s/%s\n", ref.Workspace, tool.Name)
		fmt.Fprintf(out, "visibility: %s\n", tool.Visibility)
		if tool.Description != "" {
			fmt.Fprintf(out, "description: %s\n", tool.Description)
		}
		if len(tool.Tags) > 0 {
			fmt.Fprintf(out, "tags: %v\n", tool.Tags)
		}
		if tool.DependentCount != nil {
			fmt.Fprintf(out, "dependents: %d published prompt version(s)\n", *tool.DependentCount)
		}

		if tool.Versions == nil || len(tool.Versions.Data) == 0 {
			fmt.Fprintln(out, "\nNo versions.")
			return nil
		}
		fmt.Fprintln(out, "\nversions:")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, v := range tool.Versions.Data {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", v.Version, v.Status, v.UpdatedAt.Format(time.RFC3339))
		}
		_ = tw.Flush()
		return nil
	},
}
