package cli

import (
	"context"
	"strings"
	"testing"
)

func TestParsePromptVersionRef(t *testing.T) {
	cases := []struct {
		arg             string
		wantErrContains string
	}{
		{arg: "@acme/agent@draft"},
		{arg: "@acme/agent@1.0.0"},
		// Prompt names allow dots, unlike tool names.
		{arg: "@acme/my.agent@draft"},
		{arg: "@acme/agent", wantErrContains: "version is required"},
		{arg: "@acme/+pack@draft", wantErrContains: "collection"},
	}

	for _, c := range cases {
		t.Run(c.arg, func(t *testing.T) {
			_, err := parsePromptVersionRef(c.arg)
			if c.wantErrContains == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErrContains) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantErrContains)
			}
		})
	}
}

// Wire names and constraints are checked locally so a typo does not cost a
// round trip, and so the message can explain what the rule is for.
func TestVersionToolsAdd_ValidatesBeforeAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // guarantees "not logged in"
	versionToolsAddCmd.SetContext(context.Background())
	t.Cleanup(func() { _ = versionToolsAddCmd.Flags().Set("as", "") })

	t.Run("bad constraint", func(t *testing.T) {
		_ = versionToolsAddCmd.Flags().Set("as", "")
		err := versionToolsAddCmd.RunE(versionToolsAddCmd, []string{"@acme/agent@draft", "@vendor/web-search@not-a-range"})
		if err == nil || !strings.Contains(err.Error(), "invalid version constraint") {
			t.Errorf("expected the constraint to be rejected, got %v", err)
		}
	})

	t.Run("bad wire name", func(t *testing.T) {
		_ = versionToolsAddCmd.Flags().Set("as", "web.search")
		err := versionToolsAddCmd.RunE(versionToolsAddCmd, []string{"@acme/agent@draft", "@vendor/web-search@1.0.0"})
		if err == nil || !strings.Contains(err.Error(), "the model sees it verbatim") {
			t.Errorf("expected the wire name to be rejected, got %v", err)
		}
	})

	t.Run("tool name is checked too", func(t *testing.T) {
		_ = versionToolsAddCmd.Flags().Set("as", "")
		err := versionToolsAddCmd.RunE(versionToolsAddCmd, []string{"@acme/agent@draft", "@vendor/web.search@1.0.0"})
		if err == nil || !strings.Contains(err.Error(), "wire name") {
			t.Errorf("expected the tool name to be rejected, got %v", err)
		}
	})
}

// "draft" is a valid constraint for a tool pin: a draft prompt version may pin
// a draft tool, and publish is where that gets rejected.
func TestVersionToolsAdd_AllowsDraftConstraint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	versionToolsAddCmd.SetContext(context.Background())
	_ = versionToolsAddCmd.Flags().Set("as", "")

	err := versionToolsAddCmd.RunE(versionToolsAddCmd, []string{"@acme/agent@draft", "@vendor/web-search@draft"})
	if err == nil {
		t.Fatal("expected the command to reach the auth step")
	}
	// It got past validation and failed on credentials, which is what we want.
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("expected a credentials error, got %v", err)
	}
}

func TestVersionToolsRename_RequiresAs(t *testing.T) {
	versionToolsRenameCmd.SetContext(context.Background())
	_ = versionToolsRenameCmd.Flags().Set("as", "")

	err := versionToolsRenameCmd.RunE(versionToolsRenameCmd, []string{"@acme/agent@draft", "@vendor/web-search"})
	if err == nil || !strings.Contains(err.Error(), "--as is required") {
		t.Errorf("expected --as to be required, got %v", err)
	}
}
