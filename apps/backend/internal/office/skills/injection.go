// Package skills hosts office's skill registry. The clean-slate
// CWD-based skill injection that used to live here has moved into
// internal/agent/runtime/lifecycle/skill (the runtime tier) so kanban
// and office launches share a single delivery path. This file keeps
// only the slug parser the SkillService still uses to expand
// DesiredSkills.
package skills

import (
	"encoding/json"
	"strings"
)

// DefaultProjectSkillDir is the fallback CWD-relative skill directory
// used when an agent type does not declare a ProjectSkillDir.
// Re-exported here so the office service / scheduler can reference it
// without depending on the runtime skill package.
const DefaultProjectSkillDir = ".agents/skills"

// AgentTypeResolver resolves an agent profile ID to its agent type ID.
type AgentTypeResolver func(profileID string) string

// ProjectSkillDirResolver resolves the CWD-relative skill directory for an agent
// type ID (e.g. ".claude/skills" for claude-acp, ".agents/skills" for others).
// Returns DefaultProjectSkillDir when the agent type is unknown or not configured.
type ProjectSkillDirResolver func(agentTypeID string) string

// ParseDesiredSlugs parses a DesiredSkills string into a list of slugs.
// Accepts a JSON array (the canonical persistence format) or a
// comma-separated list (legacy). Empty inputs return nil.
func ParseDesiredSlugs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var slugs []string
		if err := json.Unmarshal([]byte(raw), &slugs); err == nil {
			return filterEmpty(slugs)
		}
	}
	return filterEmpty(strings.Split(raw, ","))
}

func filterEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
