package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var datasetCasesPullCmd = &cobra.Command{
	Use:   "pull @workspace/name@version --to cases.jsonl",
	Short: "Download a version's cases as JSONL",
	Long: `Downloads a dataset version's cases (one JSON object per line) to --to, or to
stdout when --to is omitted or "-". A version with no cases yields empty output.`,
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := parseDatasetRef(args[0], true)
		if err != nil {
			return err
		}
		to, _ := cmd.Flags().GetString("to")
		force, _ := cmd.Flags().GetBool("force")
		toStdout := to == "" || to == "-"
		if !toStdout && !force {
			if _, err := os.Stat(to); err == nil {
				return fmt.Errorf("%s already exists; pass --force to overwrite", to)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("checking %s: %w", to, err)
			}
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
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

		if toStdout {
			_, err := cmd.OutOrStdout().Write(cases)
			return err
		}
		if err := os.WriteFile(to, cases, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", to, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %d case(s) to %s\n", v.CaseCount, to)
		return nil
	},
}

func init() {
	datasetCasesPullCmd.Flags().String("to", "", "Write cases to this path instead of stdout")
	datasetCasesPullCmd.Flags().Bool("force", false, "Overwrite the destination file if it exists")
}
