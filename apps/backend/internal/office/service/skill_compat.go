// skill_compat.go re-exports a small set of symbols the in-package
// SchedulerIntegration tick uses (BuildAgentPrompt + manifest builder)
// after ADR 0005 Wave E moved skill / instruction file delivery into
// the runtime. The real CRUD + service layer is in
// internal/office/skills; this file keeps the unexported bridge
// helpers compiling without touching every legacy callsite.
package service

import officeskills "github.com/kandev/kandev/internal/office/skills"

// AgentTypeResolver resolves an agent profile ID to its agent type ID.
// Re-exported from skills.AgentTypeResolver so the office Service can
// hold a resolver in its struct without importing the skills package
// from every callsite.
type AgentTypeResolver = officeskills.AgentTypeResolver

// ProjectSkillDirResolver resolves the CWD-relative skill directory
// for an agent type. Re-exported from skills.ProjectSkillDirResolver
// for the same reason as AgentTypeResolver.
type ProjectSkillDirResolver = officeskills.ProjectSkillDirResolver

// SkillSourceTypeInline is the default skill source type for content
// stored directly in the database. Re-exported so service.go's
// CreateSkill default can reference it without importing the skills
// package directly.
const SkillSourceTypeInline = officeskills.SkillSourceTypeInline

// DefaultProjectSkillDir is the fallback CWD-relative skill directory
// the scheduler/run.go resolver falls back on.
const DefaultProjectSkillDir = officeskills.DefaultProjectSkillDir

// ParseDesiredSlugs parses a DesiredSkills string into a list of slugs.
// Re-exported for the manifest builder + tests.
var ParseDesiredSlugs = officeskills.ParseDesiredSlugs

// kandevBasePath returns the kandev home directory base path.
// Used by skill_lookup.go (instructionsDir resolver) and tests.
func (s *Service) kandevBasePath() string {
	if s.cfgLoader != nil {
		return s.cfgLoader.BasePath()
	}
	return ""
}

// resolveAgentType maps a profile ID to its agent type via the
// configured resolver. Used by skill_manifest.go.
func (s *Service) resolveAgentType(profileID string) string {
	if s.agentTypeResolver == nil || profileID == "" {
		return ""
	}
	return s.agentTypeResolver(profileID)
}

// resolveProjectSkillDir maps an agent type ID to its CWD-relative
// skill directory via the configured resolver. Falls back to
// DefaultProjectSkillDir.
func (s *Service) resolveProjectSkillDir(agentTypeID string) string {
	if s.projectSkillDirResolver == nil || agentTypeID == "" {
		return DefaultProjectSkillDir
	}
	if dir := s.projectSkillDirResolver(agentTypeID); dir != "" {
		return dir
	}
	return DefaultProjectSkillDir
}
