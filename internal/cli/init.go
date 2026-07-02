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

		workspace, envVar := "", ""
		usePrivate := prompt("Will you use private prompts from a workspace? Public prompts need no API key (y/N)", "")
		if isYes(usePrivate) {
			workspace = prompt("Workspace name", "")
			if workspace != "" {
				envVar = prompt("API key environment variable", strings.ToUpper(strings.ReplaceAll(workspace, "-", "_"))+"_API_KEY")
			}
		}
		language := prompt("Output language (typescript, python)", "typescript")

		defaultFile := "./generated/prompts.ts"
		if language == "python" {
			defaultFile = "./generated/prompts.py"
		}
		outFile := prompt("Output file path", defaultFile)

		if err := config.Save("sufleur.yaml", buildInitConfig(workspace, envVar, language, outFile)); err != nil {
			return err
		}

		fmt.Println("\nCreated sufleur.yaml")
		fmt.Printf("Next steps:\n")
		if workspace != "" {
			fmt.Printf("  1. Set %s in your environment (or .env file)\n", envVar)
			fmt.Printf("  2. Run: sufleur add @%s/<prompt-name>\n", workspace)
			fmt.Printf("  3. Run: sufleur install\n")
			fmt.Printf("  4. Run: sufleur generate\n")
		} else {
			fmt.Printf("  1. Run: sufleur add @<workspace>/<prompt-name>   (public prompts need no API key)\n")
			fmt.Printf("  2. Run: sufleur install\n")
			fmt.Printf("  3. Run: sufleur generate\n")
		}
		return nil
	},
}

// isYes reports whether an interactive answer affirms a yes/no question.
// Anything other than y/yes (case-insensitive) counts as no.
func isYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// buildInitConfig assembles the sufleur.yaml written by `sufleur init`. An
// empty workspace means the user only consumes public prompts: no api_keys
// entry is written at all.
func buildInitConfig(workspace, envVar, language, outFile string) config.SufleurConfig {
	apiKeys := map[string]string{}
	if workspace != "" {
		apiKeys[workspace] = "${" + envVar + "}"
	}
	return config.SufleurConfig{
		APIKeys: apiKeys,
		Prompts: map[string]string{},
		Output: config.OutputConfig{
			Language: language,
			File:     outFile,
		},
	}
}
