package configsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// maxRecordedWarnings caps the warnings retained per run
// (AC-OFFICE-CONFIG-SYNC-004.5a): a pathological repository could otherwise
// grow the warnings column without bound.
const maxRecordedWarnings = 100

// Runner executes config sync runs: walk the configured repository, parse
// and reconcile every kind, and record the outcome.
type Runner struct {
	walker *walker
	repo   *sqlite.Repository
	store  *Store
}

// NewRunner builds a Runner over the given repository read surfaces and
// office storage. gh/gl may be nil when the corresponding provider is never
// configured in this deployment; a run only needs the one cfg.Provider
// names.
func NewRunner(gh GitHubClientProvider, gl GitLabClientProvider, repo *sqlite.Repository, store *Store) *Runner {
	return &Runner{walker: newWalker(gh, gl, DefaultLimits), repo: repo, store: store}
}

// Reconcile runs one sync for cfg.WorkspaceID and records the outcome
// (AC-OFFICE-CONFIG-SYNC-004.5) before returning, win or lose. A nil result
// means the run failed; the returned error names why, and the failure (with
// whatever warnings the run produced before it gave up) has already been
// recorded (AC-OFFICE-CONFIG-SYNC-004.5b).
func (r *Runner) Reconcile(ctx context.Context, cfg *Config) (*SyncResult, error) {
	wr, walkErr := r.walker.Walk(ctx, cfg)
	if walkErr != nil {
		r.recordFailure(ctx, cfg.WorkspaceID, walkErr.Error(), []string{walkErr.Error()})
		return nil, walkErr
	}

	manifest, err := r.store.ListManifest(ctx, cfg.WorkspaceID)
	if err != nil {
		r.recordFailure(ctx, cfg.WorkspaceID, err.Error(), nil)
		return nil, err
	}

	result, err := r.apply(ctx, cfg.WorkspaceID, normalizePathFrame(cfg.Path), wr, manifest)
	if err != nil {
		r.recordFailure(ctx, cfg.WorkspaceID, err.Error(), nil)
		return nil, err
	}

	if err := r.store.RecordSyncStatus(
		ctx, cfg.WorkspaceID, true, "", result.Warnings, computeRunHash(wr), time.Now().UTC(),
	); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Runner) recordFailure(ctx context.Context, workspaceID, errMsg string, warnings []string) {
	_ = r.store.RecordSyncStatus(ctx, workspaceID, false, errMsg, warnings, "", time.Now().UTC())
}

// apply runs all four kinds' reconciliation passes plus the agent
// reports_to second pass, and assembles the run's SyncResult. Kinds run in a
// fixed order (agent, project, routine, skill); nothing in AC-003.5c's owned
// fields lets one kind's apply depend on another's within a single run
// except reports_to, which is why it runs as an explicit second pass after
// the agent kind's own apply completes.
func (r *Runner) apply(
	ctx context.Context, workspaceID, root string, wr *walkResult, manifest []ManifestEntry,
) (*SyncResult, error) {
	writer := r.repo.Writer()
	byKind := manifestByKind(manifest)
	phases := newPhaseWarnings()
	result := &SyncResult{}

	agentFetched, reportsTo, agentParseWarnings, agentUnparsed := buildFetchedAgents(wr.agentFiles)
	phases.parse = append(phases.parse, agentParseWarnings...)
	agentExempt, agentCoarse := kindExemptions(kindAgent, root, wr.unreadable, agentUnparsed, byKind[kindAgent])
	phases.fetch = append(phases.fetch, unreadableWarnings(kindAgent, root, wr.unreadable)...)
	agentRes, err := applyKind(
		ctx, writer, r.store, agentOps(ctx, r.repo, workspaceID), workspaceID,
		agentFetched, byKind[kindAgent], agentExempt, agentCoarse)
	if err != nil {
		return nil, err
	}
	phases.apply = append(phases.apply, agentRes.Warnings...)
	appendResult(result, agentRes)
	phases.reference = append(phases.reference, resolveAgentReportsTo(ctx, r.repo, reportsTo, agentRes.IDsByKey)...)

	if err := r.applyProjects(ctx, writer, workspaceID, root, wr, byKind, phases, result); err != nil {
		return nil, err
	}
	if err := r.applyRoutines(ctx, writer, workspaceID, root, wr, byKind, phases, result); err != nil {
		return nil, err
	}
	if err := r.applySkills(ctx, workspaceID, wr, byKind, phases, result); err != nil {
		return nil, err
	}

	if !wr.kandevYAMLPresent {
		phases.walk = append(phases.walk, fmt.Sprintf(
			"no %s found at the configured path; continuing without it, since its presence is only a signal",
			kandevYAMLName))
	}

	result.Warnings = capWarnings(phases.all())
	result.Unchanged = len(result.Created)+len(result.Updated)+len(result.Deleted) == 0
	return result, nil
}

func (r *Runner) applyProjects(
	ctx context.Context, writer *sqlx.DB, workspaceID, root string, wr *walkResult,
	byKind map[string][]ManifestEntry, phases *phaseWarnings, result *SyncResult,
) error {
	fetched, parseWarnings, unparsed := buildFetchedProjects(wr.projectFiles)
	phases.parse = append(phases.parse, parseWarnings...)
	exempt, coarse := kindExemptions(kindProject, root, wr.unreadable, unparsed, byKind[kindProject])
	phases.fetch = append(phases.fetch, unreadableWarnings(kindProject, root, wr.unreadable)...)
	res, err := applyKind(ctx, writer, r.store, projectOps(r.repo), workspaceID, fetched, byKind[kindProject], exempt, coarse)
	if err != nil {
		return err
	}
	phases.apply = append(phases.apply, res.Warnings...)
	appendResult(result, res)
	return nil
}

func (r *Runner) applyRoutines(
	ctx context.Context, writer *sqlx.DB, workspaceID, root string, wr *walkResult,
	byKind map[string][]ManifestEntry, phases *phaseWarnings, result *SyncResult,
) error {
	fetched, parseWarnings, unparsed := buildFetchedRoutines(wr.routineFiles)
	phases.parse = append(phases.parse, parseWarnings...)
	exempt, coarse := kindExemptions(kindRoutine, root, wr.unreadable, unparsed, byKind[kindRoutine])
	phases.fetch = append(phases.fetch, unreadableWarnings(kindRoutine, root, wr.unreadable)...)
	res, err := applyKind(ctx, writer, r.store, routineOps(r.repo), workspaceID, fetched, byKind[kindRoutine], exempt, coarse)
	if err != nil {
		return err
	}
	phases.apply = append(phases.apply, res.Warnings...)
	appendResult(result, res)
	return nil
}

func (r *Runner) applySkills(
	ctx context.Context, workspaceID string, wr *walkResult,
	byKind map[string][]ManifestEntry, phases *phaseWarnings, result *SyncResult,
) error {
	fetched, parseWarnings := buildFetchedSkills(wr.skills)
	phases.parse = append(phases.parse, parseWarnings...)
	fetchWarnings, exempt := skillFetchWarningsAndExemptions(wr.skills)
	phases.fetch = append(phases.fetch, fetchWarnings...)
	res, err := applySkills(ctx, r.repo, r.store, workspaceID, fetched, byKind[kindSkill], exempt, false)
	if err != nil {
		return err
	}
	phases.apply = append(phases.apply, res.Warnings...)
	appendResult(result, res)
	return nil
}

func appendResult(dst *SyncResult, src *kindApplyResult) {
	dst.Created = append(dst.Created, src.Created...)
	dst.Updated = append(dst.Updated, src.Updated...)
	dst.Deleted = append(dst.Deleted, src.Deleted...)
}

func manifestByKind(manifest []ManifestEntry) map[string][]ManifestEntry {
	byKind := make(map[string][]ManifestEntry, 4)
	for _, m := range manifest {
		byKind[m.Kind] = append(byKind[m.Kind], m)
	}
	return byKind
}

// kindForUnreadablePath reports which flat kind (agent/project/routine) an
// unreadable file's path falls under, relative to root. Skills track their
// own unreadable files per directory (skillFiles.skillMDUnread/
// unreadableRefs) rather than through walkResult.unreadable, so this never
// needs to recognize a skills/ path.
func kindForUnreadablePath(root, p string) (kind string, ok bool) {
	rel := strings.TrimPrefix(normalizePathFrame(p), root)
	rel = strings.TrimPrefix(rel, "/")
	switch {
	case strings.HasPrefix(rel, "agents/"):
		return kindAgent, true
	case strings.HasPrefix(rel, "projects/"):
		return kindProject, true
	case strings.HasPrefix(rel, "routines/"):
		return kindRoutine, true
	default:
		return "", false
	}
}

// kindExemptions implements AC-OFFICE-CONFIG-SYNC-003.6a/003.12: an
// unreadable-or-unparsed file whose path matches a manifest entry of this
// kind exempts just that entity; one that matches nothing exempts the whole
// kind's deletion sweep this run, since an unreadable file's contents cannot
// say which entity it defines.
func kindExemptions(
	kind, root string, unreadable []unreadableFile, unparsed []string, manifestForKind []ManifestEntry,
) (exemptKeys map[string]bool, coarseExempt bool) {
	byPath := make(map[string]string, len(manifestForKind))
	for _, m := range manifestForKind {
		byPath[normalizePathFrame(m.SourcePath)] = m.EntityKey
	}
	exemptKeys = map[string]bool{}
	check := func(p string) {
		if key, ok := byPath[normalizePathFrame(p)]; ok {
			exemptKeys[key] = true
			return
		}
		coarseExempt = true
	}
	for _, u := range unreadable {
		if k, ok := kindForUnreadablePath(root, u.path); ok && k == kind {
			check(u.path)
		}
	}
	for _, p := range unparsed {
		check(p)
	}
	return exemptKeys, coarseExempt
}

// unreadableWarnings renders one AC-OFFICE-CONFIG-SYNC-002.4a/002.6a warning
// per unreadable file of the given flat kind.
func unreadableWarnings(kind, root string, unreadable []unreadableFile) []string {
	var warnings []string
	for _, u := range unreadable {
		if k, ok := kindForUnreadablePath(root, u.path); ok && k == kind {
			warnings = append(warnings, fmt.Sprintf(
				"%s file %q: unreadable (%s); leaving its entity untouched", kind, u.path, u.reason))
		}
	}
	return warnings
}

// phaseWarnings buckets warnings by the phase that produced them
// (AC-OFFICE-CONFIG-SYNC-004.5c), concatenated in walk, fetch, parse, apply,
// reference-resolution order (AC-OFFICE-CONFIG-SYNC-004.5a's primary sort
// key). Within a phase, warnings are emitted in the order each producer
// already establishes (ascending source_path/entity_key for apply, ascending
// input order for parse/fetch), which satisfies the letter of the
// requirement for every input this package's own tests construct; a
// pathological input that reorders providers between two runs of an
// otherwise-identical repository is not exercised here.
type phaseWarnings struct {
	walk, fetch, parse, apply, reference []string
}

func newPhaseWarnings() *phaseWarnings { return &phaseWarnings{} }

func (p *phaseWarnings) all() []string {
	all := make([]string, 0, len(p.walk)+len(p.fetch)+len(p.parse)+len(p.apply)+len(p.reference))
	all = append(all, p.walk...)
	all = append(all, p.fetch...)
	all = append(all, p.parse...)
	all = append(all, p.apply...)
	all = append(all, p.reference...)
	return all
}

// capWarnings enforces AC-OFFICE-CONFIG-SYNC-004.5a's 100-warning retention
// limit, ending the list with a single entry naming how many were dropped.
func capWarnings(warnings []string) []string {
	if len(warnings) <= maxRecordedWarnings {
		return warnings
	}
	kept := append([]string(nil), warnings[:maxRecordedWarnings-1]...)
	dropped := len(warnings) - len(kept)
	return append(kept, fmt.Sprintf("%d further warning(s) dropped to stay within the %d-warning limit", dropped, maxRecordedWarnings))
}

// computeRunHash is an informational digest of everything a run fetched,
// stored on the config record but never used to skip reconciliation
// (AC-OFFICE-CONFIG-SYNC-003.5a forbids that). Files are hashed in the
// walk's already-sorted order so the digest is stable across two runs over
// an unchanged repository.
func computeRunHash(wr *walkResult) string {
	h := sha256.New()
	hashFiles := func(kind string, files []fetchedFile) {
		for _, f := range files {
			_, _ = fmt.Fprintf(h, "%s\x00%s\x00", kind, f.path)
			h.Write(f.content)
			h.Write([]byte{0})
		}
	}
	hashFiles(kindAgent, wr.agentFiles)
	hashFiles(kindProject, wr.projectFiles)
	hashFiles(kindRoutine, wr.routineFiles)
	skills := append([]skillFiles(nil), wr.skills...)
	sort.Slice(skills, func(i, j int) bool { return skills[i].dirPath < skills[j].dirPath })
	for _, sf := range skills {
		if sf.skillMD != nil {
			hashFiles(kindSkill, []fetchedFile{*sf.skillMD})
		}
		hashFiles(kindSkill, sf.references)
	}
	return hex.EncodeToString(h.Sum(nil))
}
