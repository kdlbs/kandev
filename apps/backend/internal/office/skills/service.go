package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// skillRepo is the persistence interface required by SkillService.
// `ListSystemSkills` / `GetSkillBySlug` are needed by the lazy
// per-workspace system-skill sync that fires on each list/inject
// path (ensures fresh workspaces created after backend startup
// receive the bundled set on their first skills access).
type skillRepo interface {
	CreateSkill(ctx context.Context, skill *models.Skill) error
	GetSkill(ctx context.Context, id string) (*models.Skill, error)
	GetSkillBySlug(ctx context.Context, workspaceID, slug string) (*models.Skill, error)
	ListSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error)
	ListSystemSkills(ctx context.Context, workspaceID string) ([]*models.Skill, error)
	UpdateSkill(ctx context.Context, skill *models.Skill) error
	DeleteSkill(ctx context.Context, id string) error

	// Agent-profile access threaded through to SystemSyncRepo so the
	// lazy per-workspace sync can scrub deleted system-skill IDs out
	// of agent_profiles.skill_ids.
	ListAgentInstances(ctx context.Context, workspaceID string) ([]*settingsmodels.AgentProfile, error)
	UpdateAgentInstance(ctx context.Context, agent *settingsmodels.AgentProfile) error
}

// configLoader is the subset of configloader.ConfigLoader used by SkillService.
type configLoader interface {
	BasePath() string
}

// SkillService provides skill CRUD and related business logic.
type SkillService struct {
	repo                 skillRepo
	logger               *logger.Logger
	activity             shared.ActivityLogger
	agents               shared.AgentReader
	cfgLoader            configLoader
	userSkillDirResolver UserSkillDirResolver
	userHomeResolver     func() (string, error)
}

// NewSkillService creates a new SkillService.
func NewSkillService(
	repo skillRepo,
	log *logger.Logger,
	activity shared.ActivityLogger,
	agents shared.AgentReader,
	cfgLoader configLoader,
) *SkillService {
	return &SkillService{
		repo:             repo,
		logger:           log.WithFields(zap.String("component", "office-skills")),
		activity:         activity,
		agents:           agents,
		cfgLoader:        cfgLoader,
		userHomeResolver: osUserHomeDir,
	}
}

// SkillSourceTypeInline is the default skill source type for content stored
// directly in the DB.
const SkillSourceTypeInline = "inline"

// SkillSourceTypeUserHome identifies provider skills imported from the user's
// home-directory agent skill stores as DB snapshots.
const SkillSourceTypeUserHome = "user_home"

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateSlug creates a kebab-case slug from a name.
func GenerateSlug(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	slug := nonAlphanumRe.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "skill"
	}
	return slug
}

// SkillWithUsage extends models.Skill with the number of agent instances using it.
type SkillWithUsage struct {
	models.Skill
	UsedByCount int `json:"used_by_count"`
}

// ListSkillsWithUsage returns all skills for a workspace with used-by-agent counts.
func (s *SkillService) ListSkillsWithUsage(ctx context.Context, wsID string) ([]*SkillWithUsage, error) {
	skills, err := s.ListSkillsFromConfig(ctx, wsID)
	if err != nil {
		return nil, err
	}
	counts := s.countSkillUsage(ctx)
	result := make([]*SkillWithUsage, len(skills))
	for i, sk := range skills {
		result[i] = &SkillWithUsage{Skill: *sk, UsedByCount: counts[sk.Slug]}
	}
	return result, nil
}

// countSkillUsage counts how many agents reference each skill slug.
func (s *SkillService) countSkillUsage(ctx context.Context) map[string]int {
	counts := make(map[string]int)
	if s.agents == nil {
		return counts
	}
	agents, err := s.agents.ListAgentInstances(ctx, "")
	if err != nil {
		return counts
	}
	for _, a := range agents {
		for _, slug := range ParseDesiredSlugs(a.DesiredSkills) {
			counts[slug]++
		}
	}
	return counts
}

// ValidateAndPrepareSkill validates and prepares a skill for creation.
// Call this before CreateSkill to auto-generate slug and validate uniqueness.
func (s *SkillService) ValidateAndPrepareSkill(ctx context.Context, skill *models.Skill) error {
	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if skill.Slug == "" {
		skill.Slug = GenerateSlug(skill.Name)
	}
	if err := s.validateSlugUnique(ctx, skill.WorkspaceID, skill.Slug, ""); err != nil {
		return err
	}
	prepareSkillPackageMetadata(skill)
	return s.validateSourceType(string(skill.SourceType))
}

func prepareSkillPackageMetadata(skill *models.Skill) {
	if skill.Version == "" {
		skill.Version = "1"
	}
	if skill.ApprovalState == "" {
		skill.ApprovalState = "approved"
	}
	sum := sha256.Sum256([]byte(skill.Content + "\x00" + skill.FileInventory + "\x00" + skill.SourceLocator))
	skill.ContentHash = hex.EncodeToString(sum[:])
}

// ValidateSkillUpdate validates a skill update for slug uniqueness.
func (s *SkillService) ValidateSkillUpdate(ctx context.Context, skill *models.Skill) error {
	if skill.Slug != "" {
		return s.validateSlugUnique(ctx, skill.WorkspaceID, skill.Slug, skill.ID)
	}
	return nil
}

func (s *SkillService) validateSlugUnique(ctx context.Context, workspaceID, slug, excludeID string) error {
	skills, err := s.repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list skills for slug uniqueness check: %w", err)
	}
	for _, si := range skills {
		if si.Slug == slug && si.ID != excludeID {
			return fmt.Errorf("skill slug %q already exists in this workspace", slug)
		}
	}
	return nil
}

func (s *SkillService) validateSourceType(sourceType string) error {
	switch sourceType {
	case SkillSourceTypeInline, SkillSourceTypeUserHome, "local_path", "git", "skills_sh", "":
		return nil
	default:
		return fmt.Errorf("invalid source type: %q", sourceType)
	}
}

// CreateSkill creates a new skill in the DB.
func (s *SkillService) CreateSkill(ctx context.Context, skill *models.Skill) error {
	if skill.SourceType == "" {
		skill.SourceType = SkillSourceTypeInline
	}
	if skill.FileInventory == "" {
		skill.FileInventory = "[]"
	}
	prepareSkillPackageMetadata(skill)
	if err := s.repo.CreateSkill(ctx, skill); err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

// GetSkill returns a skill by ID.
func (s *SkillService) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	return s.GetSkillFromConfig(ctx, id)
}

// ListSkills returns all skills for a workspace.
func (s *SkillService) ListSkills(ctx context.Context, wsID string) ([]*models.Skill, error) {
	return s.ListSkillsFromConfig(ctx, wsID)
}

// UpdateSkill updates a skill in the DB.
func (s *SkillService) UpdateSkill(ctx context.Context, skill *models.Skill) error {
	prepareSkillPackageMetadata(skill)
	if err := s.repo.UpdateSkill(ctx, skill); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return nil
}

// DeleteSkill deletes a skill from the DB.
func (s *SkillService) DeleteSkill(ctx context.Context, id string) error {
	skill, err := s.GetSkillFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteSkill(ctx, skill.ID); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

// GetSkillFromConfig looks up a skill by ID or slug.
func (s *SkillService) GetSkillFromConfig(ctx context.Context, idOrSlug string) (*models.Skill, error) {
	if skill, err := s.repo.GetSkill(ctx, idOrSlug); err == nil {
		return skill, nil
	}
	skills, err := s.repo.ListSkills(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, sk := range skills {
		if sk.Slug == idOrSlug {
			return sk, nil
		}
	}
	return nil, fmt.Errorf("skill not found: %s", idOrSlug)
}

// ListSkillsFromConfig returns all skills for a workspace.
// An empty workspaceID returns rows across all workspaces.
//
// Lazy system-skill sync: before reading the list, ensure the
// bundled system skills are present for `workspaceID`. Covers
// workspaces created after backend startup (e.g. via the onboarding
// wizard) — the startup sync only sees workspaces that existed when
// the backend booted. Idempotent; cheap when the system rows are
// already current.
func (s *SkillService) ListSkillsFromConfig(ctx context.Context, workspaceID string) ([]*models.Skill, error) {
	if workspaceID != "" {
		s.ensureSystemSkillsForWorkspace(ctx, workspaceID)
	}
	return s.repo.ListSkills(ctx, workspaceID)
}

// ensureSystemSkillsForWorkspace is the lazy per-workspace counterpart
// of SyncSystemSkills. Errors are logged and swallowed: a missed sync
// shows up as an empty System group in the UI, not a 500 from the
// list endpoint.
func (s *SkillService) ensureSystemSkillsForWorkspace(ctx context.Context, workspaceID string) {
	if _, err := SyncSystemSkills(ctx, s.repo, []string{workspaceID}, nil, s.logger); err != nil {
		s.logger.Warn("ensure system skills for workspace failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}
