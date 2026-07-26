package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- fix 2: `update <name>` on a name not in sufleur.yaml must fail loudly ---

func TestUpdate_UnknownName_ReturnsError(t *testing.T) {
	writeSufleurYAML(t, map[string]string{
		"@ws/foo": "^1.0.0",
	})

	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()
	t.Setenv("SUFLEUR_ENDPOINT", ts.URL)

	updateCmd.SetContext(context.Background())

	err := updateCmd.RunE(updateCmd, []string{"@ws/bar"})
	if err == nil {
		t.Fatal("expected an error for a name not in sufleur.yaml, got nil")
	}

	// Mirrors remove's message style: "prompt %s not found in sufleur.yaml".
	want := "prompt @ws/bar not found in sufleur.yaml"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if called {
		t.Error("expected no network call for an unknown prompt name")
	}
}

func TestUpdate_KnownName_Succeeds(t *testing.T) {
	writeSufleurYAML(t, map[string]string{
		"@ws/foo": "^1.0.0",
		"@ws/bar": "^2.0.0",
	})

	reg := newFakeRegistry(t)
	reg.versions["foo"] = "1.2.0"
	reg.versions["bar"] = "2.0.3"
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	updateCmd.SetContext(context.Background())

	var runErr error
	out := captureStdout(t, func() {
		runErr = updateCmd.RunE(updateCmd, []string{"@ws/foo"})
	})
	if runErr != nil {
		t.Fatalf("expected success, got: %v", runErr)
	}
	if !strings.Contains(out, "Updated 2 prompt(s).") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestUpdate_NoArgs_UpdatesAll(t *testing.T) {
	writeSufleurYAML(t, map[string]string{
		"@ws/foo": "^1.0.0",
	})

	reg := newFakeRegistry(t)
	reg.versions["foo"] = "1.2.0"
	t.Setenv("SUFLEUR_ENDPOINT", reg.start())

	updateCmd.SetContext(context.Background())

	if err := updateCmd.RunE(updateCmd, []string{}); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}
