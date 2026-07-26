package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	semver "github.com/Masterminds/semver/v3"
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

		// Pre-validate the constraint locally before any network round-trip.
		// The backend rejects a malformed constraint too, but only after a
		// GraphQL call, and with a raw error dump.
		if err := validateConstraint(constraint); err != nil {
			return err
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

		client, anonymous, err := clientForWorkspace(cfg, ref.Workspace, verbose)
		if err != nil {
			return err
		}

		if existing, exists := cfg.Raw.Prompts[aliasKey]; exists && !force {
			return fmt.Errorf("prompt %s already in sufleur.yaml (value: %s) — use --force to update", aliasKey, existing)
		}

		if err := client.ValidatePrompts(cmd.Context(), []string{ref.Name}); err != nil {
			return fmt.Errorf("validating prompt: %w%s", err, anonymousHint(anonymous))
		}

		// Capture the manifest before mutating it so a failed resolve can roll
		// it back, never leaving a half-written sufleur.yaml behind.
		original, err := os.ReadFile("sufleur.yaml")
		if err != nil {
			return err
		}

		if cfg.Raw.Prompts == nil {
			cfg.Raw.Prompts = make(map[string]string)
		}
		cfg.Raw.Prompts[aliasKey] = config.FormatPromptValue(aliasKey, ref.Raw, constraint)
		if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
			return err
		}

		if aliasKey == ref.Raw {
			fmt.Printf("Resolving %s (%s)...\n", aliasKey, constraint)
		} else {
			fmt.Printf("Resolving %s (alias for %s @ %s)...\n", aliasKey, ref.Raw, constraint)
		}

		if err := resolveOrRollback(cmd.Context(), verbose, original); err != nil {
			return err
		}

		// Only reported once resolution has actually succeeded — printing
		// this beforehand contradicted the "left unchanged" rollback message
		// on failure.
		if aliasKey == ref.Raw {
			fmt.Printf("Added %s (%s) to sufleur.yaml\n", aliasKey, constraint)
		} else {
			fmt.Printf("Added %s (alias for %s @ %s) to sufleur.yaml\n", aliasKey, ref.Raw, constraint)
		}

		return nil
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

	client, anonymous, err := clientForWorkspace(cfg, ref.Workspace, verbose)
	if err != nil {
		return err
	}

	names, err := client.ListCollectionPrompts(cmd.Context(), ref.Name)
	if err != nil {
		return fmt.Errorf("%w%s", err, anonymousHint(anonymous))
	}
	if len(names) == 0 {
		fmt.Printf("Collection %s contains no prompts — nothing to install.\n", ref.Raw)
		return nil
	}

	// Capture the manifest before mutating it so a failed resolve can roll it
	// back. Adding a collection is all-or-nothing: if any member fails to
	// resolve (e.g. it has no published version), sufleur.yaml is restored
	// rather than left holding unresolvable entries.
	original, err := os.ReadFile("sufleur.yaml")
	if err != nil {
		return err
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

	if err := resolveOrRollback(cmd.Context(), verbose, original); err != nil {
		return improveCollectionResolveError(err, ref.Workspace, names)
	}
	return nil
}

// improveCollectionResolveError rewrites a resolver failure caused by a
// collection member having no published version into an honest, actionable
// message. members comes from ListCollectionPrompts, which only returns
// prompt names the backend has already confirmed exist — so if
// FetchPromptVersion later reports no version matching the constraint for
// one of them, that can only mean the prompt has no published version (it
// may be draft-only), never that the prompt doesn't exist. Errors unrelated
// to this case, or for names outside this collection, pass through
// unchanged.
func improveCollectionResolveError(err error, workspace string, members []string) error {
	var noVersion *fetcher.NoPublishedVersionError
	if !errors.As(err, &noVersion) {
		return err
	}
	for _, m := range members {
		if m == noVersion.PromptName {
			return fmt.Errorf(
				"@%s/%s has no published version (draft-only) — publish it or remove it from the collection\nsufleur.yaml was left unchanged",
				workspace, m)
		}
	}
	return err
}

// clientForWorkspace builds a fetcher client for a workspace. A workspace
// without an api_keys entry gets an anonymous client (public prompts only);
// a configured key that failed to resolve is an error.
func clientForWorkspace(cfg *config.Config, workspace string, verbose bool) (fetcher.Client, bool, error) {
	apiKey, err := cfg.APIKeyFor(workspace)
	if err != nil {
		return nil, false, err
	}
	return fetcher.NewClient(cfg.ResolvedEndpoint, apiKey, workspace, verbose), apiKey == "", nil
}

// anonymousHint appends the resolver's anonymous-access hint to error output
// when the failed request ran without an API key.
func anonymousHint(anonymous bool) string {
	if anonymous {
		return resolver.AnonymousAccessHint
	}
	return ""
}

// resolveOrRollback runs the resolver over sufleur.yaml and, if resolution
// fails, restores the manifest to the bytes captured before it was modified.
// This keeps `add` atomic: a failed resolve never leaves sufleur.yaml holding
// entries that cannot be installed. The lockfile is written by the resolver
// only after every prompt resolves, so it needs no rollback here.
func resolveOrRollback(ctx context.Context, verbose bool, original []byte) error {
	if err := resolveAndReport(ctx, verbose); err != nil {
		if rbErr := os.WriteFile("sufleur.yaml", original, 0644); rbErr != nil {
			return fmt.Errorf("%w\n(additionally, restoring sufleur.yaml failed: %v)", err, rbErr)
		}
		return fmt.Errorf("%w\nsufleur.yaml was left unchanged", err)
	}
	return nil
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

// validateConstraint checks a user-supplied version constraint locally,
// before any network round-trip: constraint matching itself still happens
// server-side, but a malformed constraint (typo'd operator, garbage string)
// can be rejected immediately with a friendly message instead of a raw
// GraphQL error dump. "draft" is the CLI's own sentinel for "the latest
// draft version" — not a semver constraint — so it is exempt.
func validateConstraint(constraint string) error {
	if constraint == "draft" {
		return nil
	}
	if _, err := semver.NewConstraint(constraint); err != nil {
		return fmt.Errorf("invalid version constraint %q: %s", constraint, err)
	}
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
