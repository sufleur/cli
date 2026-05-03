package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/WTomas/sufleur-cli/internal/promptref"
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

// PromptEntry is one parsed entry from the `prompts:` map. Package == Alias
// for non-aliased entries. For aliased entries, Alias is the YAML key the user
// invokes from generated code, while Package is the underlying registry
// reference (workspace + name) the resolver fetches.
type PromptEntry struct {
	Alias      string // sufleur.yaml key, e.g. "@wtomas/old-foo"
	Package    string // underlying ref, e.g. "@wtomas/foo"
	Constraint string // semver constraint or "draft"
}

// IsAlias reports whether the entry points at a different package than its key.
func (e PromptEntry) IsAlias() bool {
	return e.Package != e.Alias
}

// PromptEntries returns the parsed list of (alias, package, constraint)
// triples in deterministic order (alias key sorted alphabetically). A value
// that begins with "@" and contains another "@" is treated as an alias spec
// of the form "<package_ref>@<constraint>"; anything else is a plain
// constraint applied to the key itself.
func (c SufleurConfig) PromptEntries() ([]PromptEntry, error) {
	keys := make([]string, 0, len(c.Prompts))
	for k := range c.Prompts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]PromptEntry, 0, len(keys))
	for _, k := range keys {
		entry, err := ParsePromptEntry(k, c.Prompts[k])
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// ParsePromptEntry interprets one (key, value) pair from the prompts map.
// Exposed so the CLI can validate user-supplied input before writing YAML.
func ParsePromptEntry(key, value string) (PromptEntry, error) {
	if value == "" {
		return PromptEntry{}, fmt.Errorf("prompt %q has empty value", key)
	}
	// Plain constraint: doesn't start with "@".
	if !strings.HasPrefix(value, "@") {
		return PromptEntry{Alias: key, Package: key, Constraint: value}, nil
	}
	// Alias spec: "@workspace/name@constraint". Split on the rightmost "@"
	// so the leading workspace "@" is preserved.
	at := strings.LastIndex(value, "@")
	if at <= 0 {
		return PromptEntry{}, fmt.Errorf("prompt %q has malformed alias spec %q", key, value)
	}
	pkg := value[:at]
	constraint := value[at+1:]
	if pkg == "" || constraint == "" {
		return PromptEntry{}, fmt.Errorf("prompt %q alias spec %q must be \"@workspace/name@constraint\"", key, value)
	}
	if _, err := promptref.Parse(pkg); err != nil {
		return PromptEntry{}, fmt.Errorf("prompt %q alias package %q invalid: %w", key, pkg, err)
	}
	return PromptEntry{Alias: key, Package: pkg, Constraint: constraint}, nil
}

// FormatPromptValue produces the YAML value for a (package, constraint) pair,
// taking aliasing into account. For non-aliased entries (alias == package)
// the value is the bare constraint; for aliased entries it's
// "<package>@<constraint>".
func FormatPromptValue(alias, pkg, constraint string) string {
	if alias == pkg {
		return constraint
	}
	return pkg + "@" + constraint
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
