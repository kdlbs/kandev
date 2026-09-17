// Package instructions owns the workspace coordinator's default behavior.
package instructions

import _ "embed"

//go:embed AGENTS.md
var Default string
