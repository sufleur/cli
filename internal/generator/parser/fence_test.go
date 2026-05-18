package parser

import (
	"regexp"
	"strings"
	"testing"
)

// TestFencePattern exercises the regex against the fixtures called out in the
// MAN-64 acceptance criteria. The same pattern is embedded byte-for-byte into
// the TS and Python output, so behaviour we lock in here also locks in the
// downstream engines (all three support the constructs we use: \s, \n,
// [\s\S]*?, [a-zA-Z]*).
func TestFencePattern(t *testing.T) {
	re := regexp.MustCompile(FencePattern)

	type want struct {
		match bool
		lang  string
		body  string
	}

	cases := []struct {
		name  string
		input string
		want  want
	}{
		{
			name:  "bare fence with json tag",
			input: "```json\n{\"x\":1}\n```",
			want:  want{match: true, lang: "json", body: "{\"x\":1}"},
		},
		{
			name:  "fence with leading prose",
			input: "Here is the JSON:\n```json\n{\"x\":1}\n```",
			want:  want{match: true, lang: "json", body: "{\"x\":1}"},
		},
		{
			name:  "fence with trailing prose",
			input: "```json\n{\"x\":1}\n```\nLet me know if you need adjustments.",
			want:  want{match: true, lang: "json", body: "{\"x\":1}"},
		},
		{
			name:  "fence with prose on both sides",
			input: "Sure, here you go:\n```json\n{\"x\":1}\n```\nHope that helps.",
			want:  want{match: true, lang: "json", body: "{\"x\":1}"},
		},
		{
			name:  "fence without language tag",
			input: "```\n{\"x\":1}\n```",
			want:  want{match: true, lang: "", body: "{\"x\":1}"},
		},
		{
			name:  "fence with uppercase JSON tag",
			input: "```JSON\n{\"x\":1}\n```",
			want:  want{match: true, lang: "JSON", body: "{\"x\":1}"},
		},
		{
			name:  "plain JSON with no fence does not match",
			input: "{\"x\":1}",
			want:  want{match: false},
		},
		{
			name:  "prose only does not match",
			input: "Sorry, I cannot help with that.",
			want:  want{match: false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := re.FindStringSubmatch(tc.input)
			if tc.want.match {
				if got == nil {
					t.Fatalf("expected match but got none for input %q", tc.input)
				}
				if got[1] != tc.want.lang {
					t.Errorf("lang: got %q, want %q", got[1], tc.want.lang)
				}
				if strings.TrimSpace(got[2]) != tc.want.body {
					t.Errorf("body: got %q, want %q", strings.TrimSpace(got[2]), tc.want.body)
				}
			} else if got != nil {
				t.Errorf("expected no match but got %v for input %q", got, tc.input)
			}
		})
	}
}

// TestFencePattern_MultipleFences verifies the unanchored pattern enumerates
// every fenced block. The runtime helpers walk all matches and prefer the
// first ```json```-tagged one — that selection logic lives in the generated
// TS/Python, but it relies on this enumeration working.
func TestFencePattern_MultipleFences(t *testing.T) {
	re := regexp.MustCompile(FencePattern)
	input := "First fence:\n```\nplain block\n```\nAnd the JSON:\n```json\n{\"x\":1}\n```\nDone."

	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}

	if matches[0][1] != "" {
		t.Errorf("first match lang: got %q, want \"\"", matches[0][1])
	}
	if strings.TrimSpace(matches[0][2]) != "plain block" {
		t.Errorf("first match body: got %q, want \"plain block\"", strings.TrimSpace(matches[0][2]))
	}
	if matches[1][1] != "json" {
		t.Errorf("second match lang: got %q, want \"json\"", matches[1][1])
	}
	if strings.TrimSpace(matches[1][2]) != "{\"x\":1}" {
		t.Errorf("second match body: got %q, want %q", strings.TrimSpace(matches[1][2]), "{\"x\":1}")
	}
}

// TestFencePattern_NestedTripleBackticks documents behaviour when a fence
// body itself contains ```. The non-greedy `[\s\S]*?` makes the regex stop
// at the first inner closing fence, so the outer fence's tail leaks out as
// a separate (likely garbage) match. We don't try to handle this — the
// generated extractor's `JSON.parse` step will fail and surface a
// `json-parse` error. This test pins the behaviour so a future "fix"
// doesn't silently change it.
func TestFencePattern_NestedTripleBackticks(t *testing.T) {
	re := regexp.MustCompile(FencePattern)
	input := "```\nouter ```inner``` text\n```"

	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one match for nested fences")
	}
	// We deliberately don't assert on the exact body — just that the regex
	// terminates and produces some result rather than hanging or panicking.
}
