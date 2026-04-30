package cli

import (
	"fmt"
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
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
