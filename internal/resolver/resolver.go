package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/WTomas/sufleur-cli/internal/cache"
	"github.com/WTomas/sufleur-cli/internal/config"
	"github.com/WTomas/sufleur-cli/internal/fetcher"
	"github.com/WTomas/sufleur-cli/internal/integrity"
	"github.com/WTomas/sufleur-cli/internal/lockfile"
	"github.com/WTomas/sufleur-cli/internal/promptref"
)

// Options configures the resolver.
type Options struct {
	ConfigPath   string
	LockfilePath string
	CacheDir     string
	Frozen       bool
	Draft        bool
	Verbose      bool
	ForceAll     bool     // update all prompts (sufleur update with no args)
	UpdateOnly   []string // update specific prompts (sufleur update <name>)
}

// ResolvedEntry describes a single resolved prompt in the result.
type ResolvedEntry struct {
	Name       string
	Version    string
	Constraint string
	Status     string
	Fetched    bool // true if fetched from API, false if reused from cache
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
	// 1. Load config
	cfg, err := config.Load(r.opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// 2. Load existing lockfile (or empty)
	existing := lockfile.NewLockfile()
	if lf, err := lockfile.Load(r.opts.LockfilePath); err == nil {
		existing = lf
	}

	// 3. Build client factory if not injected
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

	// 4. Parse all prompt refs and group by workspace
	type refWithConstraint struct {
		ref        promptref.PromptRef
		constraint string
	}
	byWorkspace := make(map[string][]refWithConstraint)

	for key, constraint := range cfg.Raw.Prompts {
		ref, err := promptref.Parse(key)
		if err != nil {
			return nil, fmt.Errorf("invalid prompt key: %w", err)
		}
		if _, ok := cfg.ResolvedKeys[ref.Workspace]; !ok {
			return nil, fmt.Errorf("no API key configured for workspace %q (needed by %s)", ref.Workspace, key)
		}
		byWorkspace[ref.Workspace] = append(byWorkspace[ref.Workspace], refWithConstraint{ref: ref, constraint: constraint})
	}

	// 5. Validate prompt names per workspace
	for workspace, refs := range byWorkspace {
		client := factory(workspace)
		names := make([]string, len(refs))
		for i, rc := range refs {
			names[i] = rc.ref.Name
		}
		if err := client.ValidatePrompts(ctx, names); err != nil {
			return nil, fmt.Errorf("validating prompts for workspace %q: %w", workspace, err)
		}
	}

	// 6. Resolve each prompt
	updateSet := make(map[string]bool)
	for _, name := range r.opts.UpdateOnly {
		updateSet[name] = true
	}

	result := &Result{}
	newLockfile := lockfile.NewLockfile()

	for workspace, refs := range byWorkspace {
		client := factory(workspace)
		for _, rc := range refs {
			entry, err := r.resolvePrompt(ctx, client, dc, existing, rc.ref, rc.constraint, updateSet)
			if err != nil {
				return nil, fmt.Errorf("resolving %q: %w", rc.ref.Raw, err)
			}

			newLockfile.Resolved[rc.ref.Raw] = lockfile.ResolvedPrompt{
				Version:      entry.Version,
				IntegritySHA: entry.integritySHA,
				Constraint:   rc.constraint,
				Status:       entry.Status,
				ResolvedAt:   entry.resolvedAt,
			}

			result.Entries = append(result.Entries, ResolvedEntry{
				Name:       rc.ref.Raw,
				Version:    entry.Version,
				Constraint: rc.constraint,
				Status:     entry.Status,
				Fetched:    entry.fetched,
			})

			if entry.Status == "DRAFT" {
				result.DraftWarnings = append(result.DraftWarnings,
					fmt.Sprintf("prompt %q resolved to draft version %s", rc.ref.Raw, entry.Version))
			}
		}
	}

	// 7. Frozen check
	if r.opts.Frozen {
		diffs := computeDiffs(existing, newLockfile)
		if len(diffs) > 0 {
			return nil, &FrozenError{Diffs: diffs}
		}
	}

	// 8. Write lockfile
	if err := lockfile.Save(r.opts.LockfilePath, newLockfile); err != nil {
		return nil, fmt.Errorf("saving lockfile: %w", err)
	}

	return result, nil
}

type resolvedPromptInternal struct {
	Version      string
	Status       string
	integritySHA string
	resolvedAt   time.Time
	fetched      bool
}

func (r *Resolver) resolvePrompt(
	ctx context.Context,
	client fetcher.Client,
	dc *cache.Cache,
	existing *lockfile.Lockfile,
	ref promptref.PromptRef,
	constraint string,
	updateSet map[string]bool,
) (*resolvedPromptInternal, error) {
	cacheKey := promptref.CacheKey(ref)
	existingEntry, hasEntry := existing.Resolved[ref.Raw]

	// Determine if we need to fetch from API
	needsFetch := !hasEntry ||
		existingEntry.Constraint != constraint ||
		r.opts.ForceAll ||
		updateSet[ref.Raw]

	// If we don't think we need a fetch, verify the cache is intact
	if !needsFetch {
		cached, err := dc.Load(cacheKey)
		if err != nil {
			needsFetch = true // cache miss
		} else if integrity.Verify(cached, existingEntry.IntegritySHA) != nil {
			needsFetch = true // cache corrupt
		}
	}

	if !needsFetch {
		return &resolvedPromptInternal{
			Version:      existingEntry.Version,
			Status:       existingEntry.Status,
			integritySHA: existingEntry.IntegritySHA,
			resolvedAt:   existingEntry.ResolvedAt,
			fetched:      false,
		}, nil
	}

	// Fetch from API
	var status *fetcher.PromptVersionStatus
	if !r.opts.Draft {
		s := fetcher.StatusPublished
		status = &s
	}

	pd, err := client.FetchPromptVersion(ctx, ref.Name, constraint, status)
	if err != nil {
		return nil, err
	}

	pd.Ref = ref.Raw

	sha := integrity.Compute(pd)

	if err := dc.Store(pd); err != nil {
		return nil, fmt.Errorf("caching prompt: %w", err)
	}

	resolvedStatus := pd.Status

	return &resolvedPromptInternal{
		Version:      pd.Version,
		Status:       resolvedStatus,
		integritySHA: sha,
		resolvedAt:   time.Now().UTC(),
		fetched:      true,
	}, nil
}

func computeDiffs(old, new *lockfile.Lockfile) []PromptDiff {
	var diffs []PromptDiff

	// Check for changes and additions
	for name, newEntry := range new.Resolved {
		oldEntry, exists := old.Resolved[name]
		if !exists {
			diffs = append(diffs, PromptDiff{
				Name:          name,
				NewVersion:    newEntry.Version,
				NewConstraint: newEntry.Constraint,
			})
		} else if oldEntry.Version != newEntry.Version || oldEntry.IntegritySHA != newEntry.IntegritySHA {
			diffs = append(diffs, PromptDiff{
				Name:          name,
				OldVersion:    oldEntry.Version,
				NewVersion:    newEntry.Version,
				OldConstraint: oldEntry.Constraint,
				NewConstraint: newEntry.Constraint,
			})
		}
	}

	// Check for removals
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
