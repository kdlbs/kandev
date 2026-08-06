package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// mrAutoFix* mirror the GitHub CI-automation vocabulary
// (event_handlers_github_ci_automation.go) adapted to GitLab nouns: pipeline
// jobs instead of check runs, discussion notes instead of PR review
// comments. Named distinctly from the ciAutomation* constants shared via
// ci_automation_dispatch.go (C5) so the two providers' vocabularies stay
// visually distinct in a diff even where the shapes are parallel.
const (
	mrAutoFixKind          = "mr_auto_fix"
	mrAutoFixFeedbackToken = "{{mr.feedback}}"
	mrAutoFixMention       = "@mr-auto-fix"
)

var mrAutoFixSnapshotFieldReplacer = strings.NewReplacer("\r", " ", "\n", " ", "<", "", ">", "")

// mrAutoFixCheckpoint is the JSON shape persisted in
// TaskMRLifecycleState.LastFixCheckpointJSON, mirroring GitHub's
// ciAutomationCheckpoint.
type mrAutoFixCheckpoint struct {
	FailedJobs []mrAutoFixJobSnapshot  `json:"failed_jobs"`
	Notes      []mrAutoFixNoteSnapshot `json:"notes"`
}

type mrAutoFixJobSnapshot struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	WebURL string `json:"web_url,omitempty"`
}

// mrAutoFixNoteSnapshot is one note within an unresolved discussion. Path
// and Line come from the discussion (GitLab positions a discussion, not
// each note within it).
type mrAutoFixNoteSnapshot struct {
	ID   int64  `json:"id"`
	Body string `json:"body,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// mrAutoFixCanRun reports whether auto-fix may act on an MR in this state.
// Unlike lifecycle notifications (which per #2125's ADR treat "locked" as
// non-terminal), auto-fix treats merged/closed/locked as all terminal
// (AC12): there is nothing left to fix once the MR can no longer receive
// commits through the normal flow.
func mrAutoFixCanRun(state string) bool {
	return state != gitlabMRStateMerged && state != gitlabMRStateClosed && state != gitlabMRStateLocked
}

// mrAutomationReadyToMerge is Q3's auto-merge readiness gate: GitLab's own
// detailed_merge_status verdict (15.6+, collapses multi-rule approvals
// server-side) when present, falling back to the coarser merge_status
// otherwise — plus explicit gates we check ourselves rather than trusting
// a signal GitLab didn't confirm. Mirrors GitHub's ciAutomationReadyToMerge.
func mrAutomationReadyToMerge(snapshot *gitlab.MRAutomationSnapshot) bool {
	if snapshot == nil || snapshot.MR == nil {
		return false
	}
	mr := snapshot.MR
	if mr.State != gitlabMRStateOpen || mr.Draft {
		return false
	}
	if snapshot.PipelineStatus != ciAutomationCheckSuccess {
		return false
	}
	if snapshot.UnresolvedDiscussions > 0 {
		return false
	}
	return mrMergeStatusReady(mr)
}

// mrMergeStatusReady evaluates GitLab's own merge-readiness verdict,
// preferring detailed_merge_status (15.6+) and falling back to the coarser
// merge_status on hosts too old to report it (AC15).
func mrMergeStatusReady(mr *gitlab.MR) bool {
	if mr.DetailedMergeStatus != "" {
		return mr.DetailedMergeStatus == "mergeable"
	}
	return mr.MergeStatus == "can_be_merged"
}

// handleTaskMRCIAutomation runs auto-fix then, unless auto-fix just
// dispatched a fix (autoFixBlockedMerge — mirrors GitHub's behaviour),
// auto-merge. Both are no-ops when their respective switch is off. Errors
// are recorded on the MR's checkpoint rather than returned, matching the
// existing lifecycle evaluator's fail-soft-per-MR contract — one broken MR
// must not block the caller's loop.
func (s *Service) handleTaskMRCIAutomation(ctx context.Context, mr *gitlab.TaskMR, options *gitlab.TaskMRAutomationResponse) {
	if !options.AutoFixEnabled && !options.AutoMergeEnabled {
		return
	}
	if !mrAutoFixCanRun(mr.State) {
		return
	}
	snapshot, err := s.gitlabMRAutomation.GetMRAutomationSnapshot(ctx, options.WorkspaceID, mr.Host, mr.ProjectPath, mr.MRIID)
	if err != nil {
		s.recordMRAutomationError(ctx, mr, fmt.Errorf("fetch MR automation snapshot: %w", err))
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
		return
	}
	// Re-check against the snapshot's live GitLab state, not the possibly
	// stale gitlab_task_mrs row above: the immediate options-updated trigger
	// (as opposed to the ~1-minute poller sweep) can fire before the next
	// lightweight sync catches up, so mr.State can still read "open" for an
	// MR GitLab already reports merged/closed/locked.
	if snapshot.MR == nil || !mrAutoFixCanRun(snapshot.MR.State) {
		return
	}
	autoFixBlockedMerge := false
	if options.AutoFixEnabled {
		autoFixBlockedMerge = s.handleTaskMRCIAutoFix(ctx, mr, options, snapshot)
	}
	if !autoFixBlockedMerge && options.AutoMergeEnabled && mrAutomationReadyToMerge(snapshot) {
		s.handleTaskMRCIAutoMerge(ctx, mr, options.WorkspaceID, snapshot)
	}
}

// handleTaskMRCIAutoFix computes the delta against the last-seen
// checkpoint and, when non-empty, dispatches a fix prompt. Returns true
// when the caller's same evaluation pass must not proceed to auto-merge —
// mirrors GitHub's handleTaskPRCIAutoFix return contract.
func (s *Service) handleTaskMRCIAutoFix(
	ctx context.Context, mr *gitlab.TaskMR, options *gitlab.TaskMRAutomationResponse, snapshot *gitlab.MRAutomationSnapshot,
) bool {
	state, err := s.gitlabMRAutomation.GetTaskMRLifecycleState(ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
	if err != nil {
		s.recordMRAutomationError(ctx, mr, fmt.Errorf("load MR automation state: %w", err))
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
		return true
	}
	// Exhausted auto-fix must not block auto-merge forever: once the round
	// cap is spent, a human may still fix CI by hand and leave the MR
	// genuinely ready. Blocking unconditionally here stranded auto-merge
	// permanently (recoverable only by toggling auto-fix off), so defer to
	// the readiness gate exactly as GitHub's handleTaskPRCIAutoFix does.
	if state != nil && state.AutoFixExhaustedAt != nil {
		return !mrAutomationReadyToMerge(snapshot)
	}
	if !mrAutoFixChecksSettled(snapshot) {
		return false
	}
	previous := decodeMRAutoFixCheckpoint(state)
	delta := mrAutoFixBuildDelta(snapshot, previous)
	checkpoint := mrAutoFixBuildDelta(snapshot, mrAutoFixCheckpoint{})
	checkpointJSON, signature := encodeMRAutoFixCheckpoint(checkpoint)
	if len(delta.FailedJobs) == 0 && len(delta.Notes) == 0 {
		s.handleTaskMRCIAutoFixEmptyDelta(ctx, mr, state, previous, signature, checkpointJSON)
		return false
	}
	if state != nil && state.LastFixSignature == signature {
		return mrAutoFixDuplicateAttemptBlocksMerge(state)
	}
	allowNewRound := !mrAutoFixRoundsExhausted(state)
	prompt := mrAutoFixRenderPrompt(options.EffectiveAutoFixPrompt, mr, delta)
	session, err := s.resolveMRAutoFixSession(ctx, mr.TaskID, state)
	if err != nil || session == nil {
		return s.handleMRAutoFixWithoutSession(ctx, mr, allowNewRound)
	}
	prompt = s.expandPromptReferences(ctx, prompt, session.IsPassthrough)
	result, err := s.dispatchCIAutomationPrompt(ctx, session, ciAutomationDispatchParams{
		ChatPrompt:    mrAutoFixChatPrompt(prompt),
		CoalesceKey:   mrAutoFixCoalesceKey(mr),
		Metadata:      mrAutoFixMessageMetadata(mr, signature),
		AllowNewRound: allowNewRound,
	})
	if errors.Is(err, errCIAutoFixRoundCapReached) {
		s.markMRAutoFixExhausted(ctx, mr)
		return true
	}
	if err != nil {
		s.recordMRAutomationError(ctx, mr, err)
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
		return true
	}
	if err := s.gitlabMRAutomation.RecordTaskMRFixAttempt(context.WithoutCancel(ctx), gitlab.TaskMRFixAttempt{
		TaskID: mr.TaskID, RepositoryID: mr.RepositoryID, ProjectPath: mr.ProjectPath, MRIID: mr.MRIID,
		Signature: signature, CheckpointJSON: checkpointJSON, SessionID: session.ID,
		EnqueuedAt: time.Now().UTC(), IncrementRound: result.consumesRound(),
	}); err != nil {
		s.logger.Debug("record MR auto-fix attempt failed", zap.String("task_id", mr.TaskID), zap.Error(err))
	} else {
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
	}
	return true
}

func (s *Service) handleMRAutoFixWithoutSession(ctx context.Context, mr *gitlab.TaskMR, allowNewRound bool) bool {
	if !allowNewRound {
		s.markMRAutoFixExhausted(ctx, mr)
		return true
	}
	s.recordMRAutomationError(ctx, mr, errors.New("no promptable task session for MR auto-fix"))
	s.publishTaskMRAutomationState(ctx, mr.TaskID)
	return true
}

func (s *Service) handleTaskMRCIAutoFixEmptyDelta(
	ctx context.Context, mr *gitlab.TaskMR, state *gitlab.TaskMRLifecycleState,
	previous mrAutoFixCheckpoint, signature, checkpointJSON string,
) {
	if state != nil && len(previous.FailedJobs)+len(previous.Notes) > 0 {
		if err := s.gitlabMRAutomation.RefreshTaskMRFixCheckpoint(
			context.WithoutCancel(ctx), mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, signature, checkpointJSON,
		); err != nil {
			s.logger.Debug("record MR auto-fix checkpoint refresh failed", zap.String("task_id", mr.TaskID), zap.Error(err))
		}
	}
}

// handleTaskMRCIAutoMerge merges the MR when its readiness signature hasn't
// already been attempted (dedupe against a re-evaluation of unchanged
// state). Mirrors GitHub's handleTaskPRCIAutoMerge.
func (s *Service) handleTaskMRCIAutoMerge(ctx context.Context, mr *gitlab.TaskMR, workspaceID string, snapshot *gitlab.MRAutomationSnapshot) {
	signature := mrAutoMergeSignature(snapshot)
	state, err := s.gitlabMRAutomation.GetTaskMRLifecycleState(ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
	if err != nil {
		s.logger.Debug("load MR automation merge state failed; attempting merge without dedupe",
			zap.String("task_id", mr.TaskID), zap.Error(err))
	} else if state != nil && state.LastMergeSignature == signature {
		return
	}
	if _, err := s.gitlabMRAutomation.MergeMRForAutomation(ctx, workspaceID, mr.Host, mr.ProjectPath, mr.MRIID); err != nil {
		s.recordMRAutomationError(ctx, mr, fmt.Errorf("merge MR: %w", err))
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
		return
	}
	_ = s.gitlabMRAutomation.RecordTaskMRMergeAttempt(context.WithoutCancel(ctx), gitlab.TaskMRMergeAttempt{
		TaskID: mr.TaskID, RepositoryID: mr.RepositoryID, ProjectPath: mr.ProjectPath, MRIID: mr.MRIID,
		Signature: signature, AttemptedAt: time.Now().UTC(),
	})
	_ = s.gitlabMRAutomation.ClearTaskMRAutomationError(context.WithoutCancel(ctx), mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
	s.publishTaskMRAutomationState(ctx, mr.TaskID)
}

// resolveMRAutoFixSession adapts the provider-agnostic
// resolveAutoFixSession (ci_automation_dispatch.go, C5) to GitLab's
// checkpoint state shape.
func (s *Service) resolveMRAutoFixSession(ctx context.Context, taskID string, state *gitlab.TaskMRLifecycleState) (*models.TaskSession, error) {
	var lastFixSessionID *string
	if state != nil {
		lastFixSessionID = state.LastFixSessionID
	}
	return s.resolveAutoFixSession(ctx, taskID, lastFixSessionID)
}

// mrAutoFixChecksSettled is AC9's gate: a pipeline still running/pending
// must not trigger a dispatch on a partial picture, even when a job has
// already failed.
func mrAutoFixChecksSettled(snapshot *gitlab.MRAutomationSnapshot) bool {
	if snapshot == nil {
		return true
	}
	switch snapshot.PipelineStatus {
	case "", ciAutomationCheckSuccess, "failed", "canceled", "skipped":
		return true
	default: // running, pending, created, manual, scheduled, ...
		return false
	}
}

func mrAutoFixBuildDelta(snapshot *gitlab.MRAutomationSnapshot, previous mrAutoFixCheckpoint) mrAutoFixCheckpoint {
	var delta mrAutoFixCheckpoint
	if snapshot == nil {
		return delta
	}
	prevJobs := make(map[string]struct{}, len(previous.FailedJobs))
	for _, job := range previous.FailedJobs {
		prevJobs[mrAutoFixJobKey(job)] = struct{}{}
	}
	for _, job := range snapshot.FailingJobs {
		snap := mrAutoFixJobSnapshot{Name: job.Name, Status: job.Status, WebURL: job.WebURL}
		if _, seen := prevJobs[mrAutoFixJobKey(snap)]; !seen {
			delta.FailedJobs = append(delta.FailedJobs, snap)
		}
	}
	prevNotes := make(map[int64]mrAutoFixNoteSnapshot, len(previous.Notes))
	for _, note := range previous.Notes {
		prevNotes[note.ID] = note
	}
	for _, discussion := range snapshot.Discussions {
		if !discussion.Resolvable || discussion.Resolved {
			continue
		}
		for _, note := range discussion.Notes {
			snap := mrAutoFixNoteSnapshot{ID: note.ID, Body: note.Body, Path: discussion.Path, Line: discussion.Line}
			if prev, seen := prevNotes[note.ID]; seen && prev == snap {
				continue
			}
			delta.Notes = append(delta.Notes, snap)
		}
	}
	return delta
}

func mrAutoFixJobKey(job mrAutoFixJobSnapshot) string {
	return job.Name + "|" + job.Status + "|" + job.WebURL
}

func mrAutoFixRoundsExhausted(state *gitlab.TaskMRLifecycleState) bool {
	if state == nil {
		return false
	}
	return state.AutoFixExhaustedAt != nil || state.AutoFixRoundCount >= gitlab.TaskMRAutoFixMaxRounds
}

// mrAutoFixDuplicateAttemptBlocksMerge mirrors GitHub's
// ciAutomationDuplicateFixAttemptBlocksMerge: a fix dispatched within the
// last hour for an unchanged signature still blocks the same pass's
// auto-merge, even though no new prompt was sent this round.
func mrAutoFixDuplicateAttemptBlocksMerge(state *gitlab.TaskMRLifecycleState) bool {
	return mrAutoFixDuplicateAttemptBlocksMergeAt(state, time.Now())
}

func mrAutoFixDuplicateAttemptBlocksMergeAt(state *gitlab.TaskMRLifecycleState, now time.Time) bool {
	if state == nil || state.LastFixEnqueuedAt == nil {
		return false
	}
	return now.Sub(*state.LastFixEnqueuedAt) <= time.Hour
}

func (s *Service) markMRAutoFixExhausted(ctx context.Context, mr *gitlab.TaskMR) {
	if mr == nil {
		return
	}
	message := fmt.Sprintf("MR auto-fix paused after %d rounds for this merge request", gitlab.TaskMRAutoFixMaxRounds)
	s.logger.Warn("MR automation auto-fix round cap reached",
		zap.String("task_id", mr.TaskID), zap.String("repository_id", mr.RepositoryID),
		zap.String("project_path", mr.ProjectPath), zap.Int("mr_iid", mr.MRIID),
		zap.Int("max_rounds", gitlab.TaskMRAutoFixMaxRounds))
	if err := s.gitlabMRAutomation.MarkTaskMRAutoFixExhausted(
		context.WithoutCancel(ctx), mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, message,
	); err != nil {
		s.logger.Debug("record MR auto-fix exhaustion failed", zap.String("task_id", mr.TaskID), zap.Error(err))
		return
	}
	s.publishTaskMRAutomationState(ctx, mr.TaskID)
}

func mrAutoFixCoalesceKey(mr *gitlab.TaskMR) string {
	if mr == nil {
		return "gitlab-mr-auto-fix||0"
	}
	return fmt.Sprintf("gitlab-mr-auto-fix|%s|%s|%s|%d", mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
}

func mrAutoFixChatPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return mrAutoFixMention
	}
	return mrAutoFixMention + "\n\n" + prompt
}

func mrAutoFixMessageMetadata(mr *gitlab.TaskMR, signature string) map[string]interface{} {
	meta := NewUserMessageMeta().WithAutoStart(true).ToMap()
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta[mrAutomationMetaOrigin] = mrAutomationOrigin
	meta["automation_kind"] = mrAutoFixKind
	meta["mr_auto_fix_key"] = mrAutoFixCoalesceKey(mr)
	meta["feedback_signature"] = signature
	if mr != nil {
		meta[mrAutomationMetaRepository] = mr.RepositoryID
		meta[mrAutomationMetaProject] = mr.ProjectPath
		meta[mrAutomationMetaMRIID] = mr.MRIID
		meta[mrAutomationMetaMRURL] = mr.MRURL
	}
	return meta
}

func mrAutoMergeSignature(snapshot *gitlab.MRAutomationSnapshot) string {
	if snapshot == nil || snapshot.MR == nil {
		return ""
	}
	mr := snapshot.MR
	payload := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%d",
		mr.ProjectPath, mr.IID, mr.HeadSHA, snapshot.PipelineStatus, mr.MergeStatus, mr.DetailedMergeStatus, snapshot.UnresolvedDiscussions)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func mrAutoFixRenderPrompt(base string, mr *gitlab.TaskMR, delta mrAutoFixCheckpoint) string {
	if base = strings.TrimSpace(base); base == "" {
		return ""
	}
	return mrAutoFixRenderPromptTemplate(base, mrAutoFixRenderSnapshot(mr, delta))
}

func mrAutoFixRenderPromptTemplate(base, snapshot string) string {
	if !strings.Contains(base, mrAutoFixFeedbackToken) {
		return sysprompt.Wrap(base)
	}
	segments := strings.Split(base, mrAutoFixFeedbackToken)
	parts := make([]string, 0, len(segments)*2)
	for i, segment := range segments {
		if segment = strings.TrimSpace(segment); segment != "" {
			parts = append(parts, sysprompt.Wrap(segment))
		}
		if i < len(segments)-1 && strings.TrimSpace(snapshot) != "" {
			parts = append(parts, snapshot)
		}
	}
	return strings.Join(parts, "\n\n")
}

func mrAutoFixRenderSnapshot(mr *gitlab.TaskMR, delta mrAutoFixCheckpoint) string {
	if mr == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("MR: ")
	b.WriteString(mrAutoFixSanitizeSnapshotField(fmt.Sprintf("%s!%d", mr.ProjectPath, mr.MRIID)))
	if len(delta.FailedJobs) > 0 {
		b.WriteString("\n\nNew or changed failing pipeline jobs:")
		for _, job := range delta.FailedJobs {
			b.WriteString(fmt.Sprintf("\n- %s: %s", mrAutoFixSanitizeSnapshotField(job.Name), mrAutoFixSanitizeSnapshotField(job.Status)))
			if job.WebURL != "" {
				b.WriteString(" (")
				b.WriteString(mrAutoFixSanitizeSnapshotField(job.WebURL))
				b.WriteString(")")
			}
		}
	}
	if len(delta.Notes) > 0 {
		b.WriteString("\n\nNew or changed unresolved discussion comments:")
		for _, note := range delta.Notes {
			b.WriteString(fmt.Sprintf("\n- %s:%d %s",
				mrAutoFixSanitizeSnapshotField(note.Path), note.Line, mrAutoFixSanitizeSnapshotField(strings.TrimSpace(note.Body))))
		}
	}
	return b.String()
}

func mrAutoFixSanitizeSnapshotField(value string) string {
	return strings.TrimSpace(mrAutoFixSnapshotFieldReplacer.Replace(value))
}

func decodeMRAutoFixCheckpoint(state *gitlab.TaskMRLifecycleState) mrAutoFixCheckpoint {
	if state == nil || state.LastFixCheckpointJSON == "" {
		return mrAutoFixCheckpoint{}
	}
	var checkpoint mrAutoFixCheckpoint
	_ = json.Unmarshal([]byte(state.LastFixCheckpointJSON), &checkpoint)
	return checkpoint
}

func encodeMRAutoFixCheckpoint(checkpoint mrAutoFixCheckpoint) (string, string) {
	data, _ := json.Marshal(checkpoint)
	sum := sha256.Sum256(data)
	return string(data), hex.EncodeToString(sum[:])
}
