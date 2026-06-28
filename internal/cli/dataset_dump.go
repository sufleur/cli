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

var datasetDumpCmd = &cobra.Command{
	Use:   "dump @workspace/name@version --to ./dir",
	Short: "Export a dataset version's schema, cases, and metadata to a directory",
	Long: `Writes the version into the target directory:

  <dir>/schema.json     the JSON Schema (pretty-printed; "{}" when unset)
  <dir>/cases.jsonl     one JSON object per line (empty when the version has no cases)
  <dir>/dataset.yaml    name, description, visibility, version, status, caseCount

The directory is created if it doesn't exist. Pass --force to overwrite a
non-empty directory. Push changes back with ` + "`dataset schema set`" + `,
` + "`dataset cases push`" + `, and ` + "`dataset update`" + `.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
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

		// Fetch everything before any disk write so a failure leaves no partial dump.
		d, err := client.GetDataset(cmd.Context(), ref.Workspace, ref.Name)
		if err != nil {
			return mapBearer(err)
		}
		v, err := client.GetDatasetVersionForDownload(cmd.Context(), ref.Workspace, ref.Name, ref.Version)
		if err != nil {
			return mapBearer(err)
		}
		var cases []byte
		if v.CasesDownloadURL != "" {
			cases, err = client.DownloadCases(cmd.Context(), v.CasesDownloadURL)
			if err != nil {
				return err
			}
		}

		if err := writeDatasetDump(dir, d, v, cases); err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, map[string]any{
				"directory": dir,
				"version":   v.Version,
				"status":    v.Status,
				"caseCount": v.CaseCount,
				"hasSchema": len(v.Schema) > 0,
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Dumped @%s/%s@%s to %s (%d cases)\n", ref.Workspace, ref.Name, v.Version, dir, v.CaseCount)
		return nil
	},
}

func writeDatasetDump(dir string, d *userapi.Dataset, v *userapi.DatasetVersion, cases []byte) error {
	schema := v.Schema
	if schema == nil {
		schema = map[string]any{}
	}
	schemaRaw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding schema: %w", err)
	}
	schemaRaw = append(schemaRaw, '\n')
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, schemaRaw, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", schemaPath, err)
	}

	casesPath := filepath.Join(dir, "cases.jsonl")
	if err := os.WriteFile(casesPath, cases, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", casesPath, err)
	}

	meta := map[string]any{
		"name":        d.Name,
		"description": d.Description,
		"visibility":  d.Visibility,
		"version":     v.Version,
		"status":      v.Status,
		"caseCount":   v.CaseCount,
	}
	metaRaw, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding dataset.yaml: %w", err)
	}
	metaPath := filepath.Join(dir, "dataset.yaml")
	if err := os.WriteFile(metaPath, metaRaw, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", metaPath, err)
	}
	return nil
}

func init() {
	datasetDumpCmd.Flags().String("to", "", "Destination directory")
	datasetDumpCmd.Flags().Bool("force", false, "Overwrite a non-empty destination directory")
}
