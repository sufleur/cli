package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var collectionSetReadmeCmd = &cobra.Command{
	Use:   "set-readme @workspace/+name [--content STR | --file PATH]",
	Short: "Replace the README of a collection",
	Long: `Sets the README markdown on a collection. Provide exactly one of:

  --content STR    use the string verbatim
  --file PATH      read content from a file
  --file -         read content from stdin

The edit is applied immediately (collections have no draft workflow). The
backend enforces a length limit, which surfaces as an error.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseCollectionRef(args[0])
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

		c, err := client.UpdateCollectionReadme(cmd.Context(), ref.Workspace, ref.Name, content)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, c)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated README on @%s/+%s\n", ref.Workspace, ref.Name)
		return nil
	},
}

func init() {
	collectionSetReadmeCmd.Flags().String("content", "", "README markdown as an inline string")
	collectionSetReadmeCmd.Flags().String("file", "", "Path to a markdown file; use - to read from stdin")
}
