package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionSetReadmeCmd = &cobra.Command{
	Use:   "set-readme @workspace/name@version [--content STR | --file PATH]",
	Short: "Replace the README of a draft version",
	Long: `Sets the README markdown on a draft version. Provide exactly one of:

  --content STR    use the string verbatim
  --file PATH      read content from a file
  --file -         read content from stdin

Empty content is accepted (the backend stores it as an empty README). The
backend rejects writes to published versions and enforces a length limit;
both surface as errors.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
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

		v, err := client.SetPromptVersionReadme(cmd.Context(), ref.Workspace, ref.Name, ref.Version, content)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated README on @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	versionSetReadmeCmd.Flags().String("content", "", "README markdown as an inline string")
	versionSetReadmeCmd.Flags().String("file", "", "Path to a markdown file; use - to read from stdin")
}

func readReadmeInput(cmd *cobra.Command) (content string, contentSet, fileSet bool, err error) {
	contentSet = cmd.Flags().Changed("content")
	fileSet = cmd.Flags().Changed("file")

	if contentSet {
		content, _ = cmd.Flags().GetString("content")
	}
	if fileSet {
		path, _ := cmd.Flags().GetString("file")
		if path == "-" {
			raw, rerr := io.ReadAll(cmd.InOrStdin())
			if rerr != nil {
				return "", contentSet, fileSet, fmt.Errorf("reading stdin: %w", rerr)
			}
			content = string(raw)
		} else {
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				return "", contentSet, fileSet, fmt.Errorf("reading %s: %w", path, rerr)
			}
			content = string(raw)
		}
	}
	return content, contentSet, fileSet, nil
}
