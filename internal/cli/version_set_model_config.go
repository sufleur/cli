package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/userapi"
)

// modelConfigYAML is the shape written by `version dump` (model-config.yaml)
// and read back by `set-model-config --from-file`. Provider is the lowercase
// CLI token (e.g. "anthropic"), matching what --provider accepts.
type modelConfigYAML struct {
	Provider   string         `yaml:"provider"`
	Model      string         `yaml:"model"`
	Parameters map[string]any `yaml:"parameters"`
}

// modelConfigProviders is the CLI-facing token set for --provider. Tokens are
// lowercase; they are upper-cased into the LlmProvider GraphQL enum value
// (e.g. "anthropic" -> "ANTHROPIC") before being sent to the backend.
var modelConfigProviders = []string{
	"anthropic", "openai", "google", "mistral", "deepseek", "xai", "groq", "together",
}

var versionSetModelConfigCmd = &cobra.Command{
	Use:   "set-model-config @workspace/name@version --provider anthropic --model claude-sonnet-4-5 [--params '{...}']",
	Short: "Set a version's model config (provider, model, parameters)",
	Long: `Replaces the version's structured model config. Two modes:

Flag mode: pass --provider, --model, and optionally --params (a JSON object
of provider-specific parameters; defaults to {}).

File mode: pass --from-file PATH to a YAML file shaped like

  provider: anthropic
  model: claude-sonnet-4-5
  parameters:
    temperature: 0.7

— the same shape "version dump" writes to model-config.yaml. The provider
token is case-insensitive.

The two modes are mutually exclusive.`,
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

		fromFile, _ := cmd.Flags().GetString("from-file")
		fromFileSet := cmd.Flags().Changed("from-file")
		provider, _ := cmd.Flags().GetString("provider")
		model, _ := cmd.Flags().GetString("model")
		paramsRaw, _ := cmd.Flags().GetString("params")
		flagsSet := cmd.Flags().Changed("provider") || cmd.Flags().Changed("model") || cmd.Flags().Changed("params")

		modelConfig, err := resolveModelConfigInput(fromFile, fromFileSet, flagsSet, provider, model, paramsRaw)
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
	versionSetModelConfigCmd.Flags().String("from-file", "", "Path to a YAML file shaped like model-config.yaml (mutually exclusive with --provider/--model/--params)")
	versionSetModelConfigCmd.Flags().String("provider", "", "Model provider (one of: "+strings.Join(modelConfigProviders, ", ")+")")
	versionSetModelConfigCmd.Flags().String("model", "", "Model identifier (e.g. claude-sonnet-4-5)")
	versionSetModelConfigCmd.Flags().String("params", "{}", "JSON object of provider-specific parameters")
}

// resolveModelConfigInput enforces the mutual exclusivity between --from-file
// and the --provider/--model/--params flags, then delegates to whichever
// mode was selected.
func resolveModelConfigInput(fromFile string, fromFileSet, flagsSet bool, provider, model, paramsRaw string) (userapi.ModelConfig, error) {
	if fromFileSet && flagsSet {
		return userapi.ModelConfig{}, fmt.Errorf("--from-file is mutually exclusive with --provider/--model/--params")
	}
	if !fromFileSet && !flagsSet {
		return userapi.ModelConfig{}, fmt.Errorf("nothing to set: pass --provider/--model/--params or --from-file")
	}
	if fromFileSet {
		return parseModelConfigFile(fromFile)
	}
	return parseModelConfigFlags(provider, model, paramsRaw)
}

// parseModelConfigFile reads a YAML file of the shape written by
// `version dump` (model-config.yaml) into a userapi.ModelConfig, running it
// through the same validation as the --provider/--model/--params flags.
func parseModelConfigFile(path string) (userapi.ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return userapi.ModelConfig{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var doc modelConfigYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return userapi.ModelConfig{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	paramsJSON, err := json.Marshal(doc.Parameters)
	if err != nil {
		return userapi.ModelConfig{}, fmt.Errorf("re-encoding parameters from %s: %w", path, err)
	}
	return parseModelConfigFlags(doc.Provider, doc.Model, string(paramsJSON))
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
	// Normalize a null/omitted params object to an empty map so the wire value is
	// `{}` (the backend's parameters field is non-null), never `null`. This keeps
	// `--params null` and a --from-file YAML that omits `parameters:` consistent
	// with the `--params "{}"` default.
	if params == nil {
		params = map[string]any{}
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
