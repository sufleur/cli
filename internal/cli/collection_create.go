package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var collectionCreateCmd = &cobra.Command{
	Use:           "create @workspace/+name",
	Short:         "Create a new collection",
	Long:          "Creates a new (private) collection. Use `collection link` to add prompts and `collection set-readme`/`set-description` to document it.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseCollectionRef(args[0])
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		c, err := client.CreateCollection(cmd.Context(), ref.Workspace, ref.Name, description)
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
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/+%s\n", ref.Workspace, c.Name)
		return nil
	},
}

func init() {
	collectionCreateCmd.Flags().String("description", "", "Optional description for the new collection")
}
