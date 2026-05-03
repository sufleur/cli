package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/WTomas/sufleur-cli/internal/resolver"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Resolve, fetch, and cache all configured prompts",
	RunE: func(cmd *cobra.Command, args []string) error {
		frozen, _ := cmd.Flags().GetBool("frozen")
		verbose, _ := cmd.Flags().GetBool("verbose")

		r := resolver.New(resolver.Options{
			ConfigPath:   "sufleur.yaml",
			LockfilePath: "sufleur-lock.yaml",
			CacheDir:     ".sufleur",
			Frozen:       frozen,
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

func init() {
	installCmd.Flags().Bool("frozen", false, "Fail if lockfile is out of date (CI mode)")
}
