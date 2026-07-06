package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

// modelConfigProviders is the CLI-facing token set for --provider. Tokens are
// lowercase; they are upper-cased into the LlmProvider GraphQL enum value
// (e.g. "anthropic" -> "ANTHROPIC") before being sent to the backend.
var modelConfigProviders = []string{
	"anthropic", "openai", "google", "mistral", "deepseek", "xai", "groq", "together",
}

var versionSetModelConfigCmd = &cobra.Command{
	Use:           "set-model-config @workspace/name@version --provider anthropic --model claude-sonnet-4-5 [--params '{...}']",
	Short:         "Set a version's model config (provider, model, parameters)",
	Long:          "Replaces the version's structured model config. --params is a JSON object of provider-specific parameters (defaults to {}).",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.ParseRef(args[0])
		if err != nil {
			return err
		}
		if ref.Version == "" {
			return fmt.Errorf("version is required (use @workspace/name@version)")
		}

		provider, _ := cmd.Flags().GetString("provider")
		model, _ := cmd.Flags().GetString("model")
		paramsRaw, _ := cmd.Flags().GetString("params")

		modelConfig, err := parseModelConfigFlags(provider, model, paramsRaw)
		if err != nil {
			return err
		}

		client, _, err := loadUserAPIClient(cmd)
		if err != nil {
			return err
		}

		v, err := client.SetPromptVersionModelConfig(cmd.Context(), ref.Workspace, ref.Name, ref.Version, modelConfig)
		if err != nil {
			if errors.Is(err, userapi.ErrBearerRejected) {
				return fmt.Errorf("stored credentials are no longer valid — run `sufleur login` again")
			}
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return printJSON(cmd, v)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set model config on @%s/%s@%s\n", ref.Workspace, ref.Name, ref.Version)
		return nil
	},
}

func init() {
	versionSetModelConfigCmd.Flags().String("provider", "", "Model provider (one of: "+strings.Join(modelConfigProviders, ", ")+")")
	versionSetModelConfigCmd.Flags().String("model", "", "Model identifier (e.g. claude-sonnet-4-5)")
	versionSetModelConfigCmd.Flags().String("params", "{}", "JSON object of provider-specific parameters")
}

// parseModelConfigFlags validates and assembles the --provider/--model/--params
// flags into a userapi.ModelConfig ready to send over the wire.
func parseModelConfigFlags(provider, model, paramsRaw string) (userapi.ModelConfig, error) {
	if provider == "" {
		return userapi.ModelConfig{}, fmt.Errorf("--provider is required")
	}
	if !isValidModelConfigProvider(provider) {
		return userapi.ModelConfig{}, fmt.Errorf("invalid --provider %q: must be one of %s", provider, strings.Join(modelConfigProviders, ", "))
	}
	if model == "" {
		return userapi.ModelConfig{}, fmt.Errorf("--model is required")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(paramsRaw), &params); err != nil {
		return userapi.ModelConfig{}, fmt.Errorf("parsing --params as a JSON object: %w", err)
	}
	return userapi.ModelConfig{
		Provider:   strings.ToUpper(provider),
		Model:      model,
		Parameters: params,
	}, nil
}

func isValidModelConfigProvider(provider string) bool {
	for _, p := range modelConfigProviders {
		if strings.EqualFold(provider, p) {
			return true
		}
	}
	return false
}
