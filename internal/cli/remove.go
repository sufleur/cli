package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/WTomas/sufleur-cli/internal/cache"
	"github.com/WTomas/sufleur-cli/internal/config"
	"github.com/WTomas/sufleur-cli/internal/lockfile"
	"github.com/WTomas/sufleur-cli/internal/promptref"
)

var removeCmd = &cobra.Command{
	Use:     "remove @workspace/prompt",
	Aliases: []string{"rm"},
	Short:   "Remove a prompt from sufleur.yaml",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return err
		}

		if _, exists := cfg.Raw.Prompts[ref.Raw]; !exists {
			return fmt.Errorf("prompt %s not found in sufleur.yaml", ref.Raw)
		}

		// Remove from config
		delete(cfg.Raw.Prompts, ref.Raw)
		if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
			return err
		}

		// Remove from lockfile if it exists
		lf, err := lockfile.Load("sufleur-lock.yaml")
		if err == nil {
			delete(lf.Resolved, ref.Raw)
			if err := lockfile.Save("sufleur-lock.yaml", lf); err != nil {
				return err
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			// Ignore missing lockfile, but surface other errors
			return fmt.Errorf("loading lockfile: %w", err)
		}

		// Remove from cache (ignore not-found)
		c, err := cache.New(".sufleur")
		if err == nil {
			_ = c.Remove(promptref.CacheKey(ref))
		}

		fmt.Printf("Removed %s from sufleur.yaml\n", ref.Raw)
		return nil
	},
}
