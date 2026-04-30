package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/WTomas/sufleur-cli/internal/cache"
	"github.com/WTomas/sufleur-cli/internal/config"
	"github.com/WTomas/sufleur-cli/internal/generator"
	"github.com/WTomas/sufleur-cli/internal/lockfile"

	_ "github.com/WTomas/sufleur-cli/internal/generator/python"
	_ "github.com/WTomas/sufleur-cli/internal/generator/typescript"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Regenerate code from the current lockfile",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Load config
		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// 2. Load lockfile
		lf, err := lockfile.Load("sufleur-lock.yaml")
		if err != nil {
			return fmt.Errorf("loading lockfile: %w\nRun 'sufleur install' first.", err)
		}

		// 3. Load all cached prompts
		dc, err := cache.New(".sufleur")
		if err != nil {
			return fmt.Errorf("initializing cache: %w", err)
		}

		prompts, err := dc.LoadAll()
		if err != nil {
			return fmt.Errorf("loading cached prompts: %w", err)
		}

		// 4. Enrich prompts with status from lockfile
		for i := range prompts {
			key := prompts[i].Ref
			if key == "" {
				key = prompts[i].Name
			}
			if entry, ok := lf.Resolved[key]; ok {
				prompts[i].Status = entry.Status
			}
		}

		// 5. Look up generator by language
		lang := cfg.Raw.Output.Language
		gen, err := generator.Get(lang)
		if err != nil {
			return err
		}

		// 6. Generate
		outFile := cfg.Raw.Output.File
		if err := gen.Generate(outFile, prompts); err != nil {
			return fmt.Errorf("generating code: %w", err)
		}

		fmt.Printf("Generated %s code for %d prompt(s) → %s\n", lang, len(prompts), outFile)
		return nil
	},
}
