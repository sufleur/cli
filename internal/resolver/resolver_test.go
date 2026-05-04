package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sufleur/cli/internal/cache"
	"github.com/sufleur/cli/internal/config"
	"github.com/sufleur/cli/internal/fetcher"
	"github.com/sufleur/cli/internal/generator"
	"github.com/sufleur/cli/internal/integrity"
	"github.com/sufleur/cli/internal/lockfile"
)

// mockClient implements fetcher.Client for testing.
type mockClient struct {
	validateErr error
	prompts     map[string]*generator.PromptData
	fetchCalls  int
	// Records the last status passed for each prompt name (nil means unset).
	lastStatus map[string]*fetcher.PromptVersionStatus
}

func (m *mockClient) ValidatePrompts(_ context.Context, _ []string) error {
	return m.validateErr
}

func (m *mockClient) FetchPromptVersion(_ context.Context, promptName, _ string, status *fetcher.PromptVersionStatus) (*generator.PromptData, error) {
	m.fetchCalls++
	if m.lastStatus == nil {
		m.lastStatus = map[string]*fetcher.PromptVersionStatus{}
	}
	m.lastStatus[promptName] = status
	pd, ok := m.prompts[promptName]
	if !ok {
		return nil, errors.New("prompt not found")
	}
	return pd, nil
}

func writeTestConfig(t *testing.T, dir string, prompts map[string]string) string {
	t.Helper()
	cfgPath := filepath.Join(dir, "sufleur.yaml")
	cfg := config.SufleurConfig{
		APIKeys: map[string]string{
			"test": "test-key",
		},
		Prompts: prompts,
		Output:  config.OutputConfig{Language: "typescript", File: "./generated/prompts.ts"},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	return cfgPath
}

func makePromptData(name, version string) *generator.PromptData {
	return &generator.PromptData{
		Name:        name,
		Version:     version,
		Description: "Test " + name,
		Status:      "PUBLISHED",
		Files: []generator.PromptFile{
			{Name: "main.txt", Content: "Hello from " + name},
		},
	}
}

// storeInCache writes prompt data as JSON into the cache directory using the
// cache package's filename convention.
func storeInCache(t *testing.T, cacheDir string, pd *generator.PromptData) {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}
	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		t.Fatalf("marshaling prompt data: %v", err)
	}
	key := cache.Key(pd.Ref, pd.Name, pd.Version)
	if err := os.WriteFile(filepath.Join(cacheDir, key+".json"), data, 0644); err != nil {
		t.Fatalf("writing cache file: %v", err)
	}
}

func mockFactory(mock *mockClient) ClientFactory {
	return func(workspace string) fetcher.Client {
		return mock
	}
}

func TestFreshInstall(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
		"@test/farewell": "~2.0.0",
	})

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": makePromptData("greeting", "1.2.0"),
			"farewell": makePromptData("farewell", "2.0.3"),
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: filepath.Join(dir, "sufleur-lock.yaml"),
		CacheDir:     filepath.Join(dir, ".sufleur"),
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	if mock.fetchCalls != 2 {
		t.Errorf("expected 2 fetch calls, got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if !e.Fetched {
			t.Errorf("expected %s to be fetched", e.Alias)
		}
	}

	// Verify lockfile was written with full ref keys
	lf, err := lockfile.Load(filepath.Join(dir, "sufleur-lock.yaml"))
	if err != nil {
		t.Fatalf("loading lockfile: %v", err)
	}
	if len(lf.Resolved) != 2 {
		t.Fatalf("lockfile has %d entries, want 2", len(lf.Resolved))
	}
	if _, ok := lf.Resolved["@test/greeting"]; !ok {
		t.Error("lockfile missing @test/greeting key")
	}
	if _, ok := lf.Resolved["@test/farewell"]; !ok {
		t.Error("lockfile missing @test/farewell key")
	}
}

func TestInstall_ValidCache_SkipsFetch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
	})

	pd := makePromptData("greeting", "1.2.0")
	pd.Ref = "@test/greeting"
	sha := integrity.Compute(pd)

	// Pre-populate lockfile with full ref key
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: sha,
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	// Pre-populate valid cache with sanitized ref key
	cacheDir := filepath.Join(dir, ".sufleur")
	storeInCache(t, cacheDir, pd)

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": pd,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     cacheDir,
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if mock.fetchCalls != 0 {
		t.Errorf("expected 0 fetch calls (cache hit), got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if e.Fetched {
			t.Errorf("expected %s to not be fetched (cache hit)", e.Alias)
		}
	}
}

func TestInstall_CorruptedCache_Refetches(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
	})

	pd := makePromptData("greeting", "1.2.0")
	sha := integrity.Compute(pd)

	// Pre-populate lockfile
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: sha,
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	// Write corrupted cache with sanitized ref key
	cacheDir := filepath.Join(dir, ".sufleur")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "@test__greeting.json"), []byte(`{"name":"greeting","version":"WRONG"}`), 0644)

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": pd,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     cacheDir,
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if mock.fetchCalls != 1 {
		t.Errorf("expected 1 fetch call (cache corrupt), got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if !e.Fetched {
			t.Errorf("expected %s to be fetched (cache was corrupt)", e.Alias)
		}
	}
}

func TestInstall_ChangedConstraint_Refetches(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^2.0.0", // changed from ^1.0.0
	})

	pd := makePromptData("greeting", "2.1.0")

	// Pre-populate lockfile with OLD constraint
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: "sha256-old",
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": pd,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     filepath.Join(dir, ".sufleur"),
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if mock.fetchCalls != 1 {
		t.Errorf("expected 1 fetch call (constraint changed), got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if e.Version != "2.1.0" {
			t.Errorf("expected version 2.1.0, got %s", e.Version)
		}
	}
}

func TestFrozen_Pass(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
	})

	pd := makePromptData("greeting", "1.2.0")
	pd.Ref = "@test/greeting"
	sha := integrity.Compute(pd)

	// Pre-populate matching lockfile
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: sha,
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	// Pre-populate valid cache
	cacheDir := filepath.Join(dir, ".sufleur")
	storeInCache(t, cacheDir, pd)

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": pd,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     cacheDir,
		Frozen:       true,
	}, mockFactory(mock))

	_, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("expected no error in frozen mode with matching lockfile, got: %v", err)
	}
}

func TestFrozen_Fail(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
	})

	pd := makePromptData("greeting", "1.3.0")

	// Lockfile has old version
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: "sha256-old",
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": pd,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     filepath.Join(dir, ".sufleur"),
		Frozen:       true,
	}, mockFactory(mock))

	_, err := r.Install(context.Background())
	if err == nil {
		t.Fatal("expected FrozenError, got nil")
	}

	var fe *FrozenError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FrozenError, got %T: %v", err, err)
	}
	if len(fe.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(fe.Diffs))
	}
}

func TestDraftConstraint(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "draft",
		"@test/farewell": "^2.0.0",
	})

	draftPD := makePromptData("greeting", "draft")
	draftPD.Status = "DRAFT"
	farewellPD := makePromptData("farewell", "2.0.3")

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": draftPD,
			"farewell": farewellPD,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: filepath.Join(dir, "sufleur-lock.yaml"),
		CacheDir:     filepath.Join(dir, ".sufleur"),
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Draft constraint surfaces a warning for that prompt only.
	if len(result.DraftWarnings) != 1 {
		t.Errorf("expected 1 draft warning, got %d", len(result.DraftWarnings))
	}

	// The mock received status=DRAFT for the draft-constrained prompt
	// and status=PUBLISHED for the semver-constrained one.
	greetingStatus := mock.lastStatus["greeting"]
	if greetingStatus == nil || *greetingStatus != fetcher.StatusDraft {
		t.Errorf("expected greeting status=DRAFT, got %v", greetingStatus)
	}
	farewellStatus := mock.lastStatus["farewell"]
	if farewellStatus == nil || *farewellStatus != fetcher.StatusPublished {
		t.Errorf("expected farewell status=PUBLISHED, got %v", farewellStatus)
	}

	// Lockfile records the literal "draft" version for the draft-constrained prompt.
	for _, e := range result.Entries {
		if e.Alias == "@test/greeting" {
			if e.Status != "DRAFT" {
				t.Errorf("expected greeting status=DRAFT in result, got %s", e.Status)
			}
			if e.Version != "draft" {
				t.Errorf("expected greeting version=draft, got %s", e.Version)
			}
		}
	}
}

func TestUpdate_SinglePrompt(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
		"@test/farewell": "~2.0.0",
	})

	greetingPD := makePromptData("greeting", "1.2.0")
	greetingPD.Ref = "@test/greeting"
	farewellPD := makePromptData("farewell", "2.0.3")
	farewellPD.Ref = "@test/farewell"

	greetingSHA := integrity.Compute(greetingPD)
	farewellSHA := integrity.Compute(farewellPD)

	// Pre-populate lockfile
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: greetingSHA,
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lf.Resolved["@test/farewell"] = lockfile.ResolvedPrompt{
		Version:      "2.0.3",
		IntegritySHA: farewellSHA,
		Constraint:   "~2.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	// Pre-populate cache for both using sanitized ref keys
	cacheDir := filepath.Join(dir, ".sufleur")
	storeInCache(t, cacheDir, greetingPD)
	storeInCache(t, cacheDir, farewellPD)

	// API returns newer greeting
	updatedGreeting := makePromptData("greeting", "1.5.0")

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": updatedGreeting,
			"farewell": farewellPD,
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     cacheDir,
		UpdateOnly:   []string{"@test/greeting"},
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Only greeting should be fetched
	if mock.fetchCalls != 1 {
		t.Errorf("expected 1 fetch call (update single), got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if e.Alias == "@test/greeting" {
			if !e.Fetched {
				t.Error("expected greeting to be fetched")
			}
			if e.Version != "1.5.0" {
				t.Errorf("greeting version = %s, want 1.5.0", e.Version)
			}
		}
		if e.Alias == "@test/farewell" && e.Fetched {
			t.Error("expected farewell to not be fetched")
		}
	}
}

func TestUpdate_AllPrompts(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTestConfig(t, dir, map[string]string{
		"@test/greeting": "^1.0.0",
		"@test/farewell": "~2.0.0",
	})

	greetingPD := makePromptData("greeting", "1.2.0")
	greetingPD.Ref = "@test/greeting"
	farewellPD := makePromptData("farewell", "2.0.3")
	farewellPD.Ref = "@test/farewell"

	greetingSHA := integrity.Compute(greetingPD)
	farewellSHA := integrity.Compute(farewellPD)

	// Pre-populate lockfile
	lf := lockfile.NewLockfile()
	lf.Resolved["@test/greeting"] = lockfile.ResolvedPrompt{
		Version:      "1.2.0",
		IntegritySHA: greetingSHA,
		Constraint:   "^1.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lf.Resolved["@test/farewell"] = lockfile.ResolvedPrompt{
		Version:      "2.0.3",
		IntegritySHA: farewellSHA,
		Constraint:   "~2.0.0",
		Status:       "PUBLISHED",
		ResolvedAt:   time.Now().UTC(),
	}
	lockPath := filepath.Join(dir, "sufleur-lock.yaml")
	if err := lockfile.Save(lockPath, lf); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	// Pre-populate cache
	cacheDir := filepath.Join(dir, ".sufleur")
	storeInCache(t, cacheDir, greetingPD)
	storeInCache(t, cacheDir, farewellPD)

	mock := &mockClient{
		prompts: map[string]*generator.PromptData{
			"greeting": makePromptData("greeting", "1.5.0"),
			"farewell": makePromptData("farewell", "2.1.0"),
		},
	}

	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: lockPath,
		CacheDir:     cacheDir,
		ForceAll:     true,
	}, mockFactory(mock))

	result, err := r.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if mock.fetchCalls != 2 {
		t.Errorf("expected 2 fetch calls (update all), got %d", mock.fetchCalls)
	}

	for _, e := range result.Entries {
		if !e.Fetched {
			t.Errorf("expected %s to be fetched (update all)", e.Alias)
		}
	}
}

func TestMissingWorkspaceKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sufleur.yaml")
	cfg := config.SufleurConfig{
		APIKeys: map[string]string{
			"test": "test-key",
		},
		Prompts: map[string]string{
			"@other/greeting": "^1.0.0",
		},
		Output: config.OutputConfig{Language: "typescript", File: "./generated/prompts.ts"},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	mock := &mockClient{}
	r := NewWithClient(Options{
		ConfigPath:   cfgPath,
		LockfilePath: filepath.Join(dir, "sufleur-lock.yaml"),
		CacheDir:     filepath.Join(dir, ".sufleur"),
	}, mockFactory(mock))

	_, err := r.Install(context.Background())
	if err == nil {
		t.Fatal("expected error for missing workspace key, got nil")
	}
	if !contains(err.Error(), "no API key configured for workspace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
