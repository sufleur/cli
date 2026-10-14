package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sufleur/cli/internal/credentials"
)

const disabledCode = "CLI_PUBLISHING_DISABLED"

const disabledMessage = `CLI publishing is disabled for workspace @acme. A workspace owner or admin must turn on "Allow CLI and agent publishing" in the web app (Settings → Publish rules). This setting cannot be changed from the CLI.`

type publishCase struct {
	name     string
	args     []string
	response string
}

var publishCases = []publishCase{
	{
		name:     "prompt",
		args:     []string{"version", "publish", "@acme/agent", "1.2.0"},
		response: `{"data":{"publishPromptVersion":{"version":"1.2.0","status":"PUBLISHED"}}}`,
	},
	{
		name:     "tool",
		args:     []string{"tool", "version", "publish", "@acme/web-search", "1.2.0"},
		response: `{"data":{"publishToolVersion":{"version":"1.2.0","status":"PUBLISHED"}}}`,
	},
	{
		name:     "dataset",
		args:     []string{"dataset", "version", "publish", "@acme/cases", "1.2.0"},
		response: `{"data":{"publishDatasetVersion":{"version":"1.2.0","status":"PUBLISHED"}}}`,
	},
}

func withArgs(c publishCase, extra ...string) []string {
	return append(append([]string{}, c.args...), extra...)
}

func resetJSONFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, c := range []interface{ Set(string, string) error }{
			versionCmd.PersistentFlags(), toolCmd.PersistentFlags(), datasetCmd.PersistentFlags(),
		} {
			_ = c.Set("json", "false")
		}
	})
}

func publishServer(t *testing.T, body string) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := credentials.Save(credentials.Credentials{APIBase: server.URL, APIKey: "u_test"}); err != nil {
		t.Fatalf("saving credentials: %v", err)
	}
	resetJSONFlags(t)
	return server, &calls
}

func TestPublish_InvalidVersionRejectedBeforeAnyRequest(t *testing.T) {
	for _, c := range publishCases {
		for _, bad := range []string{"1.2", "v1.2.0", "1.2.0-rc.1", "1.2.0+build", "draft"} {
			t.Run(c.name+"/"+bad, func(t *testing.T) {
				_, calls := publishServer(t, c.response)
				args := append(append([]string{}, c.args[:len(c.args)-1]...), bad)
				_, err := executeRoot(t, args)
				if err == nil || !strings.Contains(err.Error(), "x.y.z") {
					t.Fatalf("expected a semantic-version error, got %v", err)
				}
				if *calls != 0 {
					t.Errorf("expected no request, got %d", *calls)
				}
			})
		}
	}
}

func TestPublish_RequiresTwoArguments(t *testing.T) {
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := executeRoot(t, c.args[:len(c.args)-1])
			if err == nil {
				t.Fatal("expected an error for a missing version argument")
			}
		})
	}
}

func TestPublish_HumanOutput(t *testing.T) {
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			publishServer(t, c.response)
			var err error
			out := captureStdout(t, func() { _, err = executeRoot(t, c.args) })
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			ref := c.args[len(c.args)-2]
			if strings.TrimSpace(out) != "Published "+ref+"@1.2.0" {
				t.Errorf("stdout = %q", out)
			}
		})
	}
}

func TestPublish_JSONOutputIsVersionObject(t *testing.T) {
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			publishServer(t, c.response)
			var err error
			out := captureStdout(t, func() { _, err = executeRoot(t, withArgs(c, "--json")) })
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("stdout is not JSON: %q", out)
			}
			if got["version"] != "1.2.0" || got["status"] != "PUBLISHED" {
				t.Errorf("stdout = %v", got)
			}
		})
	}
}

func TestPublish_DisabledSettingYieldsExactMessage(t *testing.T) {
	body := `{"data":null,"errors":[{"message":"backend wording","extensions":{"code":"` + disabledCode + `"}}]}`
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			publishServer(t, body)
			stderr, err := executeRoot(t, c.args)
			if err == nil || err.Error() != disabledMessage {
				t.Fatalf("err = %v", err)
			}
			if strings.TrimSpace(stderr) != "Error: "+disabledMessage {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestPublish_DisabledSettingJSONErrorIncludesCode(t *testing.T) {
	body := `{"data":null,"errors":[{"message":"backend wording","extensions":{"code":"` + disabledCode + `"}}]}`
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			publishServer(t, body)
			stderr, _ := executeRoot(t, withArgs(c, "--json"))
			if strings.Count(strings.TrimSpace(stderr), "\n") != 0 {
				t.Fatalf("expected one line, got %q", stderr)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr is not JSON: %q", stderr)
			}
			if got["error"] != disabledMessage || got["code"] != disabledCode || len(got) != 2 {
				t.Errorf("stderr = %v", got)
			}
		})
	}
}

func TestPublish_OrdinaryErrorPassesThroughWithoutCode(t *testing.T) {
	body := `{"data":null,"errors":[{"message":"version must be greater than 1.2.0"}]}`
	for _, c := range publishCases {
		t.Run(c.name, func(t *testing.T) {
			publishServer(t, body)
			stderr, err := executeRoot(t, withArgs(c, "--json"))
			if err == nil || err.Error() != "graphql error: version must be greater than 1.2.0" {
				t.Fatalf("err = %v", err)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr is not JSON: %q", stderr)
			}
			if _, has := got["code"]; has || len(got) != 1 {
				t.Errorf("stderr = %v, want only the error key", got)
			}
		})
	}
}
