package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

// datasetCmd is the parent of `sufleur dataset <action>`. Subcommands operate on
// datasets identified as @workspace/name and live in workspace scope. The
// nested `version`, `schema`, and `cases` groups manage a dataset version's
// draft→publish lifecycle and its schema + cases content.
var datasetCmd = &cobra.Command{
	Use:           "dataset",
	Short:         "Manage datasets (workspace-scoped)",
	Long:          "Subcommands operate on datasets identified as @workspace/name, and their versions, schema, and cases.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// datasetVersionCmd is the parent of `sufleur dataset version <action>`.
var datasetVersionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Manage dataset versions (draft → publish)",
	Long:          "Subcommands operate on versions of a dataset, identified as @workspace/name@version.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// datasetSchemaCmd is the parent of `sufleur dataset schema <action>`.
var datasetSchemaCmd = &cobra.Command{
	Use:           "schema",
	Short:         "Read and write a dataset version's JSON Schema",
	Long:          "Subcommands read or replace the JSON Schema of a dataset version, identified as @workspace/name@version.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// datasetCasesCmd is the parent of `sufleur dataset cases <action>`.
var datasetCasesCmd = &cobra.Command{
	Use:           "cases",
	Short:         "Upload and download a dataset version's cases",
	Long:          "Subcommands push (JSONL/JSON/CSV) or pull (JSONL) the cases of a dataset version, identified as @workspace/name@version.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

func init() {
	// One persistent --json flag on the root group; nested subgroups inherit it.
	datasetCmd.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")

	datasetCmd.AddCommand(
		datasetListCmd,
		datasetGetCmd,
		datasetCreateCmd,
		datasetUpdateCmd,
		datasetDumpCmd,
		datasetVersionCmd,
		datasetSchemaCmd,
		datasetCasesCmd,
	)
	datasetVersionCmd.AddCommand(
		datasetVersionDraftCmd,
		datasetVersionListCmd,
		datasetVersionGetCmd,
		datasetVersionValidateCmd,
		datasetVersionDeleteCmd,
	)
	datasetSchemaCmd.AddCommand(
		datasetSchemaGetCmd,
		datasetSchemaSetCmd,
	)
	datasetCasesCmd.AddCommand(
		datasetCasesPushCmd,
		datasetCasesPullCmd,
	)
}

// parseDatasetRef parses a @workspace/name or @workspace/name@version reference
// for a dataset command. The "+" collection marker is rejected — it is a
// prompt/collection-only construct. When requireVersion is true, the reference
// must include an @version suffix.
func parseDatasetRef(arg string, requireVersion bool) (promptref.PromptRef, error) {
	ref, err := promptref.ParseRef(arg)
	if err != nil {
		return promptref.PromptRef{}, err
	}
	if ref.IsCollection {
		return promptref.PromptRef{}, fmt.Errorf("%q is a collection reference; datasets do not use the + marker", arg)
	}
	if requireVersion && ref.Version == "" {
		return promptref.PromptRef{}, fmt.Errorf("version is required (use @workspace/name@version, or @workspace/name@draft)")
	}
	return ref, nil
}

// parseWorkspaceRef parses a bare "@workspace" reference (no name segment).
func parseWorkspaceRef(arg string) (string, error) {
	if !strings.HasPrefix(arg, "@") {
		return "", fmt.Errorf("workspace reference %q must start with @ (e.g. @acme)", arg)
	}
	workspace := strings.TrimPrefix(arg, "@")
	if workspace == "" {
		return "", fmt.Errorf("workspace name is empty")
	}
	if strings.Contains(workspace, "/") {
		return "", fmt.Errorf("expected a workspace reference like @acme (use `dataset get` for a single dataset)")
	}
	return workspace, nil
}

// readFileOrStdin reads bytes from path, or from stdin when path is "-".
func readFileOrStdin(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return raw, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return raw, nil
}
