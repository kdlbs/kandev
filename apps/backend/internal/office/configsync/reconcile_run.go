package configsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// maxRecordedWarnings caps the warnings retained per run
// (AC-OFFICE-CONFIG-SYNC-004.5a): a pathological repository could otherwise
// grow the warnings column without bound.
const maxRecordedWarnings = 100

// recordWriteTimeout bounds the status-recording write derived from a
// caller context that may already be canceled or past its deadline.
const recordWriteTimeout = 5 * time.Second

// recordWriteContext detaches the status-recording write from ctx's own
// cancellation and deadline. By the time a run's outcome is known, ctx may
// already be past the AC-OFFICE-CONFIG-SYNC-004.4a run deadline or canceled
// by its caller — reusing it for the write that records that very outcome
// would make the write fail immediately and silently, leaving the deadline
// or cancellation unrecorded. context.WithoutCancel keeps ctx's values
// (tracing, etc.) without its Done/Err.
func recordWriteContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), recordWriteTimeout)
}

// SessionTerminator cascades termination of an office agent's task sessions
// when config sync's deletion sweep removes the agent instance, mirroring
// AgentService.DeleteAgentInstance's own cascade for the manual delete path.
// Declared locally, as agents.SessionTerminator's signature but not its
// type, so this package gains no import to office/agents; the composition
// root wires the real implementation via Runner.SetSessionTerminator. Optional
// — when nil, a deletion sweep proceeds without flipping any session rows.
type SessionTerminator interface {
	TerminateAllForAgent(ctx context.Context, agentInstanceID, reason string) error
}

// Runner executes config sync runs: walk the configured repository, parse
// and reconcile every kind, and record the outcome.
type Runner struct {
	walker      *walker
	repo        *sqlite.Repository
	store       *Store
	sessionTerm SessionTerminator
}

// NewRunner builds a Runner over the given repository read surfaces and
// office storage. gh/gl may be nil when the corresponding provider is never
// configured in this deployment; a run only needs the one cfg.Provider
// names.
func NewRunner(gh GitHubClientProvider, gl GitLabClientProvider, repo *sqlite.Repository, store *Store) *Runner {
	return &Runner{walker: newWalker(gh, gl, DefaultLimits), repo: repo, store: store}
}

// SetSessionTerminator wires the office session terminator. Called by the
// composition root once the office/agents service (which owns the real
// implementation) is constructed; optional, so wiring order the other way
// (Runner before agents) never blocks construction.
func (r *Runner) SetSessionTerminator(t SessionTerminator) {
	r.sessionTerm = t
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

	result, applyErr := r.apply(ctx, cfg.WorkspaceID, normalizePathFrame(cfg.Path), wr, manifest)
	if applyErr != nil {
		var warnings []string
		if result != nil {
			warnings = result.Warnings
		}
		r.recordFailure(ctx, cfg.WorkspaceID, applyErr.Error(), capWarnings(append(warnings, applyErr.Error())))
		return nil, applyErr
	}

	writeCtx, cancel := recordWriteContext(ctx)
	defer cancel()
	if err := r.store.RecordSyncStatus(
		writeCtx, cfg.WorkspaceID, true, "", result.Warnings, computeRunHash(wr), time.Now().UTC(),
	); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Runner) recordFailure(ctx context.Context, workspaceID, errMsg string, warnings []string) {
	writeCtx, cancel := recordWriteContext(ctx)
	defer cancel()
	_ = r.store.RecordSyncStatus(writeCtx, workspaceID, false, errMsg, warnings, "", time.Now().UTC())
}

// kindsFetch is every kind's collision-resolved fetched set and deletion-
// sweep exemptions for one run, computed once up front so the forward and
// reverse passes (AC-OFFICE-CONFIG-SYNC-003.9) can each iterate kinds in
// their own required order without recomputing anything.
type kindsFetch struct {
	skill         []fetchedSkill
	skillExempt   map[string]bool
	skillCoarse   bool
	agent         []fetchedEntity[sqlite.AgentInstanceConfigFields]
	reportsTo     map[string]string
	agentExempt   map[string]bool
	agentCoarse   bool
	project       []fetchedEntity[projectProjection]
	projectExempt map[string]bool
	projectCoarse bool
	routine       []fetchedEntity[routineProjection]
	routineExempt map[string]bool
	routineCoarse bool
}

// kindsResult holds each kind's kindApplyResult across the forward and
// reverse passes, so the reverse pass can append deletions to the same
// object the forward pass's creates/updates were recorded on.
type kindsResult struct {
	skill, agent, project, routine *kindApplyResult
}

// buildKindsFetch parses every kind's files and computes its deletion-sweep
// exemptions, recording walk/fetch/parse-phase warnings as it goes
// (AC-OFFICE-CONFIG-SYNC-004.5c).
func buildKindsFetch(root string, wr *walkResult, byKind map[string][]ManifestEntry, phases *phaseWarnings) kindsFetch {
	var kf kindsFetch

	var skillParseWarnings, skillUnparsed []string
	kf.skill, skillParseWarnings, skillUnparsed = buildFetchedSkills(wr.skills)
	kf.skillExempt, kf.skillCoarse = skillDeletionExemptions(wr.skills, skillUnparsed, byKind[kindSkill])
	phases.walk = append(phases.walk, skillMissingDefinitionWarnings(wr.skills)...)
	phases.parse = append(phases.parse, skillParseWarnings...)
	phases.fetch = append(phases.fetch, skillFetchWarnings(wr.skills)...)

	var agentParseWarnings, agentUnparsed []string
	kf.agent, kf.reportsTo, agentParseWarnings, agentUnparsed = buildFetchedAgents(wr.agentFiles)
	kf.agentExempt, kf.agentCoarse = kindExemptions(kindAgent, root, wr.unreadable, agentUnparsed, byKind[kindAgent])
	phases.parse = append(phases.parse, agentParseWarnings...)
	phases.fetch = append(phases.fetch, unreadableWarnings(kindAgent, root, wr.unreadable)...)

	var projectParseWarnings, projectUnparsed []string
	kf.project, projectParseWarnings, projectUnparsed = buildFetchedProjects(wr.projectFiles)
	kf.projectExempt, kf.projectCoarse = kindExemptions(kindProject, root, wr.unreadable, projectUnparsed, byKind[kindProject])
	phases.parse = append(phases.parse, projectParseWarnings...)
	phases.fetch = append(phases.fetch, unreadableWarnings(kindProject, root, wr.unreadable)...)

	var routineParseWarnings, routineUnparsed []string
	kf.routine, routineParseWarnings, routineUnparsed = buildFetchedRoutines(wr.routineFiles)
	kf.routineExempt, kf.routineCoarse = kindExemptions(kindRoutine, root, wr.unreadable, routineUnparsed, byKind[kindRoutine])
	phases.parse = append(phases.parse, routineParseWarnings...)
	phases.fetch = append(phases.fetch, unreadableWarnings(kindRoutine, root, wr.unreadable)...)

	return kf
}

// apply runs every kind's reconciliation pass and assembles the run's
// SyncResult. AC-OFFICE-CONFIG-SYNC-003.9 fixes the kind order for
// determinism (no kind's definition actually depends on another's, per
// 003.9a): creates/updates run skills, agents, projects, routines; deletions
// run in reverse.
func (r *Runner) apply(
	ctx context.Context, workspaceID, root string, wr *walkResult, manifest []ManifestEntry,
) (*SyncResult, error) {
	byKind := manifestByKind(manifest)
	phases := newPhaseWarnings()
	kf := buildKindsFetch(root, wr, byKind, phases)

	kr, err := r.forwardPass(ctx, workspaceID, kf, byKind, phases)
	if err != nil {
		return &SyncResult{Warnings: partialWarnings(phases, kr)}, err
	}
	if err := r.reversePass(ctx, workspaceID, kf, byKind, kr); err != nil {
		return &SyncResult{Warnings: partialWarnings(phases, kr)}, err
	}
	r.terminateDeletedAgentSessions(ctx, kr.agent)

	result := &SyncResult{}
	// AC-OFFICE-CONFIG-SYNC-004.5a's within-phase tiebreak for warnings naming
	// no file orders by the (kind, key) of the entity they name; kind order
	// here is alphabetical (agent, project, routine, skill), independent of
	// the 003.9 apply/delete kind order above.
	for _, kres := range []*kindApplyResult{kr.agent, kr.project, kr.routine, kr.skill} {
		phases.apply = append(phases.apply, kres.Warnings...)
		appendResult(result, kres)
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

// partialWarnings assembles the warnings accumulated so far when
// forwardPass or reversePass fails mid-apply (AC-OFFICE-CONFIG-SYNC-004.5b):
// phases already holds every walk/fetch/parse-phase warning, and kr — even
// when forwardPass returns it alongside an error — holds whichever kinds'
// apply-phase warnings were produced before the failing kind. kr is nil only
// when the very first kind (skills) failed before producing a result.
func partialWarnings(phases *phaseWarnings, kr *kindsResult) []string {
	if kr != nil {
		for _, kres := range []*kindApplyResult{kr.agent, kr.project, kr.routine, kr.skill} {
			if kres != nil {
				phases.apply = append(phases.apply, kres.Warnings...)
			}
		}
	}
	return capWarnings(phases.all())
}

// forwardPass runs every kind's creates/updates in AC-OFFICE-CONFIG-SYNC-003.9
// order (skills, agents, projects, routines), then resolves agent reports_to
// (AC-OFFICE-CONFIG-SYNC-003.9b) — the one within-kind reference this run
// must settle before anything else depends on agent IDs being final.
func (r *Runner) forwardPass(
	ctx context.Context, workspaceID string, kf kindsFetch, byKind map[string][]ManifestEntry, phases *phaseWarnings,
) (*kindsResult, error) {
	writer := r.repo.Writer()
	kr := &kindsResult{}

	var err error
	if kr.skill, err = applySkillsCreatesOnly(ctx, r.repo, r.store, workspaceID, kf.skill, byKind[kindSkill]); err != nil {
		return kr, err
	}
	if kr.agent, err = applyKindCreatesOnly(ctx, writer, r.store, agentOps(ctx, r.repo, workspaceID), workspaceID, kf.agent, byKind[kindAgent]); err != nil {
		return kr, err
	}
	reportsToWarnings, reportsToErr := resolveAgentReportsTo(
		ctx, r.repo, kf.reportsTo, kr.agent.IDsByKey, unionKeySets(kf.agentExempt, kr.agent.ForeignKeys),
	)
	phases.reference = append(phases.reference, reportsToWarnings...)
	if reportsToErr != nil {
		return kr, reportsToErr
	}
	if kr.project, err = applyKindCreatesOnly(ctx, writer, r.store, projectOps(r.repo), workspaceID, kf.project, byKind[kindProject]); err != nil {
		return kr, err
	}
	if kr.routine, err = applyKindCreatesOnly(ctx, writer, r.store, routineOps(r.repo), workspaceID, kf.routine, byKind[kindRoutine]); err != nil {
		return kr, err
	}
	return kr, nil
}

// reversePass runs every kind's deletions in reverse kind order (routines,
// projects, agents, skills), appending to the kindApplyResults forwardPass
// already produced.
func (r *Runner) reversePass(ctx context.Context, workspaceID string, kf kindsFetch, byKind map[string][]ManifestEntry, kr *kindsResult) error {
	writer := r.repo.Writer()

	if err := applyKindDeletesOnly(
		ctx, writer, r.store, routineOps(r.repo), workspaceID, kf.routine, byKind[kindRoutine], kf.routineExempt, kf.routineCoarse, kr.routine,
	); err != nil {
		return err
	}
	if err := applyKindDeletesOnly(
		ctx, writer, r.store, projectOps(r.repo), workspaceID, kf.project, byKind[kindProject], kf.projectExempt, kf.projectCoarse, kr.project,
	); err != nil {
		return err
	}
	if err := applyKindDeletesOnly(
		ctx, writer, r.store, agentOps(ctx, r.repo, workspaceID), workspaceID, kf.agent, byKind[kindAgent], kf.agentExempt, kf.agentCoarse, kr.agent,
	); err != nil {
		return err
	}
	return applySkillsDeletesOnly(ctx, r.repo, r.store, workspaceID, kf.skill, byKind[kindSkill], kf.skillExempt, kf.skillCoarse, kr.skill)
}

// terminateDeletedAgentSessions cascades session termination for every
// agent this run's deletion sweep removed, mirroring
// AgentService.DeleteAgentInstance's cascade for the manual delete path
// (the deletion sweep writes to the repository directly and does not go
// through that method). Runs after reversePass's transactions have all
// committed, so a concurrent EnsureSessionForAgent cannot resurrect a row
// this call is about to terminate sessions for. A termination failure is
// folded into the agent kind's warnings rather than failing the run: the
// deletion itself already committed, and the run's other three kinds still
// need to report their own outcomes.
func (r *Runner) terminateDeletedAgentSessions(ctx context.Context, agentResult *kindApplyResult) {
	if r.sessionTerm == nil {
		return
	}
	for _, id := range agentResult.DeletedIDs {
		if err := r.sessionTerm.TerminateAllForAgent(ctx, id, "agent_instance_deleted"); err != nil {
			agentResult.Warnings = append(agentResult.Warnings, fmt.Sprintf(
				"agent %q: deleted, but terminating its sessions failed: %v", id, err))
		}
	}
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

// unionKeySets returns a new set containing every key present in either
// input, leaving both inputs unmodified.
func unionKeySets(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
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
