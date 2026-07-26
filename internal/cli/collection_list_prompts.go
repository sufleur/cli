package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var collectionListPromptsCmd = &cobra.Command{
	Use:           "list-prompts @workspace/+name",
	Short:         "List the prompts in a collection",
	Long:          "Lists the prompts in a collection, one @workspace/name per line. Feed each into `sufleur version dump` or `sufleur add` to work with it.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseCollectionRef(args[0])
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		names, err := client.ListCollectionPrompts(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		// Surface fully-qualified refs so the output is directly usable by
		// other commands.
		refs := make([]string, len(names))
		for i, n := range names {
			refs[i] = "@" + ref.Workspace + "/" + n
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, refs)
		}
		out := cmd.OutOrStdout()
		if len(refs) == 0 {
			fmt.Fprintln(out, "No prompts in collection.")
			return nil
		}
		for _, r := range refs {
			fmt.Fprintln(out, r)
		}
		return nil
	},
}
