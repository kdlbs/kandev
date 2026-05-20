package skill

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// buildManifest resolves a profile's desired skills + instruction files
// into a Manifest the delivery strategies can consume. It is pure data
// — no filesystem side effects.
//
// The agent profile is the unified ADR 0005 row. We read both
// SkillIDs (the new merged column) and DesiredSkills (legacy office
// column) and union the two into a single list of slugs / IDs. Empty
// slots are dropped before lookup.
func (d *Deployer) buildManifest(ctx context.Context, profile *settingsmodels.AgentProfile, workspaceSlug string) *Manifest {
	// Profile.AgentID IS the agent type ID after ADR 0005 — the
	// agent_profiles row's agent_id column points at the agents
	// table (claude-acp, codex-acp, ...). No extra resolver needed.
	agentTypeID := profile.AgentID
	manifest := &Manifest{
		AgentTypeID:     agentTypeID,
		WorkspaceSlug:   workspaceSlug,
		AgentID:         profile.ID,
		ProjectSkillDir: d.resolveProjectSkillDir(agentTypeID),
	}
	d.appendSkills(ctx, manifest, profile)
	d.appendInstructions(ctx, manifest, profile.ID)
	return manifest
}

// appendSkills resolves every desired slug / id on the profile to a
// runtime Skill record. Lookups that fail are logged at debug level
// and dropped — a missing skill must never abort a launch.
func (d *Deployer) appendSkills(ctx context.Context, manifest *Manifest, profile *settingsmodels.AgentProfile) {
	if d.skillReader == nil {
		return
	}
	for _, key := range mergedSkillKeys(profile) {
		skill, err := d.skillReader.GetSkillFromConfig(ctx, key)
		if err != nil || skill == nil {
			d.logger.Debug("skip skill in manifest",
				zap.String("key", key), zap.Error(err))
			continue
		}
		manifest.Skills = append(manifest.Skills, *skill)
	}
}

// appendInstructions appends every instruction file persisted for the
// agent profile. A nil InstructionLister or repo error leaves the
// manifest's Instructions slice empty — the prompt builder treats an
// empty AGENTS.md as "no preamble".
func (d *Deployer) appendInstructions(ctx context.Context, manifest *Manifest, agentProfileID string) {
	if d.instructionLister == nil {
		return
	}
	files, err := d.instructionLister.ListInstructions(ctx, agentProfileID)
	if err != nil {
		d.logger.Warn("failed to load instructions for manifest",
			zap.String("agent_id", agentProfileID), zap.Error(err))
		return
	}
	for _, f := range files {
		if f == nil {
			continue
		}
		manifest.Instructions = append(manifest.Instructions, *f)
	}
}

// resolveProjectSkillDir maps an agent type ID to the CWD-relative
// project skill directory. Returns DefaultProjectSkillDir when no
// resolver is configured or the resolver returns empty.
func (d *Deployer) resolveProjectSkillDir(agentTypeID string) string {
	if d.projectSkillDirResolver == nil || agentTypeID == "" {
		return DefaultProjectSkillDir
	}
	if dir := d.projectSkillDirResolver(agentTypeID); dir != "" {
		return dir
	}
	return DefaultProjectSkillDir
}

// mergedSkillKeys returns the union of profile.SkillIDs and
// profile.DesiredSkills, with empties trimmed. Duplicates collapse.
// The order is SkillIDs first (the canonical post-ADR field), then
// DesiredSkills (legacy office input that callers still populate).
//
// Both fields are JSON-array TEXT columns; non-empty values are unmarshalled
// into a list of slugs and concatenated. Malformed JSON is treated as an
// empty list (the launch-prep no-op fast path) — the caller's responsibility
// is to surface validation upstream.
func mergedSkillKeys(profile *settingsmodels.AgentProfile) []string {
	if profile == nil {
		return nil
	}
	skillIDs := decodeSkillSlugs(profile.SkillIDs)
	desired := decodeSkillSlugs(profile.DesiredSkills)
	seen := make(map[string]bool, len(skillIDs)+len(desired))
	out := make([]string, 0, len(skillIDs)+len(desired))
	for _, k := range skillIDs {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	for _, k := range desired {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// decodeSkillSlugs parses a JSON-array string into a []string. Empty input
// or non-array input collapses to nil so callers can treat "no entries"
// uniformly without checking for malformed JSON.
func decodeSkillSlugs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
