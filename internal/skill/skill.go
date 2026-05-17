// Package skill embeds the Sufleur agent skill markdown so the binary can
// print it via `sufleur skill`. Co-locating the content with the binary keeps
// the skill in lockstep with the command surface.
package skill

import _ "embed"

//go:embed sufleur.md
var Markdown string
