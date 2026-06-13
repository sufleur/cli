package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var collectionLinkCmd = &cobra.Command{
	Use:   "link @workspace/+collection @workspace/prompt",
	Short: "Add a prompt to a collection",
	Long: `Links a prompt into a collection. Both must live in the same workspace.

A prompt belongs to at most one collection, so linking a prompt that is already
in a different collection moves it out of that one. To avoid surprises this is
refused unless you pass --force.`,
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		collectionRef, err := parseCollectionRef(args[0])
		if err != nil {
			return err
		}
		promptArg, err := promptref.Parse(args[1])
		if err != nil {
			return err
		}
		if promptArg.IsCollection {
			return fmt.Errorf("second argument %q must be a prompt, not a collection", args[1])
		}
		if promptArg.Workspace != collectionRef.Workspace {
			return fmt.Errorf("prompt and collection must be in the same workspace (@%s vs @%s)", promptArg.Workspace, collectionRef.Workspace)
		}

		force, _ := cmd.Flags().GetBool("force")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		// Guard against silently moving a prompt out of another collection.
		current, err := client.GetPromptCurrentCollection(cmd.Context(), promptArg.Workspace, promptArg.Name)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}
		if current != "" && current != collectionRef.Name && !force {
			return fmt.Errorf("@%s/%s is already in collection @%s/+%s — re-run with --force to move it",
				promptArg.Workspace, promptArg.Name, promptArg.Workspace, current)
		}

		if err := client.SetPromptCollection(cmd.Context(), collectionRef.Workspace, promptArg.Name, collectionRef.Name); err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Linked @%s/%s to @%s/+%s\n",
			promptArg.Workspace, promptArg.Name, collectionRef.Workspace, collectionRef.Name)
		return nil
	},
}

func init() {
	collectionLinkCmd.Flags().Bool("force", false, "Move the prompt even if it is already in another collection")
}
