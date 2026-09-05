package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionSetReadmeCmd = &cobra.Command{
	Use:   "set-readme @workspace/name@draft [--content STR | --file PATH]",
	Short: "Replace a draft version's README",
	Long: `Sets the README markdown on a draft version — documentation for humans, never
sent to the model. Provide exactly one of --content, --file PATH, or --file -.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}

		content, contentSet, fileSet, err := readReadmeInput(cmd)
		if err != nil {
			return err
		}
		if contentSet && fileSet {
			return fmt.Errorf("--content and --file are mutually exclusive")
		}
		if !contentSet && !fileSet {
			return fmt.Errorf("nothing to set: pass --content or --file")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetToolVersionReadme(cmd.Context(), ref.Workspace, ref.Name, ref.Version, content)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set the README on @%s/%s@%s.\n", ref.Workspace, ref.Name, v.Version)
		return nil
	},
}

func init() {
	toolVersionSetReadmeCmd.Flags().String("content", "", "README markdown, used verbatim")
	toolVersionSetReadmeCmd.Flags().String("file", "", "Path to a markdown file (- for stdin)")
}
