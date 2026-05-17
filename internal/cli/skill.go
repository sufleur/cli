package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/skill"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Print the Sufleur agent skill markdown",
	Long: `Prints the Sufleur agent skill markdown to stdout, ready to be piped
into your agent tool of choice.

Examples:

  # Claude Code
  sufleur skill > ~/.claude/skills/sufleur.md

  # Cursor
  sufleur skill > .cursor/rules/sufleur.md

The content describes when and how to use the Sufleur CLI and ships with the
binary, so it always stays in sync with the commands you have available.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), skill.Markdown)
		return err
	},
}
