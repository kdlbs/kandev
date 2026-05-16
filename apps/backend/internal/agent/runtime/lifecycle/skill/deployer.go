package skill

import (
	"context"
	"errors"

	"go.uber.org/zap"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

// MetadataKeySkillManifestJSON mirrors lifecycle.MetadataKeySkillManifestJSON
// to avoid an import cycle. It carries the JSON-serialised Manifest the
// Sprites executor consumes during post-create setup to upload skill +
// instruction files into the sprite. The lifecycle test suite asserts
// the same string value.
const MetadataKeySkillManifestJSON = "skill_manifest_json"

// Config holds Deployer dependencies. All fields except Logger are
// optional — a nil reader / lister produces a manifest with no skills
// or instructions, which still lets the deployer run safely (and
// short-circuit on empty content).
type Config struct {
	Logger *logger.Logger

	// BasePath is the kandev home directory (typically ~/.kandev).
	// Files are written under <BasePath>/runtime/...
	BasePath string

	// SkillReader resolves skill records by ID or slug. Optional.
	SkillReader SkillReader

	// InstructionLister returns instruction files for an agent
	// profile. Optional.
	InstructionLister InstructionLister

	// ProjectSkillDirResolver maps agent type ID → project-relative
	// skill dir. Optional; falls back to DefaultProjectSkillDir.
	ProjectSkillDirResolver ProjectSkillDirResolver

	// WorkspaceSlugFn maps a workspace ID to a slug used in on-host
	// runtime paths. Optional; defaults to "default" so single-user
	// installs keep working without explicit wiring.
	WorkspaceSlugFn func(workspaceID string) string
}

// Deployer is the runtime-tier implementation of
// lifecycle.SkillDeployer. It builds a manifest from the profile and
// dispatches delivery by executor type.
type Deployer struct {
	logger                  *logger.Logger
	basePath                string
	skillReader             SkillReader
	instructionLister       InstructionLister
	projectSkillDirResolver ProjectSkillDirResolver
	workspaceSlugFn         func(string) string
}

// New builds a Deployer. The logger is required; everything else is
// optional and degrades gracefully.
func New(cfg Config) (*Deployer, error) {
	if cfg.Logger == nil {
		return nil, errors.New("skill.New: logger is required")
	}
	wsFn := cfg.WorkspaceSlugFn
	if wsFn == nil {
		wsFn = func(string) string { return "default" }
	}
	return &Deployer{
		logger:                  cfg.Logger.WithFields(zap.String("component", "runtime-skill-deployer")),
		basePath:                cfg.BasePath,
		skillReader:             cfg.SkillReader,
		instructionLister:       cfg.InstructionLister,
		projectSkillDirResolver: cfg.ProjectSkillDirResolver,
		workspaceSlugFn:         wsFn,
	}, nil
}

// Request is the input the runtime hands to a Deployer. It mirrors
// lifecycle.SkillDeployRequest but is decoupled from the lifecycle
// package to keep the import boundary one-way.
type Request struct {
	Profile       *settingsmodels.AgentProfile
	WorkspacePath string
	ExecutorType  string
	WorkspaceID   string
	SessionID     string
}

// Deploy materialises the profile's skills and instructions into the
// workspace. It returns the metadata patches and instructions
// directory the lifecycle Manager merges onto the prepared launch
// request.
func (d *Deployer) Deploy(ctx context.Context, req Request) (DeployResult, error) {
	if req.Profile == nil {
		return DeployResult{}, errors.New("skill deploy: profile is required")
	}
	manifest := d.buildManifest(ctx, req.Profile, d.workspaceSlugFn(req.WorkspaceID))
	result := d.deliver(ctx, manifest, req.ExecutorType, req.WorkspacePath)
	return result, nil
}
