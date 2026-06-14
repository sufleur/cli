package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

// collectionCmd is the parent of `sufleur collection <action>`. Subcommands
// operate on collections, identified as @workspace/+name. Collections have no
// draft→publish workflow — every edit is applied immediately.
var collectionCmd = &cobra.Command{
	Use:           "collection",
	Short:         "Manage prompt collections (workspace-scoped)",
	Long:          "Subcommands operate on collections, identified as @workspace/+name.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// parseCollectionRef parses a "@workspace/+name" argument, requiring the "+"
// collection marker. It rejects plain prompt references and version suffixes so
// the collection subcommands give a clear error instead of a confusing backend
// failure.
func parseCollectionRef(arg string) (promptref.PromptRef, error) {
	ref, err := promptref.Parse(arg)
	if err != nil {
		return promptref.PromptRef{}, err
	}
	if !ref.IsCollection {
		return promptref.PromptRef{}, fmt.Errorf("%q is not a collection reference — collections are written @workspace/+name", arg)
	}
	return ref, nil
}

func init() {
	collectionCmd.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")
	collectionCmd.AddCommand(
		collectionCreateCmd,
		collectionGetCmd,
		collectionListPromptsCmd,
		collectionLinkCmd,
		collectionSetReadmeCmd,
		collectionSetDescriptionCmd,
	)
}
