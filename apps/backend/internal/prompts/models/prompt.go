package models

import "time"

// Prompt sources. A synced prompt is owned by Prompt Sync and read-only
// everywhere else; every other prompt, including built-ins, is manual.
const (
	PromptSourceManual = "manual"
	PromptSourceSync   = "sync"
)

type Prompt struct {
	ID      string
	Name    string
	Content string
	Builtin bool
	// Source is PromptSourceManual or PromptSourceSync.
	Source string
	// SourcePath is the repository file a synced prompt was applied from.
	SourcePath string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// IsSynced reports whether Prompt Sync owns the prompt.
func (p *Prompt) IsSynced() bool {
	return p != nil && p.Source == PromptSourceSync
}
