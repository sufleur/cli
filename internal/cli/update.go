package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/config"
	"github.com/sufleur/cli/internal/resolver"
)

var updateCmd = &cobra.Command{
	Use:   "update [prompt]",
	Short: "Re-resolve and update one or all prompts",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")

		opts := resolver.Options{
			ConfigPath:   "sufleur.yaml",
			LockfilePath: "sufleur-lock.yaml",
			CacheDir:     ".sufleur",
			Verbose:      verbose,
		}

		if len(args) == 1 {
			// Fail loudly on a name that isn't in sufleur.yaml — mirrors
			// `remove`'s check — rather than silently falling through to a
			// full re-resolve of every prompt.
			cfg, err := config.Load(opts.ConfigPath)
			if err != nil {
				return err
			}
			if _, exists := cfg.Raw.Prompts[args[0]]; !exists {
				return fmt.Errorf("prompt %s not found in sufleur.yaml", args[0])
			}
			opts.UpdateOnly = []string{args[0]}
		} else {
			opts.ForceAll = true
		}

		r := resolver.New(opts)

		result, err := r.Install(cmd.Context())
		if err != nil {
			return err
		}

		for _, e := range result.Entries {
			status := "unchanged"
			if e.Fetched {
				status = "updated"
			}
			fmt.Printf("  %s@%s (%s)\n", e.Alias, e.Version, status)
		}

		fmt.Printf("\nUpdated %d prompt(s).\n", len(result.Entries))
		return nil
	},
}
