package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/WTomas/sufleur-cli/internal/config"
	"github.com/WTomas/sufleur-cli/internal/fetcher"
	"github.com/WTomas/sufleur-cli/internal/promptref"
	"github.com/WTomas/sufleur-cli/internal/resolver"
)

var addCmd = &cobra.Command{
	Use:   "add @workspace/prompt [constraint]",
	Short: "Add a prompt to sufleur.yaml and install it",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		aliasName, _ := cmd.Flags().GetString("alias")
		verbose, _ := cmd.Flags().GetBool("verbose")

		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		constraint := "*"
		if len(args) == 2 {
			constraint = args[1]
		}

		// Compute the alias key. With --alias, the new entry lives under
		// @<workspace>/<aliasName> in the same workspace as the underlying
		// package; without --alias, the key equals the package ref.
		aliasKey := ref.Raw
		if aliasName != "" {
			if err := validateAliasName(aliasName); err != nil {
				return err
			}
			aliasKey = "@" + ref.Workspace + "/" + aliasName
		}

		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return err
		}

		apiKey, ok := cfg.ResolvedKeys[ref.Workspace]
		if !ok {
			return fmt.Errorf("no API key configured for workspace %q — add it to api_keys in sufleur.yaml", ref.Workspace)
		}

		if existing, exists := cfg.Raw.Prompts[aliasKey]; exists && !force {
			return fmt.Errorf("prompt %s already in sufleur.yaml (value: %s) — use --force to update", aliasKey, existing)
		}

		client := fetcher.NewClient(cfg.ResolvedEndpoint, apiKey, ref.Workspace, verbose)
		if err := client.ValidatePrompts(cmd.Context(), []string{ref.Name}); err != nil {
			return fmt.Errorf("validating prompt: %w", err)
		}

		if cfg.Raw.Prompts == nil {
			cfg.Raw.Prompts = make(map[string]string)
		}
		cfg.Raw.Prompts[aliasKey] = config.FormatPromptValue(aliasKey, ref.Raw, constraint)
		if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
			return err
		}

		if aliasKey == ref.Raw {
			fmt.Printf("Added %s (%s) to sufleur.yaml\n", aliasKey, constraint)
		} else {
			fmt.Printf("Added %s (alias for %s @ %s) to sufleur.yaml\n", aliasKey, ref.Raw, constraint)
		}

		r := resolver.New(resolver.Options{
			ConfigPath:   "sufleur.yaml",
			LockfilePath: "sufleur-lock.yaml",
			CacheDir:     ".sufleur",
			Verbose:      verbose,
		})

		result, err := r.Install(cmd.Context())
		if err != nil {
			return err
		}

		for _, e := range result.Entries {
			status := "cached"
			if e.Fetched {
				status = "fetched"
			}
			fmt.Printf("  %s@%s (%s)\n", e.Alias, e.Version, status)
		}

		for _, w := range result.DraftWarnings {
			fmt.Printf("  warning: %s\n", w)
		}

		fmt.Printf("\nResolved %d prompt(s).\n", len(result.Entries))
		return nil
	},
}

// validateAliasName enforces that a `--alias` value is a bare prompt name
// suitable for the right-hand side of @workspace/<name> (no slashes, no @).
func validateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("--alias must not be empty")
	}
	if strings.ContainsAny(name, "@/ ") {
		return fmt.Errorf("--alias %q must not contain '@', '/', or whitespace", name)
	}
	return nil
}

func init() {
	addCmd.Flags().Bool("force", false, "Update value if prompt already exists")
	addCmd.Flags().String("alias", "", "Install the prompt under a different name in the same workspace (lets you keep multiple versions side-by-side)")
}
