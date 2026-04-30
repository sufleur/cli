package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultEndpoint is the production Sufleur API endpoint.
const DefaultEndpoint = "https://api.getmanifest.xyz/graphql"

// SufleurConfig represents the sufleur.yaml configuration file.
type SufleurConfig struct {
	APIKeys map[string]string `yaml:"api_keys"` // workspace name → API key (env var)
	Prompts map[string]string `yaml:"prompts"`  // "@workspace/prompt" → semver constraint
	Output  OutputConfig      `yaml:"output"`
}

// OutputConfig specifies code generation output settings.
type OutputConfig struct {
	Language string `yaml:"language"` // "typescript" | "python"
	File     string `yaml:"file"`     // output file path
}

// Config is the resolved runtime configuration.
type Config struct {
	Raw              SufleurConfig
	ResolvedKeys     map[string]string // workspace → resolved API key
	ResolvedEndpoint string
}

// Load reads a sufleur.yaml file and resolves environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var raw SufleurConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	resolvedKeys := make(map[string]string, len(raw.APIKeys))
	for workspace, keyRef := range raw.APIKeys {
		resolved, err := expandEnvVars(keyRef)
		if err != nil {
			return nil, fmt.Errorf("resolving API key for workspace %q: %w", workspace, err)
		}
		resolvedKeys[workspace] = resolved
	}

	endpoint := os.Getenv("SUFLEUR_ENDPOINT")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	return &Config{
		Raw:              raw,
		ResolvedKeys:     resolvedKeys,
		ResolvedEndpoint: endpoint,
	}, nil
}

// Save writes a SufleurConfig to the given path as YAML.
func Save(path string, cfg SufleurConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars replaces ${VAR_NAME} references with their environment values.
// Returns an error if a referenced variable is not set.
func expandEnvVars(s string) (string, error) {
	var expandErr error
	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		val, ok := os.LookupEnv(varName)
		if !ok {
			expandErr = fmt.Errorf("environment variable %s is not set", varName)
			return match
		}
		return val
	})
	if expandErr != nil {
		return "", expandErr
	}
	return result, nil
}
