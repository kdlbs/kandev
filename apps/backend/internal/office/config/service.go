package config

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"

	"gopkg.in/yaml.v3"
)

// defaultWorkspaceName is used for ConfigLoader lookups when we only have a
// DB workspace ID. Most single-user installs have one workspace named "default".
const defaultWorkspaceName = "default"

// ConfigService provides config export, import, and sync logic.
type ConfigService struct {
	repo      *sqlite.Repository
	cfgLoader *configloader.ConfigLoader
	cfgWriter *configloader.FileWriter
	logger    *logger.Logger
	activity  shared.ActivityLogger
	// importMu keeps each import's read/validate/write lifecycle atomic.
	importMu sync.Mutex
}

// NewConfigService constructs a ConfigService.
func NewConfigService(
	repo *sqlite.Repository,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
	log *logger.Logger,
	activity shared.ActivityLogger,
) *ConfigService {
	return &ConfigService{
		repo:      repo,
		cfgLoader: cfgLoader,
		cfgWriter: cfgWriter,
		logger:    log,
		activity:  activity,
	}
}

// ExportBundle exports the full workspace configuration as a ConfigBundle.
func (s *ConfigService) ExportBundle(ctx context.Context, workspaceID string) (*ConfigBundle, error) {
	bundle := &ConfigBundle{
		Settings: SettingsConfig{Name: workspaceID},
	}

	if err := s.exportAgents(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportSkills(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportRoutines(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportProjects(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (s *ConfigService) exportAgents(ctx context.Context, workspaceID string, bundle *ConfigBundle) error {
	agents, err := s.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	// Build ID->name map for reports_to resolution.
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, a := range agents {
		cfg := AgentConfig{
			Name:                  a.Name,
			Role:                  string(a.Role),
			Icon:                  a.Icon,
			ReportsTo:             nameByID[a.ReportsTo],
			BudgetMonthlyCents:    a.BudgetMonthlyCents,
			MaxConcurrentSessions: a.MaxConcurrentSessions,
			DesiredSkills:         a.DesiredSkills,
			ExecutorPreference:    a.ExecutorPreference,
		}
		bundle.Agents = append(bundle.Agents, cfg)
	}
	return nil
}

func (s *ConfigService) exportSkills(ctx context.Context, workspaceID string, bundle *ConfigBundle) error {
	skills, err := s.repo.ListSkills(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list skills: %w", err)
	}
	for _, sk := range skills {
		cfg := SkillConfig{
			Name:        sk.Name,
			Slug:        sk.Slug,
			Description: sk.Description,
			SourceType:  string(sk.SourceType),
			Content:     sk.Content,
		}
		bundle.Skills = append(bundle.Skills, cfg)
	}
	return nil
}

func (s *ConfigService) exportRoutines(ctx context.Context, workspaceID string, bundle *ConfigBundle) error {
	routines, err := s.repo.ListRoutines(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list routines: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list agents for routine resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, r := range routines {
		cfg := RoutineConfig{
			Name:              r.Name,
			Description:       r.Description,
			TaskTemplate:      r.TaskTemplate,
			AssigneeName:      nameByID[r.AssigneeAgentProfileID],
			ConcurrencyPolicy: string(r.ConcurrencyPolicy),
		}
		bundle.Routines = append(bundle.Routines, cfg)
	}
	return nil
}

func (s *ConfigService) exportProjects(ctx context.Context, workspaceID string, bundle *ConfigBundle) error {
	projects, err := s.repo.ListProjects(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list agents for project resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, p := range projects {
		cfg := ProjectConfig{
			Name:           p.Name,
			Description:    p.Description,
			Status:         string(p.Status),
			Color:          p.Color,
			BudgetCents:    p.BudgetCents,
			Repositories:   p.Repositories,
			ExecutorConfig: p.ExecutorConfig,
			LeadAgentName:  nameByID[p.LeadAgentProfileID],
		}
		bundle.Projects = append(bundle.Projects, cfg)
	}
	return nil
}

// ExportZip exports the workspace configuration as a zip archive.
func (s *ConfigService) ExportZip(ctx context.Context, workspaceID string) (io.Reader, error) {
	manifest, err := s.ExportManifest(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return exportFilesToZip(manifest.Files)
}

// ExportManifest serializes a workspace once so preview and download operate
// on the same bytes rather than independently generated client YAML.
func (s *ConfigService) ExportManifest(ctx context.Context, workspaceID string) (*ExportManifest, error) {
	bundle, err := s.ExportBundle(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	files, err := bundleToFiles(bundle)
	if err != nil {
		return nil, err
	}
	return &ExportManifest{Revision: exportRevision(files), Files: files}, nil
}

// ExportSelectedZip returns only files from the currently advertised
// workspace manifest. The revision check prevents a selection made against an
// older preview from downloading a different configuration.
func (s *ConfigService) ExportSelectedZip(
	ctx context.Context,
	workspaceID, revision string,
	paths []string,
) (io.Reader, error) {
	if len(paths) == 0 {
		return nil, ErrInvalidExportSelection
	}
	manifest, err := s.ExportManifest(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if revision == "" || revision != manifest.Revision {
		return nil, ErrExportRevisionConflict
	}
	selected := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		selected[path] = struct{}{}
	}
	files := make([]ExportFile, 0, len(paths))
	for _, file := range manifest.Files {
		if _, ok := selected[file.Path]; ok {
			files = append(files, file)
			delete(selected, file.Path)
		}
	}
	if len(selected) != 0 {
		return nil, ErrInvalidExportSelection
	}
	return exportFilesToZip(files)
}

var (
	ErrExportRevisionConflict = fmt.Errorf("configuration export is stale")
	ErrInvalidExportSelection = fmt.Errorf("configuration export selection is invalid")
)

// bundleToZip converts a ConfigBundle to a zip archive.
func bundleToZip(bundle *ConfigBundle) (io.Reader, error) {
	files, err := bundleToFiles(bundle)
	if err != nil {
		return nil, err
	}
	return exportFilesToZip(files)
}

func bundleToFiles(bundle *ConfigBundle) ([]ExportFile, error) {
	files := []ExportFile{}
	if err := appendYAMLFile(&files, ".kandev/kandev.yml", bundle.Settings); err != nil {
		return nil, err
	}
	for _, a := range bundle.Agents {
		if err := appendYAMLFile(&files, ".kandev/agents/"+a.Name+".yml", a); err != nil {
			return nil, err
		}
	}
	for _, sk := range bundle.Skills {
		if err := appendYAMLFile(&files, ".kandev/skills/"+sk.Slug+".yml", sk); err != nil {
			return nil, err
		}
	}
	for _, r := range bundle.Routines {
		if err := appendYAMLFile(&files, ".kandev/routines/"+r.Name+".yml", r); err != nil {
			return nil, err
		}
	}
	for _, p := range bundle.Projects {
		if err := appendYAMLFile(&files, ".kandev/projects/"+p.Name+".yml", p); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func appendYAMLFile(files *[]ExportFile, name string, data interface{}) error {
	if err := validateExportPath(files, name); err != nil {
		return err
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	*files = append(*files, ExportFile{Path: name, Content: string(b)})
	return nil
}

// validateExportPath keeps entity-derived archive names inside the
// .kandev tree and rejects duplicate entries. ZIP readers commonly normalize
// path separators, so both slash and backslash traversal are rejected before
// an entry is created.
func validateExportPath(files *[]ExportFile, name string) error {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") ||
		path.IsAbs(name) || !strings.HasPrefix(name, ".kandev/") || path.Clean(name) != name {
		return fmt.Errorf("invalid configuration export path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid configuration export path %q", name)
		}
	}
	for _, file := range *files {
		if file.Path == name {
			return fmt.Errorf("duplicate configuration export path %q", name)
		}
	}
	return nil
}

func exportRevision(files []ExportFile) string {
	ordered := append([]ExportFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	h := sha256.New()
	for _, file := range ordered {
		_, _ = h.Write([]byte(file.Path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(file.Content))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func exportFilesToZip(files []ExportFile) (io.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, file := range files {
		if err := writeZipFile(w, file.Path, []byte(file.Content)); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf, nil
}

func writeZipFile(w *zip.Writer, name string, content []byte) error {
	f, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("write zip entry %s: %w", name, err)
	}
	return nil
}

func writeYAMLFile(w *zip.Writer, name string, data interface{}) error {
	f, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	_, err = f.Write(b)
	return err
}
