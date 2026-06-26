package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	ciAutomationOrigin           = "ci_automation"
	ciAutomationCheckSuccess     = "success"
	ciAutomationCheckFailure     = "failure"
	ciAutomationCheckSkipped     = "skipped"
	ciAutomationCheckCompleted   = "completed"
	ciAutomationChangesRequested = "changes_requested"
	ciAutomationPRFeedbackToken  = "{{pr.feedback}}"
	ciAutomationFreshSyncWindow  = 30 * time.Second
)

var ciAutomationSnapshotFieldReplacer = strings.NewReplacer("\r", " ", "\n", " ", "<", "", ">", "")

type ciAutomationCheckpoint struct {
	FailedChecks []ciAutomationCheckSnapshot   `json:"failed_checks"`
	Comments     []ciAutomationCommentSnapshot `json:"comments"`
}

type ciAutomationCheckSnapshot struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	Output     string `json:"output,omitempty"`
}

type ciAutomationCommentSnapshot struct {
	ID   int64  `json:"id"`
	Body string `json:"body,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

func (s *Service) handleTaskPRCIAutomation(ctx context.Context, pr *github.TaskPR) error {
	if s.githubService == nil || pr == nil {
		return nil
	}
	options, err := s.githubService.GetTaskCIOptionsResponse(ctx, pr.TaskID)
	if err != nil {
		s.logger.Debug("load CI automation options failed", zap.String("task_id", pr.TaskID), zap.Error(err))
		return nil
	}
	freshlySynced := false
	if options.AutoFixEnabled || options.AutoMergeEnabled {
		refreshed, synced, syncErr := s.refreshTaskPRForCIAutomation(ctx, pr)
		if syncErr != nil {
			s.recordCIAutomationError(ctx, pr, fmt.Sprintf("sync PR status: %v", syncErr))
			return nil
		}
		pr = refreshed
		freshlySynced = synced
	}
	autoFixBlockedMerge := false
	if options.AutoFixEnabled && ciAutomationCanAutoFixFromFeedback(pr) {
		autoFixBlockedMerge = s.handleTaskPRCIAutoFix(ctx, pr, options)
	}
	if autoFixBlockedMerge {
		return nil
	}
	if options.AutoMergeEnabled && ciAutomationReadyToMerge(pr) {
		if !freshlySynced {
			s.recordCIAutomationError(ctx, pr, "PR status is not freshly synced for auto-merge")
			return nil
		}
		s.handleTaskPRCIAutoMerge(ctx, pr)
	}
	return nil
}

func (s *Service) refreshTaskPRForCIAutomation(ctx context.Context, pr *github.TaskPR) (*github.TaskPR, bool, error) {
	if pr == nil {
		return nil, false, nil
	}
	prs, err := s.githubService.TriggerPRSyncAll(ctx, pr.TaskID)
	if err != nil {
		return nil, false, err
	}
	if refreshed := ciAutomationFindMatchingPR(prs, pr); refreshed != nil {
		return refreshed, ciAutomationHasFreshPRStatus(refreshed), nil
	}
	return pr, false, nil
}

func ciAutomationFindMatchingPR(prs []*github.TaskPR, target *github.TaskPR) *github.TaskPR {
	if target == nil {
		return nil
	}
	for _, pr := range prs {
		if pr == nil || pr.TaskID != target.TaskID || pr.PRNumber != target.PRNumber {
			continue
		}
		if target.RepositoryID != "" && pr.RepositoryID == target.RepositoryID {
			return pr
		}
		if target.RepositoryID == "" && pr.Owner == target.Owner && pr.Repo == target.Repo {
			return pr
		}
	}
	return nil
}

func ciAutomationHasFreshPRStatus(pr *github.TaskPR) bool {
	if pr == nil || pr.LastSyncedAt == nil {
		return false
	}
	return time.Since(*pr.LastSyncedAt) <= ciAutomationFreshSyncWindow
}

func (s *Service) handleTaskPRCIAutoFix(ctx context.Context, pr *github.TaskPR, options *github.TaskCIOptionsResponse) bool {
	feedback, err := s.githubService.GetPRFeedback(ctx, pr.Owner, pr.Repo, pr.PRNumber)
	if err != nil {
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("fetch PR feedback: %v", err))
		return true
	}
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("load CI automation state: %v", err))
		return true
	}
	previous := decodeCIAutomationCheckpoint(state)
	delta := ciAutomationBuildDelta(feedback, previous)
	checkpoint := ciAutomationCurrentCheckpoint(feedback)
	if len(delta.FailedChecks) == 0 && len(delta.Comments) == 0 {
		if state != nil && len(previous.FailedChecks)+len(previous.Comments) > 0 {
			checkpointJSON, signature := encodeCIAutomationCheckpoint(checkpoint)
			if err := s.githubService.RefreshTaskCIFixCheckpoint(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, signature, checkpointJSON); err != nil {
				s.logger.Debug("record CI auto-fix checkpoint refresh failed", zap.String("task_id", pr.TaskID), zap.Error(err))
			}
		}
		return false
	}
	checkpointJSON, signature := encodeCIAutomationCheckpoint(checkpoint)
	if state != nil && state.LastFixSignature == signature {
		return false
	}
	prompt := ciAutomationRenderPrompt(options.EffectiveAutoFixPrompt, pr, delta)
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, pr.TaskID)
	if err != nil || session == nil {
		s.recordCIAutomationError(ctx, pr, "no promptable task session for CI auto-fix")
		return true
	}
	if err := s.dispatchCIAutomationPrompt(ctx, session, prompt); err != nil {
		s.recordCIAutomationError(ctx, pr, err.Error())
		return true
	}
	if err := s.githubService.RecordTaskCIFixAttempt(context.WithoutCancel(ctx), github.TaskCIFixAttempt{
		TaskID:         pr.TaskID,
		RepositoryID:   pr.RepositoryID,
		PRNumber:       pr.PRNumber,
		Signature:      signature,
		CheckpointJSON: checkpointJSON,
		SessionID:      session.ID,
		EnqueuedAt:     time.Now().UTC(),
	}); err != nil {
		s.logger.Debug("record CI auto-fix attempt failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	}
	return true
}

func (s *Service) handleTaskPRCIAutoMerge(ctx context.Context, pr *github.TaskPR) {
	signature := ciAutomationMergeSignature(pr)
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		s.logger.Debug("load CI automation merge state failed; attempting merge without dedupe", zap.String("task_id", pr.TaskID), zap.Error(err))
	} else if state != nil && state.LastMergeSignature == signature {
		return
	}
	attempt := github.TaskCIMergeAttempt{
		TaskID:       pr.TaskID,
		RepositoryID: pr.RepositoryID,
		PRNumber:     pr.PRNumber,
		Signature:    signature,
		AttemptedAt:  time.Now().UTC(),
	}
	if err := s.githubService.MergePR(ctx, pr.Owner, pr.Repo, pr.PRNumber, ""); err != nil {
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("merge PR: %v", err))
		return
	}
	_ = s.githubService.RecordTaskCIMergeAttempt(context.WithoutCancel(ctx), attempt)
	_ = s.githubService.ClearTaskCIError(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber)
}

func (s *Service) dispatchCIAutomationPrompt(ctx context.Context, session *models.TaskSession, prompt string) error {
	chatPrompt := ciAutomationChatPrompt(prompt)
	switch session.State {
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		if s.messageQueue == nil {
			return fmt.Errorf("message queue is not configured")
		}
		metadata := ciAutomationMessageMetadata()
		_, err := s.messageQueue.QueueMessageWithMetadata(ctx, session.ID, session.TaskID, chatPrompt, "", messagequeue.QueuedByWorkflow, false, nil, metadata)
		if err != nil {
			return err
		}
		s.publishQueueStatusEvent(ctx, session.ID)
		return nil
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		if !s.recordCIAutomationUserMessage(ctx, session.TaskID, session.ID, chatPrompt) {
			return fmt.Errorf("failed to record CI automation user message")
		}
		if _, err := s.PromptTask(ctx, session.TaskID, session.ID, chatPrompt, "", false, nil, true); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("session is not promptable: %s", session.State)
	}
}

func (s *Service) recordCIAutomationUserMessage(ctx context.Context, taskID, sessionID, prompt string) bool {
	if s.messageCreator == nil || prompt == "" {
		return false
	}
	turnID := s.getActiveTurnID(sessionID)
	if turnID == "" {
		s.startTurnForSession(ctx, sessionID)
		turnID = s.getActiveTurnID(sessionID)
	}
	meta := ciAutomationMessageMetadata()
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, meta); err != nil {
		s.logger.Error("failed to create CI automation user message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}
	return true
}

func ciAutomationMessageMetadata() map[string]interface{} {
	meta := NewUserMessageMeta().WithAutoStart(true).ToMap()
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["origin"] = ciAutomationOrigin
	return meta
}

func ciAutomationChatPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "@ci-auto-fix"
	}
	return "@ci-auto-fix\n\n" + prompt
}

func (s *Service) recordCIAutomationError(ctx context.Context, pr *github.TaskPR, message string) {
	s.logger.Warn("CI automation error",
		zap.String("task_id", pr.TaskID),
		zap.String("repository_id", pr.RepositoryID),
		zap.String("owner", pr.Owner),
		zap.String("repo", pr.Repo),
		zap.Int("pr_number", pr.PRNumber),
		zap.String("error", message))
	if err := s.githubService.RecordTaskCIError(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, message); err != nil {
		s.logger.Debug("record CI automation error failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	}
}

func ciAutomationShouldAutoFix(pr *github.TaskPR) bool {
	if pr == nil || pr.State == "closed" || pr.State == "merged" {
		return false
	}
	return pr.ChecksState == ciAutomationCheckFailure || pr.ReviewState == ciAutomationChangesRequested || pr.UnresolvedReviewThreads > 0
}

func ciAutomationCanAutoFixFromFeedback(pr *github.TaskPR) bool {
	return pr != nil && pr.State != "closed" && pr.State != "merged"
}

func ciAutomationReadyToMerge(pr *github.TaskPR) bool {
	if pr == nil || pr.State != "open" {
		return false
	}
	if pr.ChecksState != ciAutomationCheckSuccess || pr.MergeableState != "clean" {
		return false
	}
	if pr.ReviewState == ciAutomationChangesRequested || pr.PendingReviewCount > 0 || pr.UnresolvedReviewThreads > 0 {
		return false
	}
	if pr.RequiredReviews != nil && pr.ReviewCount < *pr.RequiredReviews {
		return false
	}
	return true
}

func ciAutomationBuildDelta(feedback *github.PRFeedback, previous ciAutomationCheckpoint) ciAutomationCheckpoint {
	prevChecks := make(map[string]struct{}, len(previous.FailedChecks))
	for _, check := range previous.FailedChecks {
		prevChecks[ciAutomationCheckKey(check)] = struct{}{}
	}
	prevComments := make(map[int64]ciAutomationCommentSnapshot, len(previous.Comments))
	for _, comment := range previous.Comments {
		prevComments[comment.ID] = comment
	}
	var delta ciAutomationCheckpoint
	if feedback == nil {
		return delta
	}
	for _, check := range feedback.Checks {
		if check.Status != ciAutomationCheckCompleted || check.Conclusion == ciAutomationCheckSuccess || check.Conclusion == ciAutomationCheckSkipped {
			continue
		}
		snap := ciAutomationCheckSnapshot{Name: check.Name, Conclusion: check.Conclusion, HTMLURL: check.HTMLURL, Output: check.Output}
		if _, seen := prevChecks[ciAutomationCheckKey(snap)]; !seen {
			delta.FailedChecks = append(delta.FailedChecks, snap)
		}
	}
	for _, comment := range feedback.Comments {
		snap := ciAutomationCommentSnapshot{
			ID: comment.ID, Body: comment.Body, Path: comment.Path, Line: comment.Line,
		}
		if previous, seen := prevComments[comment.ID]; seen && previous == snap {
			continue
		}
		delta.Comments = append(delta.Comments, snap)
	}
	return delta
}

func ciAutomationCheckKey(check ciAutomationCheckSnapshot) string {
	return check.Name + "|" + check.Conclusion + "|" + check.HTMLURL + "|" + check.Output
}

func ciAutomationCurrentCheckpoint(feedback *github.PRFeedback) ciAutomationCheckpoint {
	return ciAutomationBuildDelta(feedback, ciAutomationCheckpoint{})
}

func ciAutomationRenderPrompt(base string, pr *github.TaskPR, delta ciAutomationCheckpoint) string {
	if base = strings.TrimSpace(base); base != "" {
		return ciAutomationRenderPromptTemplate(base, ciAutomationRenderSnapshot(pr, delta))
	}
	return ""
}

func ciAutomationRenderPromptTemplate(base, snapshot string) string {
	if !strings.Contains(base, ciAutomationPRFeedbackToken) {
		return sysprompt.Wrap(base)
	}
	segments := strings.Split(base, ciAutomationPRFeedbackToken)
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

func ciAutomationRenderSnapshot(pr *github.TaskPR, delta ciAutomationCheckpoint) string {
	if pr == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("PR: ")
	b.WriteString(fmt.Sprintf("%s/%s#%d", ciAutomationSanitizeSnapshotField(pr.Owner), ciAutomationSanitizeSnapshotField(pr.Repo), pr.PRNumber))
	if len(delta.FailedChecks) > 0 {
		b.WriteString("\n\nNew or changed failing checks:")
		for _, check := range delta.FailedChecks {
			b.WriteString(fmt.Sprintf("\n- %s: %s", ciAutomationSanitizeSnapshotField(check.Name), ciAutomationSanitizeSnapshotField(check.Conclusion)))
			if check.HTMLURL != "" {
				b.WriteString(" (")
				b.WriteString(ciAutomationSanitizeSnapshotField(check.HTMLURL))
				b.WriteString(")")
			}
		}
	}
	if len(delta.Comments) > 0 {
		b.WriteString("\n\nNew or changed review comments:")
		for _, comment := range delta.Comments {
			b.WriteString(fmt.Sprintf("\n- %s:%d %s", ciAutomationSanitizeSnapshotField(comment.Path), comment.Line, ciAutomationSanitizeSnapshotField(strings.TrimSpace(comment.Body))))
		}
	}
	return b.String()
}

func ciAutomationSanitizeSnapshotField(value string) string {
	return strings.TrimSpace(ciAutomationSnapshotFieldReplacer.Replace(value))
}

func decodeCIAutomationCheckpoint(state *github.TaskCIPRAutomationState) ciAutomationCheckpoint {
	if state == nil || state.LastFixCheckpointJSON == "" {
		return ciAutomationCheckpoint{}
	}
	var checkpoint ciAutomationCheckpoint
	_ = json.Unmarshal([]byte(state.LastFixCheckpointJSON), &checkpoint)
	return checkpoint
}

func encodeCIAutomationCheckpoint(checkpoint ciAutomationCheckpoint) (string, string) {
	data, _ := json.Marshal(checkpoint)
	sum := sha256.Sum256(data)
	return string(data), hex.EncodeToString(sum[:])
}

func ciAutomationMergeSignature(pr *github.TaskPR) string {
	payload := fmt.Sprintf("%s|%s|%d|%s|%d|%d|%s|%s|%s|%d|%d", pr.TaskID, pr.RepositoryID, pr.PRNumber, pr.HeadBranch, pr.Additions, pr.Deletions, pr.ChecksState, pr.ReviewState, pr.MergeableState, pr.ReviewCount, pr.UnresolvedReviewThreads)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
