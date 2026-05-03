package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/WTomas/sufleur-cli/internal/cache"
	"github.com/WTomas/sufleur-cli/internal/config"
	"github.com/WTomas/sufleur-cli/internal/generator"
	"github.com/WTomas/sufleur-cli/internal/lockfile"
	"github.com/WTomas/sufleur-cli/internal/promptref"

	_ "github.com/WTomas/sufleur-cli/internal/generator/python"
	_ "github.com/WTomas/sufleur-cli/internal/generator/typescript"
)

// printSchemaWarnings emits one stderr line per inference warning across all
// entrypoints. Called from generate; install also surfaces draft warnings,
// but schema warnings only matter at codegen time.
func printSchemaWarnings(prompts []generator.PromptData) {
	for _, p := range prompts {
		ref := p.Ref
		if ref == "" {
			ref = p.Name
		}
		for _, f := range p.Files {
			for _, w := range f.SchemaWarnings {
				if w.Path != "" {
					fmt.Fprintf(os.Stderr, "[sufleur] %s:%s — warning: %s (path: %s)\n", ref, f.Name, w.Message, w.Path)
				} else {
					fmt.Fprintf(os.Stderr, "[sufleur] %s:%s — warning: %s\n", ref, f.Name, w.Message)
				}
			}
		}
	}
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Regenerate code from the current lockfile",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("sufleur.yaml")
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		lf, err := lockfile.Load("sufleur-lock.yaml")
		if err != nil {
			return fmt.Errorf("loading lockfile: %w\nRun 'sufleur install' first.", err)
		}

		dc, err := cache.New(".sufleur")
		if err != nil {
			return fmt.Errorf("initializing cache: %w", err)
		}

		entries, err := cfg.Raw.PromptEntries()
		if err != nil {
			return err
		}

		// For each (alias, package, constraint) entry, load the cache file
		// keyed by the underlying package + resolved version, then rewrite
		// Ref/Name on the in-memory PromptData so the generator emits
		// alias-named identifiers.
		prompts := make([]generator.PromptData, 0, len(entries))
		for _, e := range entries {
			rp, ok := lf.Resolved[e.Alias]
			if !ok {
				return fmt.Errorf("lockfile missing entry for %q — run 'sufleur install'", e.Alias)
			}
			pkgRef, err := promptref.Parse(e.Package)
			if err != nil {
				return fmt.Errorf("parsing package %q for alias %q: %w", e.Package, e.Alias, err)
			}
			pd, err := dc.Load(cache.Key(pkgRef.Raw, pkgRef.Name, rp.Version))
			if err != nil {
				return fmt.Errorf("cache miss for %q: %w (run 'sufleur install')", e.Alias, err)
			}
			aliasRef, err := promptref.Parse(e.Alias)
			if err != nil {
				return fmt.Errorf("parsing alias %q: %w", e.Alias, err)
			}
			pd.Ref = aliasRef.Raw
			pd.Name = aliasRef.Name
			pd.Status = rp.Status
			prompts = append(prompts, *pd)
		}

		lang := cfg.Raw.Output.Language
		gen, err := generator.Get(lang)
		if err != nil {
			return err
		}

		printSchemaWarnings(prompts)

		outFile := cfg.Raw.Output.File
		if err := gen.Generate(outFile, prompts); err != nil {
			return fmt.Errorf("generating code: %w", err)
		}

		fmt.Printf("Generated %s code for %d prompt(s) → %s\n", lang, len(prompts), outFile)
		return nil
	},
}
