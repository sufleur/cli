package cli

import "github.com/spf13/cobra"

// workspaceCmd is the parent of `sufleur workspace <action>`. Subcommands are
// user-scoped (they act on the workspaces the logged-in user belongs to) rather
// than taking an explicit @workspace reference.
var workspaceCmd = &cobra.Command{
	Use:           "workspace",
	Short:         "Manage the workspaces you belong to",
	Long:          "Subcommands operate on the workspaces the authenticated user is a member of.",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.NoArgs,
	Run:           func(cmd *cobra.Command, _ []string) { _ = cmd.Help() },
}

func init() {
	workspaceCmd.PersistentFlags().Bool("json", false, "Output a single JSON value on stdout instead of human-readable text")
	workspaceCmd.AddCommand(workspaceListCmd)
}
