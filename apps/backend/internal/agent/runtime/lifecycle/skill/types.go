// Package skill is the runtime-tier implementation of per-profile skill
// and instruction-file deployment introduced in ADR 0005 Wave A. The
// package owns the manifest builder and the per-executor delivery
// strategies (local filesystem symlinks, Docker bind-mount, Sprites
// upload). Wave E moved this code out of internal/office into the
// runtime so every launch (kanban or office) goes through the same
// path.
//
// Concrete office wiring is provided externally: the runtime accepts
// SkillReader / InstructionLister implementations that the office
// service exposes from its repository. The runtime never imports
// office; office never duplicates the deployer's logic.
package skill

import "context"

// Skill is the runtime-tier view of a skill record. It carries only
// what the deployer needs to materialise files; the upstream office
// model (with workspace, file inventory, created_at, ...) collapses
// to these three fields.
type Skill struct {
	Slug    string
	Content string
	// SourceType discriminates how Content was produced. Today the
	// deployer only writes inline content; other source types are
	// resolved upstream by the SkillReader before reaching the
	// deployer.
	SourceType string
}

// InstructionFile is the runtime-tier view of an agent's instruction
// file (e.g. AGENTS.md, HEARTBEAT.md). Offices store these per
// agent_profile_id; the deployer writes them to a per-agent
// instructions directory.
type InstructionFile struct {
	Filename string
	Content  string
	IsEntry  bool
}

// SkillReader resolves a skill by ID or slug. Implemented by the
// office service (skill_compat.go) and passed in via the deployer
// constructor.
type SkillReader interface {
	GetSkillFromConfig(ctx context.Context, idOrSlug string) (*Skill, error)
}

// InstructionLister returns the instruction files associated with a
// given agent_profile_id. Implemented by the office repository and
// passed in via the deployer constructor.
type InstructionLister interface {
	ListInstructions(ctx context.Context, agentProfileID string) ([]*InstructionFile, error)
}

// ProjectSkillDirResolver maps an agent type ID to the CWD-relative
// project skill directory the agent expects (e.g. ".claude/skills",
// ".agents/skills"). Returns "" for unknown agent types — callers
// fall back to DefaultProjectSkillDir.
type ProjectSkillDirResolver func(agentTypeID string) string

// DefaultProjectSkillDir is the fallback CWD-relative skill directory
// used when an agent type does not declare a ProjectSkillDir.
const DefaultProjectSkillDir = ".agents/skills"

// Manifest holds the resolved skills and instructions for a single
// launch. It is built before delivery so the strategy can adapt to
// the executor type without re-querying upstream stores.
type Manifest struct {
	Skills          []Skill
	Instructions    []InstructionFile
	AgentTypeID     string
	WorkspaceSlug   string
	AgentID         string
	ProjectSkillDir string
}

// DeployResult carries information the runtime needs to wire after a
// successful deploy: the per-executor metadata to attach to the
// LaunchRequest, and the on-host instructions directory the office
// prompt builder reads from.
type DeployResult struct {
	// Metadata is a set of MetadataKey* entries to merge onto the
	// prepared LaunchRequest before the executor backend runs.
	Metadata map[string]any
	// InstructionsDir is the path (host or sprite-side) where
	// instruction files were written. May be empty when no
	// instructions were materialised.
	InstructionsDir string
}
