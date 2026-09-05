package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/userapi"
)

var toolDumpCmd = &cobra.Command{
	Use:   "dump @workspace/name@version --to ./dir",
	Short: "Export a tool version's contract into a directory",
	Long: `Writes the version into the target directory:

  <dir>/input-schema.json    the arguments the model emits; always written
  <dir>/output-schema.json   the result your implementation returns; omitted if unset
  <dir>/description.md       the model-facing description (versioned)
  <dir>/README.md            documentation for humans; always written
  <dir>/metadata.yaml        free-form metadata; "{}" when empty
  <dir>/tool.yaml            catalog metadata (name, description, visibility, tags) — read-only

description.md and tool.yaml's description are different things: the first is
what the model reads, the second is the catalog blurb.

The directory is created if it doesn't exist. Pass --force to overwrite a
non-empty directory.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseToolRef(args[0], true)
		if err != nil {
			return err
		}
		dir, _ := cmd.Flags().GetString("to")
		if dir == "" {
			return fmt.Errorf("--to is required")
		}
		force, _ := cmd.Flags().GetBool("force")

		if err := prepareDumpDir(dir, force); err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		// Everything is fetched before anything is written, so a failure part
		// way through leaves no half-populated directory.
		tool, err := client.GetTool(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}
		version, err := client.GetToolVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}

		written, err := writeToolDump(dir, tool, version)
		if err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{
				"tool":         "@" + ref.Workspace + "/" + tool.Name,
				"version":      version.Version,
				"directory":    dir,
				"filesWritten": written,
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Dumped @%s/%s@%s to %s (%d files).\n",
			ref.Workspace, tool.Name, version.Version, dir, written)
		return nil
	},
}

// writeToolDump writes the dump files and returns how many it wrote. Split out
// so the layout can be tested without a server.
func writeToolDump(dir string, tool *userapi.Tool, version *userapi.ToolVersion) (int, error) {
	written := 0

	inputSchema, err := json.MarshalIndent(version.InputSchema, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("rendering input schema: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "input-schema.json"), append(inputSchema, '\n'), 0o644); err != nil {
		return 0, fmt.Errorf("writing input-schema.json: %w", err)
	}
	written++

	if version.OutputSchema != nil {
		outputSchema, err := json.MarshalIndent(version.OutputSchema, "", "  ")
		if err != nil {
			return 0, fmt.Errorf("rendering output schema: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "output-schema.json"), append(outputSchema, '\n'), 0o644); err != nil {
			return 0, fmt.Errorf("writing output-schema.json: %w", err)
		}
		written++
	}

	if err := os.WriteFile(filepath.Join(dir, "description.md"), []byte(version.ModelDescription), 0o644); err != nil {
		return 0, fmt.Errorf("writing description.md: %w", err)
	}
	written++

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(version.Readme), 0o644); err != nil {
		return 0, fmt.Errorf("writing README.md: %w", err)
	}
	written++

	metadata := version.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataYAML, err := yaml.Marshal(metadata)
	if err != nil {
		return 0, fmt.Errorf("rendering metadata: %w", err)
	}
	if len(metadata) == 0 {
		metadataYAML = []byte("{}\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), metadataYAML, 0o644); err != nil {
		return 0, fmt.Errorf("writing metadata.yaml: %w", err)
	}
	written++

	// Informational only — nothing reads this back, so it records the catalog
	// facts a reader needs without pretending they can be pushed from here.
	toolYAML, err := yaml.Marshal(map[string]any{
		"name":        tool.Name,
		"description": tool.Description,
		"visibility":  tool.Visibility,
		"tags":        tool.Tags,
		"version":     version.Version,
		"status":      version.Status,
	})
	if err != nil {
		return 0, fmt.Errorf("rendering tool.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.yaml"), toolYAML, 0o644); err != nil {
		return 0, fmt.Errorf("writing tool.yaml: %w", err)
	}
	written++

	return written, nil
}

func init() {
	toolDumpCmd.Flags().String("to", "", "Directory to write the contract into")
	toolDumpCmd.Flags().Bool("force", false, "Overwrite a non-empty directory")
}
