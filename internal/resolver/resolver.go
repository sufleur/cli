package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/sufleur/cli/internal/cache"
	"github.com/sufleur/cli/internal/config"
	"github.com/sufleur/cli/internal/fetcher"
	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/integrity"
	"github.com/sufleur/cli/internal/lockfile"
	"github.com/sufleur/cli/internal/promptref"
)

// Options configures the resolver.
type Options struct {
	ConfigPath   string
	LockfilePath string
	CacheDir     string
	Frozen       bool
	Verbose      bool
	ForceAll     bool     // update all prompts (sufleur update with no args)
	UpdateOnly   []string // update specific prompts by alias key (sufleur update <alias>)
}

// ResolvedEntry describes a single resolved prompt in the result.
type ResolvedEntry struct {
	Alias      string // sufleur.yaml key
	Package    string // underlying ref (== Alias for non-aliased entries)
	Version    string
	Constraint string
	Status     string
	Fetched    bool
}

// Result is returned from Install with the resolved state.
type Result struct {
	Entries       []ResolvedEntry
	DraftWarnings []string
}

// ClientFactory creates a fetcher Client for a given workspace.
type ClientFactory func(workspace string) fetcher.Client

// Resolver orchestrates the install/update flow.
type Resolver struct {
	opts    Options
	factory ClientFactory
}

// New creates a resolver that builds its own fetcher clients from config.
func New(opts Options) *Resolver {
	return &Resolver{opts: opts}
}

// NewWithClient creates a resolver with an injected client factory (for testing).
func NewWithClient(opts Options, factory ClientFactory) *Resolver {
	return &Resolver{opts: opts, factory: factory}
}

// Install resolves all prompts according to config and writes the lockfile.
func (r *Resolver) Install(ctx context.Context) (*Result, error) {
	cfg, err := config.Load(r.opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	existing := lockfile.NewLockfile()
	if lf, err := lockfile.Load(r.opts.LockfilePath); err == nil {
		existing = lf
	}

	factory := r.factory
	if factory == nil {
		factory = func(workspace string) fetcher.Client {
			apiKey := cfg.ResolvedKeys[workspace]
			return fetcher.NewClient(cfg.ResolvedEndpoint, apiKey, workspace, r.opts.Verbose)
		}
	}

	dc, err := cache.New(r.opts.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("initializing cache: %w", err)
	}

	entries, err := cfg.Raw.PromptEntries()
	if err != nil {
		return nil, err
	}

	// Group by underlying package's workspace for batched validation.
	type entryWithRef struct {
		entry config.PromptEntry
		pkg   promptref.PromptRef
	}
	byWorkspace := make(map[string][]entryWithRef)
	anonymous := make(map[string]bool)
	for _, e := range entries {
		pkgRef, err := promptref.Parse(e.Package)
		if err != nil {
			return nil, fmt.Errorf("invalid package ref for prompt %q: %w", e.Alias, err)
		}
		// No api_keys entry means anonymous access — enough for public
		// prompts. A configured key that failed to resolve is an error.
		key, err := cfg.APIKeyFor(pkgRef.Workspace)
		if err != nil {
			return nil, fmt.Errorf("%w (needed by %s)", err, e.Alias)
		}
		anonymous[pkgRef.Workspace] = key == ""
		byWorkspace[pkgRef.Workspace] = append(byWorkspace[pkgRef.Workspace], entryWithRef{entry: e, pkg: pkgRef})
	}

	for workspace, ws := range byWorkspace {
		client := factory(workspace)
		// Dedupe package names — registry validation is name-based, not
		// (alias, name) based. Two aliases pointing to the same package
		// only need one validation call.
		seen := map[string]bool{}
		var names []string
		for _, e := range ws {
			if seen[e.pkg.Name] {
				continue
			}
			seen[e.pkg.Name] = true
			names = append(names, e.pkg.Name)
		}
		if err := client.ValidatePrompts(ctx, names); err != nil {
			return nil, fmt.Errorf("validating prompts for workspace %q: %w%s", workspace, err, anonymousHint(anonymous[workspace]))
		}
	}

	updateSet := make(map[string]bool)
	for _, name := range r.opts.UpdateOnly {
		updateSet[name] = true
	}

	result := &Result{}
	newLockfile := lockfile.NewLockfile()

	for workspace, ws := range byWorkspace {
		client := factory(workspace)
		for _, e := range ws {
			internal, err := r.resolveOne(ctx, client, dc, existing, e.entry, e.pkg, updateSet)
			if err != nil {
				return nil, fmt.Errorf("resolving %q: %w%s", e.entry.Alias, err, anonymousHint(anonymous[workspace]))
			}

			rp := lockfile.ResolvedPrompt{
				Version:      internal.Version,
				IntegritySHA: internal.integritySHA,
				Constraint:   e.entry.Constraint,
				Status:       internal.Status,
				ResolvedAt:   internal.resolvedAt,
			}
			if e.entry.IsAlias() {
				rp.Package = e.entry.Package
			}
			newLockfile.Resolved[e.entry.Alias] = rp

			result.Entries = append(result.Entries, ResolvedEntry{
				Alias:      e.entry.Alias,
				Package:    e.entry.Package,
				Version:    internal.Version,
				Constraint: e.entry.Constraint,
				Status:     internal.Status,
				Fetched:    internal.fetched,
			})

			if internal.Status == "DRAFT" {
				result.DraftWarnings = append(result.DraftWarnings,
					fmt.Sprintf("prompt %q resolved to draft version %s", e.entry.Alias, internal.Version))
			}
			// A pinned draft tool's contract is still mutable, so generated code
			// can go stale without the prompt's own version moving.
			for _, alias := range internal.draftTools {
				result.DraftWarnings = append(result.DraftWarnings,
					fmt.Sprintf("prompt %q pins draft tool %q", e.entry.Alias, alias))
			}
		}
	}

	if r.opts.Frozen {
		diffs := computeDiffs(existing, newLockfile)
		if len(diffs) > 0 {
			return nil, &FrozenError{Diffs: diffs}
		}
	}

	if err := lockfile.Save(r.opts.LockfilePath, newLockfile); err != nil {
		return nil, fmt.Errorf("saving lockfile: %w", err)
	}

	// Drop cache files no longer referenced by the lockfile. Keep set is the
	// (package, version) tuples implied by the new lockfile.
	keep := make(map[string]bool)
	for alias, rp := range newLockfile.Resolved {
		pkg := rp.Package
		if pkg == "" {
			pkg = alias
		}
		pkgRef, err := promptref.Parse(pkg)
		if err != nil {
			continue
		}
		keep[cache.Key(pkgRef.Raw, pkgRef.Name, rp.Version)] = true
	}
	if err := dc.PruneTo(keep); err != nil {
		return nil, fmt.Errorf("pruning cache: %w", err)
	}

	return result, nil
}

type resolvedPromptInternal struct {
	Version      string
	Status       string
	integritySHA string
	resolvedAt   time.Time
	fetched      bool
	// draftTools are the wire names of pins on unpublished tool versions. A
	// draft tool's contract can still change without the prompt's version
	// moving, so generated code can go stale under the installer.
	draftTools []string
}

func (r *Resolver) resolveOne(
	ctx context.Context,
	client fetcher.Client,
	dc *cache.Cache,
	existing *lockfile.Lockfile,
	entry config.PromptEntry,
	pkg promptref.PromptRef,
	updateSet map[string]bool,
) (*resolvedPromptInternal, error) {
	existingEntry, hasEntry := existing.Resolved[entry.Alias]

	// If the underlying package or constraint changed, force a fetch.
	existingPackage := existingEntry.Package
	if existingPackage == "" {
		existingPackage = entry.Alias
	}
	needsFetch := !hasEntry ||
		existingEntry.Constraint != entry.Constraint ||
		existingPackage != entry.Package ||
		r.opts.ForceAll ||
		updateSet[entry.Alias]

	var cachedDraftTools []string
	if !needsFetch {
		cacheKey := cache.Key(pkg.Raw, pkg.Name, existingEntry.Version)
		cached, err := dc.Load(cacheKey)
		if err != nil {
			needsFetch = true
		} else if integrity.Verify(cached, existingEntry.IntegritySHA) != nil {
			needsFetch = true
		} else {
			cachedDraftTools = generator.DraftToolAliases(*cached)
		}
	}

	if !needsFetch {
		return &resolvedPromptInternal{
			Version:      existingEntry.Version,
			Status:       existingEntry.Status,
			integritySHA: existingEntry.IntegritySHA,
			resolvedAt:   existingEntry.ResolvedAt,
			fetched:      false,
			draftTools:   cachedDraftTools,
		}, nil
	}

	// Fetch from API. The "draft" sentinel constraint requires status=DRAFT;
	// every other constraint resolves against published versions only.
	var status *fetcher.PromptVersionStatus
	if entry.Constraint == "draft" {
		s := fetcher.StatusDraft
		status = &s
	} else {
		s := fetcher.StatusPublished
		status = &s
	}

	pd, err := client.FetchPromptVersion(ctx, pkg.Name, entry.Constraint, status)
	if err != nil {
		return nil, err
	}

	// The cache stores the underlying package's identity, not the alias —
	// two aliases pointing at the same backing version share one file.
	pd.Ref = pkg.Raw
	pd.Name = pkg.Name

	sha := integrity.Compute(pd)

	if err := dc.Store(pd); err != nil {
		return nil, fmt.Errorf("caching prompt: %w", err)
	}

	return &resolvedPromptInternal{
		Version:      pd.Version,
		Status:       pd.Status,
		integritySHA: sha,
		resolvedAt:   time.Now().UTC(),
		fetched:      true,
		draftTools:   generator.DraftToolAliases(*pd),
	}, nil
}

func computeDiffs(old, new *lockfile.Lockfile) []PromptDiff {
	var diffs []PromptDiff

	for name, newEntry := range new.Resolved {
		oldEntry, exists := old.Resolved[name]
		if !exists {
			diffs = append(diffs, PromptDiff{
				Name:          name,
				NewVersion:    newEntry.Version,
				NewConstraint: newEntry.Constraint,
			})
		} else if oldEntry.Version != newEntry.Version || oldEntry.IntegritySHA != newEntry.IntegritySHA {
			diff := PromptDiff{
				Name:          name,
				OldVersion:    oldEntry.Version,
				NewVersion:    newEntry.Version,
				OldConstraint: oldEntry.Constraint,
				NewConstraint: newEntry.Constraint,
			}
			// Same version, different content: on a draft, the only things that
			// can move are the files and the tool pins.
			if oldEntry.Version == newEntry.Version {
				diff.Reason = "content changed"
			}
			diffs = append(diffs, diff)
		}
	}

	for name, oldEntry := range old.Resolved {
		if _, exists := new.Resolved[name]; !exists {
			diffs = append(diffs, PromptDiff{
				Name:          name,
				OldVersion:    oldEntry.Version,
				OldConstraint: oldEntry.Constraint,
			})
		}
	}

	return diffs
}
