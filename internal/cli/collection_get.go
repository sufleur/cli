package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/userapi"
)

var collectionGetCmd = &cobra.Command{
	Use:           "get @workspace/+name",
	Short:         "Show details for a collection (including its README)",
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

		c, err := client.GetCollection(cmd.Context(), ref.Workspace, ref.Name)
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
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Name:        @%s/+%s\n", ref.Workspace, c.Name)
		fmt.Fprintf(out, "Description: %s\n", c.Description)
		fmt.Fprintf(out, "Visibility:  %s\n", c.Visibility)
		fmt.Fprintf(out, "Prompts:     %d\n", c.PromptCount)
		fmt.Fprintf(out, "Created:     %s\n", c.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(out, "Updated:     %s\n", c.UpdatedAt.Format(time.RFC3339))
		if c.Readme != "" {
			fmt.Fprintf(out, "\n--- README ---\n%s\n", c.Readme)
		}
		return nil
	},
}
