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
	"github.com/kandev/kandev/internal/task/models"
)

const (
	ciAutomationOrigin           = "ci_automation"
	ciAutomationCheckSuccess     = "success"
	ciAutomationCheckFailure     = "failure"
	ciAutomationCheckSkipped     = "skipped"
	ciAutomationCheckCompleted   = "completed"
	ciAutomationChangesRequested = "changes_requested"
)

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
	if options.AutoFixEnabled && ciAutomationShouldAutoFix(pr) {
		s.handleTaskPRCIAutoFix(ctx, pr, options)
	}
	if options.AutoMergeEnabled && ciAutomationReadyToMerge(pr) {
		s.handleTaskPRCIAutoMerge(ctx, pr)
	}
	return nil
}

func (s *Service) handleTaskPRCIAutoFix(ctx context.Context, pr *github.TaskPR, options *github.TaskCIOptionsResponse) {
	feedback, err := s.githubService.GetPRFeedback(ctx, pr.Owner, pr.Repo, pr.PRNumber)
	if err != nil {
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("fetch PR feedback: %v", err))
		return
	}
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("load CI automation state: %v", err))
		return
	}
	previous := decodeCIAutomationCheckpoint(state)
	delta := ciAutomationBuildDelta(feedback, previous)
	if len(delta.FailedChecks) == 0 && len(delta.Comments) == 0 {
		return
	}
	checkpoint := ciAutomationCheckpoint{FailedChecks: append(previous.FailedChecks, delta.FailedChecks...), Comments: append(previous.Comments, delta.Comments...)}
	checkpointJSON, signature := encodeCIAutomationCheckpoint(checkpoint)
	if state != nil && state.LastFixSignature == signature {
		return
	}
	prompt := ciAutomationRenderPrompt(options.EffectiveAutoFixPrompt, pr, delta)
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, pr.TaskID)
	if err != nil || session == nil {
		s.recordCIAutomationError(ctx, pr, "no promptable task session for CI auto-fix")
		return
	}
	if err := s.dispatchCIAutomationPrompt(ctx, session, prompt); err != nil {
		s.recordCIAutomationError(ctx, pr, err.Error())
		return
	}
	if err := s.githubService.RecordTaskCIFixAttempt(ctx, github.TaskCIFixAttempt{
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
}

func (s *Service) handleTaskPRCIAutoMerge(ctx context.Context, pr *github.TaskPR) {
	signature := ciAutomationMergeSignature(pr)
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err == nil && state != nil && state.LastMergeSignature == signature {
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
		_ = s.githubService.RecordTaskCIMergeAttempt(ctx, attempt)
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("merge PR: %v", err))
		return
	}
	_ = s.githubService.RecordTaskCIMergeAttempt(ctx, attempt)
	_ = s.githubService.ClearTaskCIError(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
}

func (s *Service) dispatchCIAutomationPrompt(ctx context.Context, session *models.TaskSession, prompt string) error {
	switch session.State {
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		if s.messageQueue == nil {
			return fmt.Errorf("message queue is not configured")
		}
		_, err := s.messageQueue.QueueMessageWithMetadata(ctx, session.ID, session.TaskID, prompt, "", messagequeue.QueuedByWorkflow, false, nil, map[string]interface{}{
			"origin": ciAutomationOrigin,
		})
		if err != nil {
			return err
		}
		s.publishQueueStatusEvent(ctx, session.ID)
		return nil
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		_, err := s.PromptTask(ctx, session.TaskID, session.ID, prompt, "", false, nil, false)
		return err
	default:
		return fmt.Errorf("session is not promptable: %s", session.State)
	}
}

func (s *Service) recordCIAutomationError(ctx context.Context, pr *github.TaskPR, message string) {
	if err := s.githubService.RecordTaskCIError(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber, message); err != nil {
		s.logger.Debug("record CI automation error failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	}
}

func ciAutomationShouldAutoFix(pr *github.TaskPR) bool {
	if pr == nil || pr.State == "closed" || pr.State == "merged" {
		return false
	}
	return pr.ChecksState == ciAutomationCheckFailure || pr.ReviewState == ciAutomationChangesRequested || pr.UnresolvedReviewThreads > 0
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
		prevChecks[check.Name+"|"+check.Conclusion+"|"+check.HTMLURL] = struct{}{}
	}
	prevComments := make(map[int64]struct{}, len(previous.Comments))
	for _, comment := range previous.Comments {
		prevComments[comment.ID] = struct{}{}
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
		if _, seen := prevChecks[snap.Name+"|"+snap.Conclusion+"|"+snap.HTMLURL]; !seen {
			delta.FailedChecks = append(delta.FailedChecks, snap)
		}
	}
	for _, comment := range feedback.Comments {
		if _, seen := prevComments[comment.ID]; seen {
			continue
		}
		delta.Comments = append(delta.Comments, ciAutomationCommentSnapshot{
			ID: comment.ID, Body: comment.Body, Path: comment.Path, Line: comment.Line,
		})
	}
	return delta
}

func ciAutomationRenderPrompt(base string, pr *github.TaskPR, delta ciAutomationCheckpoint) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(base))
	b.WriteString("\n\nPR: ")
	b.WriteString(fmt.Sprintf("%s/%s#%d", pr.Owner, pr.Repo, pr.PRNumber))
	if len(delta.FailedChecks) > 0 {
		b.WriteString("\n\nNew or changed failing checks:")
		for _, check := range delta.FailedChecks {
			b.WriteString(fmt.Sprintf("\n- %s: %s", check.Name, check.Conclusion))
			if check.HTMLURL != "" {
				b.WriteString(" (")
				b.WriteString(check.HTMLURL)
				b.WriteString(")")
			}
		}
	}
	if len(delta.Comments) > 0 {
		b.WriteString("\n\nNew or changed review comments:")
		for _, comment := range delta.Comments {
			b.WriteString(fmt.Sprintf("\n- %s:%d %s", comment.Path, comment.Line, strings.TrimSpace(comment.Body)))
		}
	}
	return b.String()
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
	payload := fmt.Sprintf("%s|%s|%d|%s|%s|%s|%d|%d", pr.TaskID, pr.RepositoryID, pr.PRNumber, pr.ChecksState, pr.ReviewState, pr.MergeableState, pr.ReviewCount, pr.UnresolvedReviewThreads)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
