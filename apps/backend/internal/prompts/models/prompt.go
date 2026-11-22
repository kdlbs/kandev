package models

import "time"

// Prompt sources. A synced prompt is owned by Prompt Sync and read-only
// everywhere else. An override is a local, editable copy of a synced prompt
// that sync leaves alone. Every other prompt, including built-ins, is manual.
const (
	PromptSourceManual   = "manual"
	PromptSourceSync     = "sync"
	PromptSourceOverride = "override"
)

type Prompt struct {
	ID      string
	Name    string
	Content string
	Builtin bool
	// Source is PromptSourceManual, PromptSourceSync, or PromptSourceOverride.
	Source string
	// SourcePath is the repository file a synced prompt was applied from.
	SourcePath string
	// AlternateContent is the version not in use: the kept local version of a
	// synced prompt, or the repository version of an override. Empty means
	// none.
	AlternateContent string
	// Conflict marks an override adopted from a manual or built-in prompt
	// whose name a repository file also uses, until the user picks a side.
	Conflict  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsOverride reports whether the prompt locally overrides a synced prompt.
func (p *Prompt) IsOverride() bool {
	return p != nil && p.Source == PromptSourceOverride
}

// IsSynced reports whether Prompt Sync owns the prompt.
func (p *Prompt) IsSynced() bool {
	return p != nil && p.Source == PromptSourceSync
}
