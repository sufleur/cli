package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var fileUpdateCmd = &cobra.Command{
	Use:           "update @workspace/name@version --name NAME [--file PATH] [--rename NEWNAME]",
	Short:         "Update a file's content and/or rename it",
	Long:          "At least one of --file or --rename must be set. Either or both can be combined in a single call.",
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
		nameFlag, _ := cmd.Flags().GetString("name")
		if nameFlag == "" {
			return fmt.Errorf("--name is required")
		}
		filePath, _ := cmd.Flags().GetString("file")
		rename, _ := cmd.Flags().GetString("rename")
		if filePath == "" && rename == "" {
			return fmt.Errorf("nothing to update: pass --file and/or --rename")
		}

		fileName := stripMustacheSuffix(nameFlag)
		newName := stripMustacheSuffix(rename)
		var newContent string
		if filePath != "" {
			raw, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", filePath, err)
			}
			newContent = string(raw)
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.UpdatePromptFile(cmd.Context(), ref.Workspace, ref.Name, ref.Version, fileName, newContent, newName)
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
		finalName := newName
		if finalName == "" {
			finalName = fileName
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated %s on @%s/%s@%s\n", finalName, ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	fileUpdateCmd.Flags().String("name", "", "Current registry name of the file to update")
	fileUpdateCmd.Flags().String("file", "", "Path to a local file whose content replaces the current content")
	fileUpdateCmd.Flags().String("rename", "", "New registry name for the file")
}
