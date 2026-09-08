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

// kindPlan is one flat kind directory's (agents/, projects/, routines/)
// selected file paths, found while listing round 1, before any content fetch
// is issued. dest is where fetchFlatKinds writes the resulting fetchedFiles.
type kindPlan struct {
	dest  *[]fetchedFile
	paths []string
}

// skillPlan is one skill directory's selected file paths, found while
// listing round 2, before any content fetch is issued. skillMDPath is empty
// when the directory has no SKILL.md. skillMDUnread is set instead, in
// place of an empty skillMDPath, when the directory itself vanished between
// round 1 (which just listed it as an existing entry) and this listing: a
// same-run disappearance is a race, not proof the directory never had a
// SKILL.md, so it is carried through fetchSkills exactly like an unreadable
// SKILL.md rather than as "genuinely empty" (AC-OFFICE-CONFIG-SYNC-002.6a).
type skillPlan struct {
	dirName       string
	dirPath       string
	skillMDPath   string
	skillMDUnread *unreadableFile
	refPaths      []string
}

// candidateCount is the total number of files planFlatKinds and planSkills
// selected across every directory, known once listing is complete and before
// any fetch is issued — which is what lets the file cap
// (AC-OFFICE-CONFIG-SYNC-002.5) report an exact dropped count instead of
// only "more than N selected": listing calls are cheap and already bounded by
// their own budget, so counting candidates this way costs nothing extra and
// never re-introduces round 3's runaway-fetch bug (which paid for fetching
// content just to count it).
func candidateCount(kindPlans []kindPlan, skillPlans []skillPlan) int {
	total := 0
	for _, kp := range kindPlans {
		total += len(kp.paths)
	}
	for _, sp := range skillPlans {
		if sp.skillMDPath != "" {
			total++
		}
		total += len(sp.refPaths)
	}
	return total
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

	kindPlans, ferr := w.planFlatKinds(ctx, cfg, root, result)
	if ferr != nil {
		return nil, ferr
	}
	skillPlans, ferr := w.planSkills(ctx, cfg, path.Join(root, "skills"))
	if ferr != nil {
		return nil, ferr
	}

	if total := candidateCount(kindPlans, skillPlans); total > w.limits.MaxFiles {
		return nil, &walkFailure{
			reason: fmt.Sprintf(
				"file cap exceeded: %d files found, limit is %d (%d dropped); stopping before fetching further",
				total, w.limits.MaxFiles, total-w.limits.MaxFiles),
			capped: true,
		}
	}

	if ferr := w.fetchFlatKinds(ctx, cfg, kindPlans, result); ferr != nil {
		return nil, ferr
	}
	skills, ferr := w.fetchSkills(ctx, cfg, skillPlans)
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

// planFlatKinds lists the three flat kind directories (agents/, projects/,
// routines/) and selects every *.yml/*.yaml entry directly in each, without
// fetching any content yet. AC-OFFICE-CONFIG-SYNC-002.3: not-found beneath
// the root is an absent directory, contributing no files. Any other listing
// failure fails the run.
func (w *walker) planFlatKinds(ctx context.Context, cfg *Config, root string, result *walkResult) ([]kindPlan, *walkFailure) {
	kinds := []struct {
		dir  string
		dest *[]fetchedFile
	}{
		{"agents", &result.agentFiles},
		{"projects", &result.projectFiles},
		{"routines", &result.routineFiles},
	}
	plans := make([]kindPlan, 0, len(kinds))
	for _, k := range kinds {
		dir := path.Join(root, k.dir)
		entries, err := w.list(ctx, cfg, dir)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				plans = append(plans, kindPlan{dest: k.dest})
				continue
			}
			return nil, &walkFailure{reason: fmt.Sprintf("list %s: %v", dir, err)}
		}
		var paths []string
		for _, e := range entries {
			if e.IsFile && isYAMLFile(e.Name) {
				paths = append(paths, e.Path)
			}
		}
		plans = append(plans, kindPlan{dest: k.dest, paths: paths})
	}
	return plans, nil
}

// fetchFlatKinds fetches every file planFlatKinds selected. An unavailable
// fetch fails the run; a not-found or unreadable-content fetch is recorded in
// result.unreadable instead.
func (w *walker) fetchFlatKinds(ctx context.Context, cfg *Config, plans []kindPlan, result *walkResult) *walkFailure {
	for _, kp := range plans {
		for _, p := range kp.paths {
			content, unread, ferr := w.fetchContent(ctx, cfg, p)
			if ferr != nil {
				return ferr
			}
			if unread != nil {
				result.unreadable = append(result.unreadable, *unread)
				continue
			}
			*kp.dest = append(*kp.dest, fetchedFile{path: p, content: content})
		}
	}
	return nil
}

// fetchContent fetches one selected file's content. An unavailable fetch
// fails the run; a not-found or unreadable-content fetch is returned as an
// unreadableFile instead of failing.
func (w *walker) fetchContent(ctx context.Context, cfg *Config, filePath string) ([]byte, *unreadableFile, *walkFailure) {
	content, err := w.fetch(ctx, cfg, filePath)
	if err == nil {
		return content, nil, nil
	}
	if errors.Is(err, ErrUnavailable) {
		return nil, nil, &walkFailure{reason: fmt.Sprintf("fetch %s: %v", filePath, err)}
	}
	return nil, &unreadableFile{path: filePath, reason: err.Error()}, nil
}

// planSkills lists the skills/ directory and, for every subdirectory (up to
// the skill cap), selects its SKILL.md and references/ files without
// fetching any content yet. AC-OFFICE-CONFIG-SYNC-002.5: the skill cap is
// evaluated here, before any round-2 request, so a repository exceeding it
// produces the same message on every run.
func (w *walker) planSkills(ctx context.Context, cfg *Config, skillsDir string) ([]skillPlan, *walkFailure) {
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
	plans := make([]skillPlan, 0, len(dirs))
	for _, d := range dirs {
		sp, ferr := w.planOneSkill(ctx, cfg, d)
		if ferr != nil {
			return nil, ferr
		}
		plans = append(plans, sp)
	}
	return plans, nil
}

func (w *walker) planOneSkill(ctx context.Context, cfg *Config, dir DirEntry) (skillPlan, *walkFailure) {
	sp := skillPlan{dirName: dir.Name, dirPath: dir.Path}

	entries, err := w.list(ctx, cfg, dir.Path)
	switch {
	case err != nil && errors.Is(err, ErrNotFound):
		// The parent listing (planSkills) just returned dir as an existing
		// directory entry this same run; a 404 re-listing it now is the
		// directory disappearing mid-run, not proof it never had a
		// SKILL.md. Recorded the same way fetchContent records a
		// disappeared file, so skillDeletionExemptions exempts it instead
		// of the deletion sweep reading "no SKILL.md" as "never managed".
		sp.skillMDUnread = &unreadableFile{path: dir.Path, reason: err.Error()}
	case err != nil:
		return skillPlan{}, &walkFailure{reason: fmt.Sprintf("list %s: %v", dir.Path, err)}
	default:
		for _, e := range entries {
			if e.IsFile && e.Name == skillDefinitionName {
				sp.skillMDPath = e.Path
			}
		}
	}

	refDir := path.Join(dir.Path, "references")
	refEntries, err := w.list(ctx, cfg, refDir)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return skillPlan{}, &walkFailure{reason: fmt.Sprintf("list %s: %v", refDir, err)}
	}
	for _, e := range refEntries {
		if e.IsFile {
			sp.refPaths = append(sp.refPaths, e.Path)
		}
	}
	return sp, nil
}

// fetchSkills fetches every file planSkills selected, per skill directory.
func (w *walker) fetchSkills(ctx context.Context, cfg *Config, plans []skillPlan) ([]skillFiles, *walkFailure) {
	skills := make([]skillFiles, 0, len(plans))
	for _, sp := range plans {
		sf := skillFiles{dirName: sp.dirName, dirPath: sp.dirPath}
		if sp.skillMDUnread != nil {
			sf.skillMDUnread = sp.skillMDUnread
		} else if sp.skillMDPath != "" {
			content, unread, ferr := w.fetchContent(ctx, cfg, sp.skillMDPath)
			if ferr != nil {
				return nil, ferr
			}
			if unread != nil {
				sf.skillMDUnread = unread
			} else {
				f := fetchedFile{path: sp.skillMDPath, content: content}
				sf.skillMD = &f
			}
		}
		for _, rp := range sp.refPaths {
			content, unread, ferr := w.fetchContent(ctx, cfg, rp)
			if ferr != nil {
				return nil, ferr
			}
			if unread != nil {
				sf.unreadableRefs = append(sf.unreadableRefs, *unread)
				continue
			}
			sf.references = append(sf.references, fetchedFile{path: rp, content: content})
		}
		skills = append(skills, sf)
	}
	return skills, nil
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
	sortUnreadable(result.unreadable)
	sort.Slice(result.skills, func(i, j int) bool { return result.skills[i].dirPath < result.skills[j].dirPath })
	for i := range result.skills {
		sortFiles(result.skills[i].references)
		sortUnreadable(result.skills[i].unreadableRefs)
	}
}

// sortUnreadable orders unreadable-file records by path, ascending and
// byte-wise, matching sortFiles for the files that were readable
// (AC-OFFICE-CONFIG-SYNC-004.5a): warning order must not depend on provider
// listing order.
func sortUnreadable(files []unreadableFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
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
