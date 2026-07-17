package userapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ValidateEvalYaml(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "parseEvalYaml(promptName: $promptName, version: $version, yaml: $yaml)") {
			t.Errorf("query = %q", req.Query)
		}
		if !strings.Contains(req.Query, "blocking") {
			t.Errorf("query missing blocking field: %q", req.Query)
		}
		// config is an object type on the wire; requesting it as a leaf 400s, so
		// the query must NOT select it.
		if strings.Contains(req.Query, "config") {
			t.Errorf("query must not select config (object type, would 400): %q", req.Query)
		}
		if req.Variables["yaml"] != "description: hi\n" {
			t.Errorf("yaml = %v", req.Variables["yaml"])
		}
		_, _ = w.Write([]byte(`{"data":{"parseEvalYaml":{` +
			`"errors":[` +
			`{"path":"prompt.model","line":3,"column":5,"message":"bad model","code":"MODEL","blocking":true},` +
			`{"path":"assertions[0]","message":"unknown var","code":"CEL","blocking":false}` +
			`],"warnings":[{"path":null,"message":"heads up","code":null,"blocking":null}]}}}`))
	}))
	defer server.Close()

	res, err := New(server.URL, "u_test", false).ValidateEvalYaml(context.Background(), "acme", "welcome", "draft", "description: hi\n")
	if err != nil {
		t.Fatalf("ValidateEvalYaml: %v", err)
	}
	if len(res.Errors) != 2 || len(res.Warnings) != 1 {
		t.Fatalf("got %d errors, %d warnings", len(res.Errors), len(res.Warnings))
	}
	if !res.Errors[0].IsBlocking() {
		t.Error("errors[0] should be blocking")
	}
	if res.Errors[1].IsBlocking() {
		t.Error("errors[1] should be non-blocking")
	}
	if res.Warnings[0].IsBlocking() {
		t.Error("warning should never be blocking")
	}
}

func TestClient_ApplyEvalYaml(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "applyEvalYaml(promptName: $promptName, version: $version, yaml: $yaml)") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"applyEvalYaml":{"id":"7f3a","description":"hi","promptVersionId":"pv1","datasetVersionId":"dv1","provider":"ANTHROPIC","model":"claude","passingThreshold":0.8,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	ev, err := New(server.URL, "u_test", false).ApplyEvalYaml(context.Background(), "acme", "welcome", "draft", "description: hi\n")
	if err != nil {
		t.Fatalf("ApplyEvalYaml: %v", err)
	}
	if ev.ID != "7f3a" || ev.Provider != "ANTHROPIC" {
		t.Errorf("got %+v", ev)
	}
	if ev.PassingThreshold == nil || *ev.PassingThreshold != 0.8 {
		t.Errorf("passingThreshold = %v", ev.PassingThreshold)
	}
}

func TestClient_GetEvalYaml(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"evalYaml":"description: hi\ndataset:\n  ref: null\n"}}`))
	}))
	defer server.Close()

	yaml, err := New(server.URL, "u_test", false).GetEvalYaml(context.Background(), "acme", "welcome", "draft")
	if err != nil {
		t.Fatalf("GetEvalYaml: %v", err)
	}
	if !strings.HasPrefix(yaml, "description: hi") {
		t.Errorf("yaml = %q", yaml)
	}
}

func TestClient_DeleteEval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "deleteEval") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"deleteEval":true}}`))
	}))
	defer server.Close()

	ok, err := New(server.URL, "u_test", false).DeleteEval(context.Background(), "acme", "welcome", "draft")
	if err != nil {
		t.Fatalf("DeleteEval: %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
}

func TestClient_GetEval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"eval":{"id":"e1","description":"","promptVersionId":"pv1","datasetVersionId":"dv1","provider":"OPENAI","model":"gpt-4o","passingThreshold":null,"createdAt":"2024-03-12T10:23:45Z","updatedAt":"2024-03-12T10:23:45Z"}}}`))
	}))
	defer server.Close()

	ev, err := New(server.URL, "u_test", false).GetEval(context.Background(), "acme", "welcome", "draft")
	if err != nil {
		t.Fatalf("GetEval: %v", err)
	}
	if ev == nil || ev.ID != "e1" || ev.DatasetVersionID != "dv1" {
		t.Errorf("got %+v", ev)
	}
	if ev.PassingThreshold != nil {
		t.Errorf("passingThreshold should be nil, got %v", *ev.PassingThreshold)
	}
}

func TestClient_GetEval_Null(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"eval":null}}`))
	}))
	defer server.Close()

	ev, err := New(server.URL, "u_test", false).GetEval(context.Background(), "acme", "welcome", "draft")
	if err != nil {
		t.Fatalf("GetEval (null): unexpected error %v", err)
	}
	if ev != nil {
		t.Errorf("expected nil eval, got %+v", ev)
	}
}

func TestClient_RunEval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "runEval(input: $input)") {
			t.Errorf("query = %q", req.Query)
		}
		input, ok := req.Variables["input"].(map[string]any)
		if !ok || input["evalId"] != "e1" {
			t.Errorf("input = %+v", req.Variables["input"])
		}
		_, _ = w.Write([]byte(`{"data":{"runEval":{"id":"r1","evalId":"e1","status":"QUEUED","verdict":null,"provider":"ANTHROPIC","model":"claude","totalScore":0,"passingThreshold":0.8,"processedCases":0,"errorMessage":null,"detailAvailable":false,"createdAt":"2024-03-12T10:23:45Z","startedAt":null,"finishedAt":null}}}`))
	}))
	defer server.Close()

	run, err := New(server.URL, "u_test", false).RunEval(context.Background(), "acme", "e1")
	if err != nil {
		t.Fatalf("RunEval: %v", err)
	}
	if run.ID != "r1" || run.Status != "QUEUED" {
		t.Errorf("got %+v", run)
	}
	if run.StartedAt != nil {
		t.Errorf("startedAt should be nil")
	}
}

func TestClient_GetEvalRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["id"] != "r1" {
			t.Errorf("id = %v", req.Variables["id"])
		}
		_, _ = w.Write([]byte(`{"data":{"evalRun":{"id":"r1","evalId":"e1","status":"SUCCEEDED","verdict":"PASSED","provider":"ANTHROPIC","model":"claude","totalScore":0.92,"passingThreshold":0.8,"processedCases":12,"errorMessage":null,"detailAvailable":true,"createdAt":"2024-03-12T10:23:45Z","startedAt":"2024-03-12T10:24:00Z","finishedAt":"2024-03-12T10:25:00Z"}}}`))
	}))
	defer server.Close()

	run, err := New(server.URL, "u_test", false).GetEvalRun(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetEvalRun: %v", err)
	}
	if run.Verdict != "PASSED" || run.TotalScore != 0.92 || run.ProcessedCases != 12 {
		t.Errorf("got %+v", run)
	}
	if run.StartedAt == nil || run.FinishedAt == nil {
		t.Errorf("timestamps should be set: %+v", run)
	}
}

func TestClient_GetEvalRunDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if req.Variables["id"] != "r1" {
			t.Errorf("id = %v", req.Variables["id"])
		}
		for _, want := range []string{"caseDetails", "renderedPrompts", "assertions", "judges", "renderedPrompts { fileName role renderedPrompt }"} {
			if !strings.Contains(req.Query, want) {
				t.Errorf("query missing %q: %q", want, req.Query)
			}
		}
		_, _ = w.Write([]byte(`{"data":{"evalRun":{` +
			`"id":"r1","evalId":"e1","status":"SUCCEEDED","verdict":"FAILED","provider":"ANTHROPIC","model":"claude",` +
			`"totalScore":0.5,"passingThreshold":0.8,"processedCases":2,"errorMessage":null,"detailAvailable":true,` +
			`"createdAt":"2024-03-12T10:23:45Z","startedAt":"2024-03-12T10:24:00Z","finishedAt":"2024-03-12T10:25:00Z",` +
			`"assertions":[` +
			`{"id":"a1","ordinal":0,"label":"contains foo","kind":"CEL","definition":"output.contains('foo')","status":"ACTIVE","result":"","errorMessage":""},` +
			`{"id":"a2","ordinal":1,"label":"is json","kind":"CEL","definition":"output.isJson()","status":"ACTIVE","result":"","errorMessage":""}` +
			`],` +
			`"judges":[{"id":"j1","alias":"quality","promptVersionId":"pv1","provider":"OPENAI","model":"gpt-4o"}],` +
			`"caseDetails":[` +
			`{"caseIndex":0,"passed":true,"resolvedInputs":{"q":"hi"},"outputRaw":"foo bar","outputParsed":{"answer":"foo bar"},` +
			`"assertions":[{"evalRunAssertionId":"a1","passed":true,"error":"","message":""}],` +
			`"judges":[{"evalRunJudgeId":"j1","input":{"q":"hi"},"output":{"verdict":"good"},"score":0.9,"error":"","renderedPrompts":[{"fileName":"judge.md","role":"user","renderedPrompt":"rate this"}]}],` +
			`"renderedPrompts":[{"fileName":"main.md","role":"user","renderedPrompt":"hi"}],` +
			`"providerError":""},` +
			`{"caseIndex":1,"passed":false,"resolvedInputs":{"q":"bye"},"outputRaw":"","outputParsed":null,` +
			`"assertions":[{"evalRunAssertionId":"a2","passed":false,"error":"timeout","message":"provider timed out"}],` +
			`"judges":[{"evalRunJudgeId":"j1","input":{"q":"bye"},"output":null,"score":null,"error":"provider timed out","renderedPrompts":[]}],` +
			`"renderedPrompts":[{"fileName":"main.md","role":"user","renderedPrompt":"bye"}],` +
			`"providerError":"provider timed out"}` +
			`]}}}`))
	}))
	defer server.Close()

	run, err := New(server.URL, "u_test", false).GetEvalRunDetail(context.Background(), "r1")
	if err != nil {
		t.Fatalf("GetEvalRunDetail: %v", err)
	}
	if run.ID != "r1" || run.Verdict != "FAILED" {
		t.Errorf("got run %+v", run.EvalRun)
	}
	if len(run.Assertions) != 2 || len(run.Judges) != 1 || len(run.Cases) != 2 {
		t.Fatalf("counts: assertions=%d judges=%d cases=%d", len(run.Assertions), len(run.Judges), len(run.Cases))
	}

	case0, case1 := run.Cases[0], run.Cases[1]
	if !case0.Passed {
		t.Errorf("case0.Passed = false, want true")
	}
	if case1.Passed {
		t.Errorf("case1.Passed = true, want false")
	}
	if case1.ProviderError != "provider timed out" {
		t.Errorf("case1.ProviderError = %q", case1.ProviderError)
	}
	if case0.OutputParsed == nil || string(case0.OutputParsed) == "null" {
		t.Errorf("case0.OutputParsed should be present, got %s", case0.OutputParsed)
	}
	if case1.OutputParsed != nil && string(case1.OutputParsed) != "null" {
		t.Errorf("case1.OutputParsed should be null, got %s", case1.OutputParsed)
	}
	if len(case0.Assertions) != 1 || case0.Assertions[0].EvalRunAssertionID != "a1" {
		t.Errorf("case0.Assertions = %+v", case0.Assertions)
	}
	if len(case1.Assertions) != 1 || case1.Assertions[0].EvalRunAssertionID != "a2" {
		t.Errorf("case1.Assertions = %+v", case1.Assertions)
	}
	if len(case0.Judges) != 1 || case0.Judges[0].Score == nil || *case0.Judges[0].Score != 0.9 {
		t.Errorf("case0.Judges = %+v", case0.Judges)
	}
	if len(case1.Judges) != 1 || case1.Judges[0].Score != nil {
		t.Errorf("case1.Judges[0].Score should be nil, got %+v", case1.Judges[0].Score)
	}
	if len(case0.RenderedPrompts) != 1 || case0.RenderedPrompts[0].FileName != "main.md" {
		t.Errorf("case0.RenderedPrompts = %+v", case0.RenderedPrompts)
	}
}

func TestClient_ListEvalRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req graphqlRequest
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "$pagination: PaginationArgs!") {
			t.Errorf("query = %q", req.Query)
		}
		_, _ = w.Write([]byte(`{"data":{"eval":{"runs":{"total":2,"data":[{"id":"r1","evalId":"e1","status":"SUCCEEDED","verdict":"PASSED","provider":"ANTHROPIC","model":"claude","totalScore":0.92,"passingThreshold":0.8,"processedCases":12,"errorMessage":null,"detailAvailable":true,"createdAt":"2024-03-12T10:23:45Z","startedAt":null,"finishedAt":null}]}}}}`))
	}))
	defer server.Close()

	page, err := New(server.URL, "u_test", false).ListEvalRuns(context.Background(), "acme", "welcome", "draft", 20, 0)
	if err != nil {
		t.Fatalf("ListEvalRuns: %v", err)
	}
	if page.Total != 2 || len(page.Data) != 1 {
		t.Errorf("got %+v", page)
	}
}

func TestClient_ListEvalRuns_NoEval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"eval":null}}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "u_test", false).ListEvalRuns(context.Background(), "acme", "welcome", "draft", 20, 0)
	if !errors.Is(err, ErrNoEval) {
		t.Fatalf("expected ErrNoEval, got %v", err)
	}
}
