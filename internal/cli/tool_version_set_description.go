package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolVersionSetDescriptionCmd = &cobra.Command{
	Use:   "set-description @workspace/name@draft [--content STR | --file PATH]",
	Short: "Replace a draft version's model-facing description",
	Long: `Sets the description the model reads when deciding whether to call this tool.
It is versioned and frozen on publish.

This is not "sufleur tool update --description", which sets the unversioned
catalog blurb used in listing and search and is never sent to the model.

Provide exactly one of:

  --content STR    use the string verbatim
  --file PATH      read from a file
  --file -         read from stdin`,
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

		v, err := client.SetToolVersionModelDescription(cmd.Context(), ref.Workspace, ref.Name, ref.Version, content)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set the model description on @%s/%s@%s.\n", ref.Workspace, ref.Name, v.Version)
		return nil
	},
}

func init() {
	toolVersionSetDescriptionCmd.Flags().String("content", "", "Description text, used verbatim")
	toolVersionSetDescriptionCmd.Flags().String("file", "", "Path to a file containing the description (- for stdin)")
}
