package configsync

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

// Limits bounds a single sync run's traversal, in the units
// AC-OFFICE-CONFIG-SYNC-002.5 promises: skill subdirectories and files.
type Limits struct {
	MaxSkills int
	MaxFiles  int
}

// DefaultLimits are the limits every run is held to.
var DefaultLimits = Limits{MaxSkills: 200, MaxFiles: 1000}

// MaxFileBytes is the per-file size limit enforced on every fetched file,
// measured on received content before it is parsed.
const MaxFileBytes = 1 << 20 // 1 MiB

const kandevYAMLName = "kandev.yml"
const skillDefinitionName = "SKILL.md"

// DirEntry is a provider-neutral directory listing entry.
type DirEntry struct {
	Name   string
	Path   string
	IsFile bool
}

// GitHubClientProvider exposes workspace-routed GitHub repository reads.
// Satisfied by github.Service.
type GitHubClientProvider interface {
	ListRepoDirectoryForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]github.RepoContentEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]byte, error)
}

// GitLabClientProvider exposes workspace-routed GitLab repository reads.
// Satisfied by gitlab.Service.
type GitLabClientProvider interface {
	ListRepoTreeForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]gitlab.RepoTreeEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]byte, error)
}

// Compile-time checks that both integrations' real services satisfy the
// interfaces above, so drift in either package's workspace-routed methods
// breaks the build rather than surfacing only at DI-wiring time.
var (
	_ GitHubClientProvider = (*github.Service)(nil)
	_ GitLabClientProvider = (*gitlab.Service)(nil)
)

// fetchedFile is one file the walk selected and successfully read.
type fetchedFile struct {
	path    string
	content []byte
}

// unreadableFile is one file the walk selected but could not read: over the
// size limit, gone at fetch time, or any other unreadable-content failure.
// It never fails the run on its own (AC-OFFICE-CONFIG-SYNC-002.4a); the
// reconciler warns naming it and exempts the entity it previously defined
// from deletion.
type unreadableFile struct {
	path   string
	reason string
}

// skillFiles is one skill directory's selected files: SKILL.md plus every
// regular file directly under references/, keyed by the skill directory
// name and path.
type skillFiles struct {
	dirName        string
	dirPath        string
	skillMD        *fetchedFile
	skillMDUnread  *unreadableFile
	references     []fetchedFile
	unreadableRefs []unreadableFile
}

// walkResult is what a bounded walk produced.
type walkResult struct {
	agentFiles        []fetchedFile
	projectFiles      []fetchedFile
	routineFiles      []fetchedFile
	skills            []skillFiles
	unreadable        []unreadableFile
	kandevYAMLPresent bool
}

// walkFailure is a run-ending outcome of the walk: an unavailable/unreadable
// listing, an unavailable file fetch, or a cap exceeded. capped distinguishes
// a cap failure, whose warning is worded differently from an error failure.
type walkFailure struct {
	reason string
	capped bool
}

func (f *walkFailure) Error() string { return f.reason }

// fileBudget tracks how many files one Walk call has fetched against
// Limits.MaxFiles, so AC-OFFICE-CONFIG-SYNC-002.5's file cap stops the walk
// from issuing further fetches once it is reached, rather than only being
// checked in an aggregate count after every fetch has already happened.
type fileBudget struct {
	limit    int
	selected int
}

// reserve claims budget for one more fetch, or returns a capped walkFailure
// when the limit has already been reached.
func (b *fileBudget) reserve() *walkFailure {
	if b.selected >= b.limit {
		return &walkFailure{
			reason: fmt.Sprintf(
				"file cap exceeded: more than %d files selected; stopping before fetching further", b.limit),
			capped: true,
		}
	}
	b.selected++
	return nil
}

// walker drives the bounded multi-round directory walk over one provider,
// built only from each provider's existing non-recursive listing call.
type walker struct {
	gh     GitHubClientProvider
	gl     GitLabClientProvider
	limits Limits
}

func newWalker(gh GitHubClientProvider, gl GitLabClientProvider, limits Limits) *walker {
	return &walker{gh: gh, gl: gl, limits: limits}
}

// Walk performs the bounded traversal described in
// docs/specs/office/system-design/config-sync-reconciliation.md#bounded-walk:
// round 1 lists the configured root plus its four kind subdirectories; round
// 2 lists each skill directory and its references/ subdirectory. The
// configured root's listing is the access probe (AC-OFFICE-CONFIG-SYNC-002.4)
// — any failure there, including not-found, fails the run.
func (w *walker) Walk(ctx context.Context, cfg *Config) (*walkResult, *walkFailure) {
	root := normalizePathFrame(cfg.Path)
	rootEntries, err := w.list(ctx, cfg, root)
	if err != nil {
		return nil, &walkFailure{reason: fmt.Sprintf("list configured path %q: %v", displayPath(root), err)}
	}
	result := &walkResult{kandevYAMLPresent: hasKandevYAML(rootEntries)}
	budget := &fileBudget{limit: w.limits.MaxFiles}

	if ferr := w.walkFlatKinds(ctx, cfg, root, result, budget); ferr != nil {
		return nil, ferr
	}

	skills, ferr := w.walkSkills(ctx, cfg, path.Join(root, "skills"), budget)
	if ferr != nil {
		return nil, ferr
	}
	result.skills = skills

	sortWalkResult(result)
	return result, nil
}

func hasKandevYAML(entries []DirEntry) bool {
	for _, e := range entries {
		if e.IsFile && e.Name == kandevYAMLName {
			return true
		}
	}
	return false
}

func (w *walker) walkFlatKinds(ctx context.Context, cfg *Config, root string, result *walkResult, budget *fileBudget) *walkFailure {
	kinds := []struct {
		dir  string
		dest *[]fetchedFile
	}{
		{"agents", &result.agentFiles},
		{"projects", &result.projectFiles},
		{"routines", &result.routineFiles},
	}
	for _, k := range kinds {
		res, ferr := w.walkFlatKind(ctx, cfg, path.Join(root, k.dir), budget)
		if ferr != nil {
			return ferr
		}
		*k.dest = res.files
		result.unreadable = append(result.unreadable, res.unreadable...)
	}
	return nil
}

// kindResult is one flat kind directory's selected files and any that could
// not be read.
type kindResult struct {
	files      []fetchedFile
	unreadable []unreadableFile
}

// walkFlatKind lists one flat kind directory (agents/, projects/, routines/)
// and fetches every *.yml/*.yaml entry directly in it.
// AC-OFFICE-CONFIG-SYNC-002.3: not-found beneath the root is an absent
// directory, contributing no files. Any other listing failure fails the run.
func (w *walker) walkFlatKind(ctx context.Context, cfg *Config, dir string, budget *fileBudget) (kindResult, *walkFailure) {
	entries, err := w.list(ctx, cfg, dir)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return kindResult{}, nil
		}
		return kindResult{}, &walkFailure{reason: fmt.Sprintf("list %s: %v", dir, err)}
	}
	var res kindResult
	for _, e := range entries {
		if !e.IsFile || !isYAMLFile(e.Name) {
			continue
		}
		content, ferr := w.fetchOne(ctx, cfg, e.Path, &res, budget)
		if ferr != nil {
			return kindResult{}, ferr
		}
		if content != nil {
			res.files = append(res.files, fetchedFile{path: e.Path, content: content})
		}
	}
	return res, nil
}

// fetchOne fetches one selected file. An unavailable fetch fails the run; a
// not-found or unreadable-content fetch appends to res.unreadable and
// returns (nil, nil) so the caller continues. budget.reserve is checked
// before the fetch so the file cap stops issuing fetches rather than only
// being noticed afterward.
func (w *walker) fetchOne(ctx context.Context, cfg *Config, filePath string, res *kindResult, budget *fileBudget) ([]byte, *walkFailure) {
	if ferr := budget.reserve(); ferr != nil {
		return nil, ferr
	}
	content, err := w.fetch(ctx, cfg, filePath)
	if err == nil {
		return content, nil
	}
	if errors.Is(err, ErrUnavailable) {
		return nil, &walkFailure{reason: fmt.Sprintf("fetch %s: %v", filePath, err)}
	}
	res.unreadable = append(res.unreadable, unreadableFile{path: filePath, reason: err.Error()})
	return nil, nil
}

// walkSkills lists the skills/ directory and, for every subdirectory (up to
// the skill cap), fetches its SKILL.md and references/ files.
// AC-OFFICE-CONFIG-SYNC-002.5: the skill cap is evaluated here, before any
// round-2 request, so a repository exceeding it produces the same message on
// every run.
func (w *walker) walkSkills(ctx context.Context, cfg *Config, skillsDir string, budget *fileBudget) ([]skillFiles, *walkFailure) {
	entries, err := w.list(ctx, cfg, skillsDir)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, &walkFailure{reason: fmt.Sprintf("list %s: %v", skillsDir, err)}
	}
	var dirs []DirEntry
	for _, e := range entries {
		if !e.IsFile {
			dirs = append(dirs, e)
		}
	}
	if len(dirs) > w.limits.MaxSkills {
		return nil, &walkFailure{
			reason: fmt.Sprintf("skill directory cap exceeded: %d skill directories found, limit is %d (%d dropped)",
				len(dirs), w.limits.MaxSkills, len(dirs)-w.limits.MaxSkills),
			capped: true,
		}
	}
	skills := make([]skillFiles, 0, len(dirs))
	for _, d := range dirs {
		sf, ferr := w.walkOneSkill(ctx, cfg, d, budget)
		if ferr != nil {
			return nil, ferr
		}
		skills = append(skills, sf)
	}
	return skills, nil
}

func (w *walker) walkOneSkill(ctx context.Context, cfg *Config, dir DirEntry, budget *fileBudget) (skillFiles, *walkFailure) {
	sf := skillFiles{dirName: dir.Name, dirPath: dir.Path}

	entries, err := w.list(ctx, cfg, dir.Path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return skillFiles{}, &walkFailure{reason: fmt.Sprintf("list %s: %v", dir.Path, err)}
	}
	for _, e := range entries {
		if e.IsFile && e.Name == skillDefinitionName {
			if ferr := w.fetchSkillMD(ctx, cfg, e.Path, &sf, budget); ferr != nil {
				return skillFiles{}, ferr
			}
		}
	}

	refDir := path.Join(dir.Path, "references")
	refEntries, err := w.list(ctx, cfg, refDir)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return skillFiles{}, &walkFailure{reason: fmt.Sprintf("list %s: %v", refDir, err)}
	}
	for _, e := range refEntries {
		if !e.IsFile {
			continue
		}
		if ferr := w.fetchReference(ctx, cfg, e.Path, &sf, budget); ferr != nil {
			return skillFiles{}, ferr
		}
	}
	return sf, nil
}

func (w *walker) fetchSkillMD(ctx context.Context, cfg *Config, filePath string, sf *skillFiles, budget *fileBudget) *walkFailure {
	if ferr := budget.reserve(); ferr != nil {
		return ferr
	}
	content, err := w.fetch(ctx, cfg, filePath)
	if err == nil {
		f := fetchedFile{path: filePath, content: content}
		sf.skillMD = &f
		return nil
	}
	if errors.Is(err, ErrUnavailable) {
		return &walkFailure{reason: fmt.Sprintf("fetch %s: %v", filePath, err)}
	}
	u := unreadableFile{path: filePath, reason: err.Error()}
	sf.skillMDUnread = &u
	return nil
}

func (w *walker) fetchReference(ctx context.Context, cfg *Config, filePath string, sf *skillFiles, budget *fileBudget) *walkFailure {
	if ferr := budget.reserve(); ferr != nil {
		return ferr
	}
	content, err := w.fetch(ctx, cfg, filePath)
	if err == nil {
		sf.references = append(sf.references, fetchedFile{path: filePath, content: content})
		return nil
	}
	if errors.Is(err, ErrUnavailable) {
		return &walkFailure{reason: fmt.Sprintf("fetch %s: %v", filePath, err)}
	}
	sf.unreadableRefs = append(sf.unreadableRefs, unreadableFile{path: filePath, reason: err.Error()})
	return nil
}

// list dispatches a directory listing to the configured provider and
// converts its native entry shape to the neutral DirEntry.
func (w *walker) list(ctx context.Context, cfg *Config, dir string) ([]DirEntry, error) {
	switch cfg.Provider {
	case ProviderGitHub:
		if w.gh == nil {
			return nil, fmt.Errorf("config sync: github integration is not connected for this workspace")
		}
		entries, err := w.gh.ListRepoDirectoryForWorkspace(ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, dir, cfg.Branch)
		if err != nil {
			return nil, classifyFetchErr(err)
		}
		out := make([]DirEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, DirEntry{Name: e.Name, Path: e.Path, IsFile: e.Type == github.RepoContentTypeFile})
		}
		return out, nil
	case ProviderGitLab:
		if w.gl == nil {
			return nil, fmt.Errorf("config sync: gitlab integration is not connected for this workspace")
		}
		entries, err := w.gl.ListRepoTreeForWorkspace(ctx, cfg.WorkspaceID, cfg.ProjectPath, dir, cfg.Branch)
		if err != nil {
			return nil, classifyFetchErr(err)
		}
		out := make([]DirEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, DirEntry{Name: e.Name, Path: e.Path, IsFile: e.Type == gitlab.TreeEntryTypeBlob})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("config sync: unknown provider %q", cfg.Provider)
	}
}

// fetch dispatches a file content read to the configured provider and
// enforces the per-file size limit on the result.
// AC-OFFICE-CONFIG-SYNC-002.6/002.6b: measured on received content, since
// only GitLab's client caps a read and only GitHub's listing reports size.
func (w *walker) fetch(ctx context.Context, cfg *Config, filePath string) ([]byte, error) {
	var content []byte
	var err error
	switch cfg.Provider {
	case ProviderGitHub:
		if w.gh == nil {
			return nil, fmt.Errorf("config sync: github integration is not connected for this workspace")
		}
		content, err = w.gh.GetRepoFileContentForWorkspace(ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, filePath, cfg.Branch)
	case ProviderGitLab:
		if w.gl == nil {
			return nil, fmt.Errorf("config sync: gitlab integration is not connected for this workspace")
		}
		content, err = w.gl.GetRepoFileContentForWorkspace(ctx, cfg.WorkspaceID, cfg.ProjectPath, filePath, cfg.Branch)
	default:
		return nil, fmt.Errorf("config sync: unknown provider %q", cfg.Provider)
	}
	if err != nil {
		return nil, classifyFetchErr(err)
	}
	if len(content) > MaxFileBytes {
		return nil, fmt.Errorf("%w: file exceeds %d byte limit", ErrUnreadable, MaxFileBytes)
	}
	return content, nil
}

// sortWalkResult orders every file slice by full repository path, ascending
// and byte-wise (AC-OFFICE-CONFIG-SYNC-002.7), so a run over an unchanged
// repository orders identically every time.
func sortWalkResult(result *walkResult) {
	sortFiles(result.agentFiles)
	sortFiles(result.projectFiles)
	sortFiles(result.routineFiles)
	sort.Slice(result.skills, func(i, j int) bool { return result.skills[i].dirPath < result.skills[j].dirPath })
	for i := range result.skills {
		sortFiles(result.skills[i].references)
	}
}

func sortFiles(files []fetchedFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
}

func displayPath(root string) string {
	if root == "" {
		return "(repository root)"
	}
	return root
}

// isYAMLFile reports whether name has a .yml or .yaml extension.
func isYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}
