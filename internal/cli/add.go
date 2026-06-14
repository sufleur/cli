package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/config"
	"github.com/sufleur/cli/internal/fetcher"
	"github.com/sufleur/cli/internal/promptref"
	"github.com/sufleur/cli/internal/resolver"
)

var addCmd = &cobra.Command{
	Use:   "add @workspace/{name|+collection} [constraint]",
	Short: "Add a prompt (or every prompt in a collection) to sufleur.yaml and install it",
	Long: `Add a prompt to sufleur.yaml and install it.

A "+"-prefixed reference is a collection: ` + "`sufleur add @workspace/+name`" + ` expands
the collection and adds every member prompt under its own @workspace/prompt key
(constraint "*"). Prompts already present in sufleur.yaml are left untouched and
reported as skipped; pass --force to reset them to "*".`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		aliasName, _ := cmd.Flags().GetString("alias")
		verbose, _ := cmd.Flags().GetBool("verbose")

		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		if ref.IsCollection {
			return runCollectionAdd(cmd, ref, args, force, verbose)
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

		return resolveAndReport(cmd.Context(), verbose)
	},
}

// runCollectionAdd expands a "@workspace/+collection" reference into its member
// prompts, writes each one into sufleur.yaml under its native @workspace/prompt
// key (constraint "*"), then resolves the whole config. Prompts already present
// are skipped (constraint preserved) unless --force is set.
func runCollectionAdd(cmd *cobra.Command, ref promptref.PromptRef, args []string, force, verbose bool) error {
	if len(args) == 2 {
		return fmt.Errorf("collections do not take a version constraint (got %q)", args[1])
	}
	if aliasName, _ := cmd.Flags().GetString("alias"); aliasName != "" {
		return fmt.Errorf("--alias is not supported when adding a collection")
	}

	cfg, err := config.Load("sufleur.yaml")
	if err != nil {
		return err
	}

	apiKey, ok := cfg.ResolvedKeys[ref.Workspace]
	if !ok {
		return fmt.Errorf("no API key configured for workspace %q — add it to api_keys in sufleur.yaml", ref.Workspace)
	}

	client := fetcher.NewClient(cfg.ResolvedEndpoint, apiKey, ref.Workspace, verbose)
	names, err := client.ListCollectionPrompts(cmd.Context(), ref.Name)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("Collection %s contains no prompts — nothing to install.\n", ref.Raw)
		return nil
	}

	if cfg.Raw.Prompts == nil {
		cfg.Raw.Prompts = make(map[string]string)
	}

	var added, skipped []string
	for _, name := range names {
		// Members live in the same workspace as the collection.
		key := "@" + ref.Workspace + "/" + name
		if _, exists := cfg.Raw.Prompts[key]; exists && !force {
			skipped = append(skipped, key)
			continue
		}
		cfg.Raw.Prompts[key] = "*"
		added = append(added, key)
	}

	if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
		return err
	}

	fmt.Printf("Collection %s — %d prompt(s): %d added, %d already present\n", ref.Raw, len(names), len(added), len(skipped))
	for _, k := range added {
		fmt.Printf("  + %s\n", k)
	}
	for _, k := range skipped {
		fmt.Printf("  = %s (already present, skipped)\n", k)
	}

	return resolveAndReport(cmd.Context(), verbose)
}

// resolveAndReport runs the resolver over sufleur.yaml and prints the per-prompt
// resolution result. Shared by `add` (both prompt and collection paths).
func resolveAndReport(ctx context.Context, verbose bool) error {
	r := resolver.New(resolver.Options{
		ConfigPath:   "sufleur.yaml",
		LockfilePath: "sufleur-lock.yaml",
		CacheDir:     ".sufleur",
		Verbose:      verbose,
	})

	result, err := r.Install(ctx)
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
