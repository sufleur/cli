package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var datasetCreateCmd = &cobra.Command{
	Use:           "create @workspace/name",
	Short:         "Create a new dataset",
	Long:          "Creates a new dataset and its initial draft version. Names follow npm conventions (lowercase, 5–214 chars; letters, digits, '-', '_', '.').",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], false)
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")
		visibility, _ := cmd.Flags().GetString("visibility")
		visibility, err = normalizeVisibility(visibility)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		d, err := client.CreateDataset(cmd.Context(), ref.Workspace, ref.Name, description, visibility)
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, d)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created @%s/%s (%s) with an initial draft\n", ref.Workspace, d.Name, d.Visibility)
		return nil
	},
}

// normalizeVisibility maps a case-insensitive private/public flag to the wire
// enum value name. An empty string is returned unchanged (omit the field).
func normalizeVisibility(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "private":
		return "PRIVATE", nil
	case "public":
		return "PUBLIC", nil
	default:
		return "", fmt.Errorf("--visibility must be private or public")
	}
}

func init() {
	datasetCreateCmd.Flags().String("description", "", "Optional description for the new dataset")
	datasetCreateCmd.Flags().String("visibility", "", "Visibility: private (default) or public")
}
