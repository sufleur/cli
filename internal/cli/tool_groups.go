package cli

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
)

// toolNamePattern mirrors the registry's rule, which is stricter than the one
// for prompts: no dots, and it must start with a letter. The name doubles as
// the default wire name the model sees, and providers constrain that.
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// toolCmd is the parent of `sufleur tool <action>`. Tools are workspace-scoped
// and addressed as @workspace/name, their versions as @workspace/name@version.
var toolCmd = &cobra.Command{
	Use:           "tool",
	Short:         "Manage tool contracts (workspace-scoped)",
	Long:          "Subcommands operate on tool contracts identified as @workspace/name, and their versions.",
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// toolVersionCmd is the parent of `sufleur tool version <action>`.
var toolVersionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Manage tool versions (draft → publish)",
	Long:          "Subcommands operate on versions of a tool, identified as @workspace/name@version.",
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// toolSchemaCmd is the parent of `sufleur tool schema <action>`.
var toolSchemaCmd = &cobra.Command{
	Use:           "schema",
	Short:         "Read and write a tool version's JSON Schemas",
	Long:          "Subcommands read or replace the input and output JSON Schemas of a tool version, identified as @workspace/name@version.",
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

func init() {
	// One persistent --json flag on the root group; nested subgroups inherit it.
	toolCmd.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")

	toolCmd.AddCommand(
		toolListCmd,
		toolGetCmd,
		toolCreateCmd,
		toolUpdateCmd,
		toolDumpCmd,
		toolVersionCmd,
		toolSchemaCmd,
	)
	toolVersionCmd.AddCommand(
		toolVersionDraftCmd,
		toolVersionListCmd,
		toolVersionGetCmd,
		toolVersionDeleteCmd,
		toolVersionSetDescriptionCmd,
		toolVersionSetReadmeCmd,
		toolVersionSetMetadataCmd,
	)
	toolSchemaCmd.AddCommand(toolSchemaGetCmd, toolSchemaSetCmd)
}

// parseToolRef parses @workspace/name, optionally with @version. Tool names are
// validated locally because the registry's rule is stricter than the prompt one
// and the reason (the name is a wire name) is worth explaining before a round trip.
func parseToolRef(arg string, requireVersion bool) (promptref.PromptRef, error) {
	ref, err := promptref.ParseRef(arg)
	if err != nil {
		return promptref.PromptRef{}, err
	}
	if ref.IsCollection {
		return promptref.PromptRef{}, fmt.Errorf("%q is a collection reference; tools do not use the + marker", arg)
	}
	if !toolNamePattern.MatchString(ref.Name) {
		return promptref.PromptRef{}, fmt.Errorf(
			"tool name %q must match %s — stricter than a prompt name (no dots) because it doubles as the wire name the model sees",
			ref.Name, toolNamePattern.String())
	}
	if requireVersion && ref.Version == "" {
		return promptref.PromptRef{}, fmt.Errorf("version is required (use @workspace/name@version, or @workspace/name@draft)")
	}
	return ref, nil
}
