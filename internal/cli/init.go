package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new sufleur.yaml configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat("sufleur.yaml"); err == nil {
			return fmt.Errorf("sufleur.yaml already exists")
		}

		scanner := bufio.NewScanner(os.Stdin)
		prompt := func(label, defaultVal string) string {
			if defaultVal != "" {
				fmt.Printf("%s (%s): ", label, defaultVal)
			} else {
				fmt.Printf("%s: ", label)
			}
			if scanner.Scan() {
				if val := strings.TrimSpace(scanner.Text()); val != "" {
					return val
				}
			}
			return defaultVal
		}

		workspace := prompt("Workspace name", "")
		envVar := prompt("API key environment variable", strings.ToUpper(strings.ReplaceAll(workspace, "-", "_"))+"_API_KEY")
		language := prompt("Output language (typescript, python)", "typescript")

		defaultFile := "./generated/prompts.ts"
		if language == "python" {
			defaultFile = "./generated/prompts.py"
		}
		outFile := prompt("Output file path", defaultFile)

		cfg := config.SufleurConfig{
			APIKeys: map[string]string{
				workspace: "${" + envVar + "}",
			},
			Prompts: map[string]string{},
			Output: config.OutputConfig{
				Language: language,
				File:     outFile,
			},
		}

		if err := config.Save("sufleur.yaml", cfg); err != nil {
			return err
		}

		fmt.Println("\nCreated sufleur.yaml")
		fmt.Printf("Next steps:\n")
		fmt.Printf("  1. Set %s in your environment (or .env file)\n", envVar)
		fmt.Printf("  2. Run: sufleur add @%s/<prompt-name>\n", workspace)
		fmt.Printf("  3. Run: sufleur install\n")
		fmt.Printf("  4. Run: sufleur generate\n")
		return nil
	},
}
