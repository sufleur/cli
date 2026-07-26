package cli

import (
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// captureStderr mirrors captureStdout (fake_registry_test.go) but for stderr,
// which is where cobra's own error/usage machinery writes.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

// executeRoot runs the real rootCmd.ExecuteC() pipeline (not the package-level
// Execute(), which os.Exit(1)s on error) and captures whatever cobra/handleError
// write to stderr. It mirrors Execute()'s own post-ExecuteC logic (call
// handleError iff the matched command has SilenceErrors set) so that new-era
// commands — which rely on Execute() to print their error, since cobra itself
// stays silent for them — get their "Error:" line in tests too, exactly as
// they would in the real binary. rootCmd and its subcommands are package-level
// singletons, so any command-local state ExecuteC mutates (here, SilenceUsage
// flipped by the root PersistentPreRunE) is reset once the test completes to
// avoid leaking into other tests.
func executeRoot(t *testing.T, args []string) (stderr string, err error) {
	t.Helper()
	rootCmd.SetArgs(args)
	var cmd *cobra.Command
	stderr = captureStderr(t, func() {
		cmd, err = rootCmd.ExecuteC()
		if err != nil && cmd != nil && cmd.SilenceErrors {
			handleError(cmd, err)
		}
	})
	if cmd != nil {
		t.Cleanup(func() { cmd.SilenceUsage = false })
	}
	return stderr, err
}

// --- fix 1: cobra usage dumps suppressed on runtime errors, kept for genuine
// usage mistakes (unknown flags / bad arg counts) ---

func TestExecute_GenuineUsageMistake_ShowsUsage(t *testing.T) {
	stderr, err := executeRoot(t, []string{"add", "@ws/foo", "1.0.0", "extra-arg"})
	if err == nil {
		t.Fatal("expected an error for too many args")
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage to be shown for a genuine arg-count mistake, got: %q", stderr)
	}
}

func TestExecute_UnknownFlag_ShowsUsage(t *testing.T) {
	stderr, err := executeRoot(t, []string{"add", "--bogus-flag", "@ws/foo"})
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage to be shown for an unknown flag, got: %q", stderr)
	}
}

func TestExecute_RuntimeError_NoUsageDump(t *testing.T) {
	writeSufleurYAML(t, nil)

	ts := httptest.NewServer(nil)
	url := ts.URL
	ts.Close() // closed immediately: connection refused is a runtime failure, not a usage mistake

	t.Setenv("SUFLEUR_ENDPOINT", url)

	stderr, err := executeRoot(t, []string{"add", "@ws/foo"})
	if err == nil {
		t.Fatal("expected a runtime error")
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("expected the error message to still print, got: %q", stderr)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Errorf("a runtime error should not dump command usage, got: %q", stderr)
	}
}

// --- fix 1 (new-era commands): the same two invariants, but on a command that
// hardcodes SilenceErrors: true (and, before this fix, also hardcoded
// SilenceUsage: true). These commands route their runtime-error printing
// through handleError instead of cobra's own printer (see Execute in
// root.go), so it's worth checking the single-print property holds for them
// too, not just for legacy commands like `add`. ---

func TestExecute_NewEraCommand_UnknownFlag_ShowsUsageAndError(t *testing.T) {
	stderr, err := executeRoot(t, []string{"eval", "get", "--bogus-flag", "@ws/foo@1.0.0"})
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage to be shown for an unknown flag on a new-era command, got: %q", stderr)
	}
	if got := strings.Count(stderr, "Error:"); got != 1 {
		t.Errorf("expected exactly one \"Error:\" line (cobra suppresses its own print via SilenceErrors, "+
			"handleError prints once), got %d in: %q", got, stderr)
	}
}

func TestExecute_NewEraCommand_RuntimeError_NoUsageDump(t *testing.T) {
	// Point XDG_CONFIG_HOME at an empty temp dir so credentials.Load() finds
	// no credentials.yaml and loadUserAPIClient returns the "not logged in"
	// runtime error deterministically, without a network round-trip.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stderr, err := executeRoot(t, []string{"eval", "get", "@ws/foo@1.0.0"})
	if err == nil {
		t.Fatal("expected a runtime error (not logged in)")
	}
	if got := strings.Count(stderr, "Error:"); got != 1 {
		t.Errorf("expected exactly one \"Error:\" line, got %d in: %q", got, stderr)
	}
	if !strings.Contains(stderr, "not logged in") {
		t.Errorf("expected the not-logged-in message, got: %q", stderr)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Errorf("a runtime error should not dump command usage on a new-era command, got: %q", stderr)
	}
}
