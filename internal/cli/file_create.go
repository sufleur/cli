package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var fileCreateCmd = &cobra.Command{
	Use:           "create @workspace/name@version --file PATH [--name NAME] [--entrypoint]",
	Short:         "Add a new file to a draft version",
	Long:          "Reads content from --file and creates a new file on the named version. The registry name defaults to the local filename with the .mustache suffix stripped; pass --name to override.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			return fmt.Errorf("--file is required")
		}
		nameFlag, _ := cmd.Flags().GetString("name")
		isEntrypoint, _ := cmd.Flags().GetBool("entrypoint")

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		registryName := stripMustacheSuffix(nameFlag)
		if registryName == "" {
			registryName = stripMustacheSuffix(filepath.Base(path))
		}
		if registryName == "" {
			return fmt.Errorf("could not derive a name; pass --name explicitly")
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		f, err := client.CreatePromptFile(cmd.Context(), ref.Workspace, ref.Name, ref.Version, registryName, string(content), isEntrypoint)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, f)
		}
		star := ""
		if f.IsEntrypoint {
			star = " (entrypoint)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s%s on @%s/%s@%s\n", f.Name, star, ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	fileCreateCmd.Flags().String("file", "", "Path to the local file whose content to upload")
	fileCreateCmd.Flags().String("name", "", "Registry name (defaults to local basename without .mustache)")
	fileCreateCmd.Flags().Bool("entrypoint", false, "Mark the new file as an entrypoint")
}

// stripMustacheSuffix removes a trailing ".mustache" if present. The registry
// stores file names without the extension; the CLI accepts either form from
// the user and normalises here.
func stripMustacheSuffix(name string) string {
	return strings.TrimSuffix(name, ".mustache")
}
