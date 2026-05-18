// Package parser holds shared parser primitives that are embedded into the
// generated TypeScript and Python output.
//
// The constants here are emitted verbatim into both target languages, so the
// runtime regex behaviour is identical across all three engines (Go RE2 for
// the unit tests in this package, JavaScript RegExp for the TS output, and
// Python `re` for the Python output).
package parser

// FencePattern matches a markdown code fence anywhere within a string.
//
// Group 1: optional language tag (may be empty, e.g. "json", "JSON", "").
// Group 2: fence body (untrimmed).
//
// The pattern is deliberately unanchored so it tolerates leading or trailing
// prose around the fence — that resilience is the whole point of MAN-64. Use
// `FindAllStringSubmatch` / `findAll` / `re.finditer` to enumerate every
// fenced block in a response when picking the preferred match.
const FencePattern = "```([a-zA-Z]*)\\s*\\n?([\\s\\S]*?)\\n?\\s*```"
