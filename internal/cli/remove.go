package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/cache"
	"github.com/sufleur/cli/internal/config"
	"github.com/sufleur/cli/internal/lockfile"
	"github.com/sufleur/cli/internal/promptref"
)

var removeCmd = &cobra.Command{
	Use:     "remove @workspace/prompt",
	Aliases: []string{"rm"},
	Short:   "Remove a prompt from sufleur.yaml",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias, err := promptref.Parse(args[0])
		if err != nil {
			return err
		}

		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return err
		}

		if _, exists := cfg.Raw.Prompts[alias.Raw]; !exists {
			return fmt.Errorf("prompt %s not found in sufleur.yaml", alias.Raw)
		}

		// Remove from config.
		delete(cfg.Raw.Prompts, alias.Raw)
		if err := config.Save("sufleur.yaml", cfg.Raw); err != nil {
			return err
		}

		// Update lockfile and cache. The cache file is keyed by the
		// underlying package + version; only delete it if no other
		// alias still resolves to the same backing version.
		lf, err := lockfile.Load("sufleur-lock.yaml")
		switch {
		case err == nil:
			removed, ok := lf.Resolved[alias.Raw]
			if ok {
				delete(lf.Resolved, alias.Raw)
				if err := lockfile.Save("sufleur-lock.yaml", lf); err != nil {
					return err
				}
				removedPackage := removed.Package
				if removedPackage == "" {
					removedPackage = alias.Raw
				}
				stillReferenced := false
				for otherAlias, rp := range lf.Resolved {
					pkg := rp.Package
					if pkg == "" {
						// A non-aliased entry has no explicit package; its
						// backing package is its own alias key. Resolve it so
						// such entries are counted as referencing the package.
						pkg = otherAlias
					}
					if pkg == removedPackage && rp.Version == removed.Version {
						stillReferenced = true
						break
					}
				}
				if !stillReferenced {
					if c, err := cache.New(".sufleur"); err == nil {
						pkgRef, perr := promptref.Parse(removedPackage)
						if perr == nil {
							_ = c.Remove(cache.Key(pkgRef.Raw, pkgRef.Name, removed.Version))
						}
					}
				}
			}
		case errors.Is(err, fs.ErrNotExist):
			// nothing to clean up
		default:
			return fmt.Errorf("loading lockfile: %w", err)
		}

		fmt.Printf("Removed %s from sufleur.yaml\n", alias.Raw)
		return nil
	},
}
