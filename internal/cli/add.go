package cli

import (
	"fmt"

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
		verbose, _ := cmd.Flags().GetBool("verbose")

		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		constraint := "*"
		if len(args) == 2 {
			constraint = args[1]
		}

		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return err
		}

		apiKey, ok := cfg.ResolvedKeys[ref.Workspace]
		if !ok {
			return fmt.Errorf("no API key configured for workspace %q — add it to api_keys in sufleur.yaml", ref.Workspace)
		}

		if existing, exists := cfg.Raw.Prompts[ref.Raw]; exists && !force {
			return fmt.Errorf("prompt %s already in sufleur.yaml (constraint: %s) — use --force to update", ref.Raw, existing)
		}

		// Validate the prompt exists in the API
		client := fetcher.NewClient(cfg.ResolvedEndpoint, apiKey, ref.Workspace, verbose)
		if err := client.ValidatePrompts(cmd.Context(), []string{ref.Name}); err != nil {
			return fmt.Errorf("validating prompt: %w", err)
		}

		// Add/update the prompt in config
		if cfg.Raw.Prompts == nil {
			cfg.Raw.Prompts = make(map[string]string)
		}
		cfg.Raw.Prompts[ref.Raw] = constraint
		if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
			return err
		}

		fmt.Printf("Added %s (%s) to sufleur.yaml\n", ref.Raw, constraint)

		// Run install to fetch and cache
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
			fmt.Printf("  %s@%s (%s)\n", e.Name, e.Version, status)
		}

		for _, w := range result.DraftWarnings {
			fmt.Printf("  warning: %s\n", w)
		}

		fmt.Printf("\nResolved %d prompt(s).\n", len(result.Entries))
		return nil
	},
}

func init() {
	addCmd.Flags().Bool("force", false, "Update constraint if prompt already exists")
}
