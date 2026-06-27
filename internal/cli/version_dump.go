package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

var versionDumpCmd = &cobra.Command{
	Use:           "dump @workspace/name@version --to ./dir",
	Short:         "Export a version's files, output schema, and metadata into a directory",
	Long: `Writes the version into the target directory:

  <dir>/files/<filename>      one file per PromptFile, content verbatim
  <dir>/output-schema.json    pretty-printed JSON; omitted if the version has no schema
  <dir>/README.md             raw markdown; always written (empty if never set)
  <dir>/metadata.yaml         flat key-value YAML; "{}" if metadata is empty
  <dir>/eval.yaml             the version's eval config as YAML; always written
                              (a complete skeleton if no eval is configured)

The directory is created if it doesn't exist. Pass --force to overwrite a
non-empty directory; otherwise dump aborts if the target already has files.`,
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

		v, err := client.GetPromptVersion(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		// Fetch the eval YAML before any disk write so a failure here doesn't
		// leave a partial dump. The backend always returns a complete skeleton.
		evalYAML, err := client.GetEvalYaml(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		if err := writeDump(dir, v, evalYAML); err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{
				"directory":       dir,
				"files":           len(v.Files),
				"hasOutputSchema": v.OutputSchema != nil,
				"readmeBytes":     len(v.Readme),
				"metadataKeys":    len(v.Metadata),
				"evalBytes":       len(evalYAML),
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Dumped @%s/%s@%s to %s (%d files)\n",
			ref.Workspace, ref.Name, ref.Version, dir, len(v.Files))
		return nil
	},
}

func init() {
	versionDumpCmd.Flags().String("to", "", "Destination directory")
	versionDumpCmd.Flags().Bool("force", false, "Overwrite a non-empty destination directory")
}

func prepareDumpDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.MkdirAll(dir, 0o755)
		}
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("%s is not empty; pass --force to overwrite", dir)
	}
	return nil
}

func writeDump(dir string, v *userapi.PromptVersion, evalYAML string) error {
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filesDir, err)
	}
	for _, f := range v.Files {
		if strings.ContainsAny(f.Name, "/\\") {
			return fmt.Errorf("file name %q contains a path separator; refusing to write", f.Name)
		}
		path := filepath.Join(filesDir, f.Name+".mustache")
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	if v.OutputSchema != nil {
		schemaPath := filepath.Join(dir, "output-schema.json")
		raw, err := json.MarshalIndent(v.OutputSchema, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding output schema: %w", err)
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(schemaPath, raw, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", schemaPath, err)
		}
	}

	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(v.Readme), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", readmePath, err)
	}

	metadataPath := filepath.Join(dir, "metadata.yaml")
	flat := flattenMetadata(v.Metadata)
	raw, err := yaml.Marshal(flat)
	if err != nil {
		return fmt.Errorf("encoding metadata: %w", err)
	}
	if len(flat) == 0 {
		raw = []byte("{}\n")
	}
	if err := os.WriteFile(metadataPath, raw, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", metadataPath, err)
	}

	evalPath := filepath.Join(dir, "eval.yaml")
	evalRaw := []byte(evalYAML)
	if len(evalRaw) == 0 || evalRaw[len(evalRaw)-1] != '\n' {
		evalRaw = append(evalRaw, '\n')
	}
	if err := os.WriteFile(evalPath, evalRaw, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", evalPath, err)
	}
	return nil
}

// flattenMetadata unwraps the backend's {type, value} wrappers so the dumped
// YAML is flat scalar key→value pairs that round-trip cleanly with
// `version set-metadata --from-file`.
func flattenMetadata(m map[string]any) map[string]any {
	if len(m) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if wrapper, ok := v.(map[string]any); ok {
			if val, hasValue := wrapper["value"]; hasValue {
				out[k] = val
				continue
			}
		}
		out[k] = v
	}
	return out
}
