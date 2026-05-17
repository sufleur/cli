package cli

import "github.com/spf13/cobra"

// promptCmd is the parent of `sufleur prompt <action>`. Subcommands operate
// on prompts identified as @workspace/name and live in workspace scope.
var promptCmd = &cobra.Command{
	Use:           "prompt",
	Short:         "Manage prompts (workspace-scoped)",
	Long:          "Subcommands operate on prompts identified as @workspace/name.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// versionCmd is the parent of `sufleur version <action>`. Subcommands operate
// on versions of a prompt, identified as @workspace/name@version.
var versionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Manage prompt versions",
	Long:          "Subcommands operate on versions of a prompt, identified as @workspace/name@version.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

// fileCmd is the parent of `sufleur file <action>`. Subcommands operate on
// files inside a draft version (@workspace/name@version + file name).
var fileCmd = &cobra.Command{
	Use:           "file",
	Short:         "Manage files within a draft prompt version",
	Long:          "Subcommands operate on files inside a draft version, identified as @workspace/name@version with a file-name argument.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

func init() {
	for _, g := range []*cobra.Command{promptCmd, versionCmd, fileCmd} {
		g.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")
	}
	promptCmd.AddCommand(promptCreateCmd, promptUpdateCmd, promptListCmd, promptGetCmd)
}
