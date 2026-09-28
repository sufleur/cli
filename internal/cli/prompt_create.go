package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var promptCreateCmd = &cobra.Command{
	Use:           "create @workspace/name",
	Short:         "Create a new prompt",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")
		kindFlag, _ := cmd.Flags().GetString("kind")
		var kind string
		switch strings.ToLower(kindFlag) {
		case "", "llm":
			kind = ""
		case "system-one", "system_one", "decision":
			kind = "SYSTEM_ONE"
		default:
			return fmt.Errorf("--kind must be llm or system-one (got %q)", kindFlag)
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		p, err := client.CreatePrompt(cmd.Context(), ref.Workspace, ref.Name, description, kind)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, p)
		}
		if kind == "SYSTEM_ONE" {
			fmt.Fprintf(cmd.OutOrStdout(), "Created decision prompt @%s/%s with an initial draft seeded with one noul question (isRelevant) on jev-latest. Edit the questions with `sufleur version set-decision-spec`.\n", ref.Workspace, p.Name)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/%s with an initial draft seeded with two empty entrypoint files: systemPrompt, userPrompt (reuse or delete them)\n", ref.Workspace, p.Name)
		return nil
	},
}

func init() {
	promptCreateCmd.Flags().String("description", "", "Optional description for the new prompt")
	promptCreateCmd.Flags().String("kind", "llm", "Prompt kind: llm (chat templates) or system-one (typed decision questions for System-One models such as TypeSafe Jev); fixed at creation")
}
