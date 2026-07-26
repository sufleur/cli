package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var datasetCasesPushCmd = &cobra.Command{
	Use:   "push @workspace/name@version --file cases.jsonl",
	Short: "Upload cases to a draft version (JSONL/JSON/CSV)",
	Long: `Uploads a cases file to a draft version. The format is detected from the file
extension (.jsonl, .json, or .csv); pass --format to override, which is required
when reading from stdin (--file -). On the first upload to a fresh draft the
backend infers the schema and may suggest enums.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			return fmt.Errorf("--file is required (path to a cases file, or - for stdin)")
		}
		format, _ := cmd.Flags().GetString("format")
		filename, err := casesUploadFilename(path, format)
		if err != nil {
			return err
		}

		raw, err := readFileOrStdin(cmd, path)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		res, err := client.IngestCases(cmd.Context(), ref.Workspace, ref.Name, ref.Version, filename, bytes.NewReader(raw))
		if err != nil {
			return mapBearer(err)
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, res)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Uploaded %d case(s) to @%s/%s@%s\n", res.CaseCount, ref.Workspace, ref.Name, ref.Version)
		if res.SchemaInferred {
			fmt.Fprintln(out, "schema: inferred from this upload")
		}
		for _, s := range res.EnumSuggestions {
			fmt.Fprintf(out, "enum suggestion: %s → %v\n", s.Field, s.Values)
		}
		writeDatasetValidation(out, res.Validation)
		return nil
	},
}

// casesUploadFilename picks the multipart filename whose extension drives the
// backend's format detection. An explicit --format wins; otherwise a real path
// keeps its own extension. Stdin requires an explicit --format.
func casesUploadFilename(path, format string) (string, error) {
	if format != "" {
		f := strings.ToLower(strings.TrimSpace(format))
		switch f {
		case "jsonl", "json", "csv":
			return "cases." + f, nil
		default:
			return "", fmt.Errorf("--format must be jsonl, json, or csv")
		}
	}
	if path == "-" {
		return "", fmt.Errorf("--format is required when reading cases from stdin (jsonl|json|csv)")
	}
	return filepath.Base(path), nil
}

func init() {
	datasetCasesPushCmd.Flags().String("file", "", "Path to a cases file (.jsonl/.json/.csv); use - to read from stdin")
	datasetCasesPushCmd.Flags().String("format", "", "Override format detection: jsonl, json, or csv (required with --file -)")
}
