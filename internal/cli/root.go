package cli

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "sufleur",
	Short: "{sufleur} — type-safe codegen for versioned LLM prompts",
	Long:  "{sufleur} resolves, fetches, and generates type-safe code from versioned prompts in the prompt registry.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load .env file if present (ignore error if missing)
		_ = godotenv.Load()

		// By the time PersistentPreRunE runs, cobra has already parsed flags
		// and validated the argument count/shape for the matched command, so
		// any error returned from here on out is a runtime failure (network,
		// GraphQL, business-rule) rather than a usage mistake. Flipping
		// SilenceUsage here — instead of hardcoding it on every command —
		// keeps cobra's own usage dump for genuine mistakes (unknown flags,
		// wrong arg count), which are rejected before this hook runs.
		cmd.SilenceUsage = true
		return nil
	},
}

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("sufleur version {{.Version}}\n")

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Show detailed HTTP request/response logs")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(meCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(promptCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(fileCmd)
	rootCmd.AddCommand(datasetCmd)
	rootCmd.AddCommand(toolCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(collectionCmd)
	rootCmd.AddCommand(skillCmd)
}

// Execute runs the root command. When the matched command has SilenceErrors
// set (the new agent commands), we format the error ourselves so cobra's
// default print does not run; existing commands keep cobra's default output.
func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return
	}
	if cmd != nil && cmd.SilenceErrors {
		handleError(cmd, err)
	}
	os.Exit(1)
}
