package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/db"
)

// PREndAction is what Kandev does to a PR task when its pull request ends.
type PREndAction string

const (
	PREndActionKeep    PREndAction = "keep"
	PREndActionArchive PREndAction = "archive"
	PREndActionDelete  PREndAction = "delete"
)

// ErrInvalidPRContinuationSettings reports per-PR settings that do not apply
// to the automation's policy or target.
var ErrInvalidPRContinuationSettings = errors.New("invalid per-PR continuation settings")

// PRContinuationSettings are the reuse_per_pr options. They must be empty
// (or keep) for every other policy.
type PRContinuationSettings struct {
	// FollowUpPrompt replaces Prompt when a firing continues an existing PR task.
	FollowUpPrompt string `json:"follow_up_prompt" db:"follow_up_prompt"`
	// PRContinuationStepID is the workflow step a continued normal task moves to.
	PRContinuationStepID string      `json:"pr_continuation_step_id" db:"pr_continuation_step_id"`
	PRMergedAction       PREndAction `json:"pr_merged_action" db:"pr_merged_action"`
	PRClosedAction       PREndAction `json:"pr_closed_action" db:"pr_closed_action"`
}

func normalizePREndAction(action PREndAction) PREndAction {
	if action == "" {
		return PREndActionKeep
	}
	return action
}

func (p *PRContinuationSettings) normalize() {
	p.PRMergedAction = normalizePREndAction(p.PRMergedAction)
	p.PRClosedAction = normalizePREndAction(p.PRClosedAction)
}

// validatePRContinuationSettings checks the per-PR options against the
// policy and target mode. Workflow ownership of the step is checked by the
// service, which has the workspace context.
func validatePRContinuationSettings(policy ContinuationPolicy, taskMode TaskMode, settings PRContinuationSettings) error {
	settings.normalize()
	for _, action := range []PREndAction{settings.PRMergedAction, settings.PRClosedAction} {
		switch action {
		case PREndActionKeep, PREndActionArchive, PREndActionDelete:
		default:
			return fmt.Errorf("%w: unknown end-of-PR action %q", ErrInvalidPRContinuationSettings, action)
		}
	}
	if policy != ContinuationPolicyReusePerPR {
		if strings.TrimSpace(settings.FollowUpPrompt) != "" || settings.PRContinuationStepID != "" ||
			settings.PRMergedAction != PREndActionKeep || settings.PRClosedAction != PREndActionKeep {
			return fmt.Errorf("%w: follow_up_prompt, pr_continuation_step_id, pr_merged_action, and pr_closed_action require continuation_policy reuse_per_pr", ErrInvalidPRContinuationSettings)
		}
		return nil
	}
	if taskMode == TaskModeManagedConversation {
		return fmt.Errorf("%w: reuse_per_pr does not support the managed conversation target", ErrInvalidPRContinuationSettings)
	}
	if settings.PRContinuationStepID != "" && taskMode != TaskModeNormalTask {
		return fmt.Errorf("%w: pr_continuation_step_id requires the normal task target", ErrInvalidPRContinuationSettings)
	}
	return nil
}

// applyPRContinuationUpdate applies the partial per-PR fields of an update.
func applyPRContinuationUpdate(a *Automation, req *UpdateAutomationRequest) {
	if req.FollowUpPrompt != nil {
		a.FollowUpPrompt = *req.FollowUpPrompt
	}
	if req.PRContinuationStepID != nil {
		a.PRContinuationStepID = *req.PRContinuationStepID
	}
	if req.PRMergedAction != nil {
		a.PRMergedAction = *req.PRMergedAction
	}
	if req.PRClosedAction != nil {
		a.PRClosedAction = *req.PRClosedAction
	}
	a.normalize()
}

func writePRContinuationSettings(ctx context.Context, tx *sqlx.Tx, id string, settings PRContinuationSettings) error {
	settings.normalize()
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET follow_up_prompt = ?, pr_continuation_step_id = ?,
		pr_merged_action = ?, pr_closed_action = ? WHERE id = ?`),
		settings.FollowUpPrompt, settings.PRContinuationStepID,
		string(settings.PRMergedAction), string(settings.PRClosedAction), id)
	return err
}

// prContinuationTablesSQL maps each PR subject of a reuse_per_pr automation
// to the task that handles it.
const prContinuationTablesSQL = `
	CREATE TABLE IF NOT EXISTS automation_pr_continuations (
		automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
		pr_subject TEXT NOT NULL,
		repo_owner TEXT NOT NULL,
		repo_name TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		task_id TEXT NOT NULL,
		created_at {{timestamp}} NOT NULL,
		updated_at {{timestamp}} NOT NULL,
		PRIMARY KEY (automation_id, pr_subject)
	);
`

// PRContinuation maps one PR subject to its task.
type PRContinuation struct {
	AutomationID string `db:"automation_id"`
	PRSubject    string `db:"pr_subject"`
	RepoOwner    string `db:"repo_owner"`
	RepoName     string `db:"repo_name"`
	PRNumber     int    `db:"pr_number"`
	TaskID       string `db:"task_id"`
}

// GetPRContinuationTaskID returns the task mapped to a PR subject, or "".
func (s *Store) GetPRContinuationTaskID(ctx context.Context, automationID, subject string) (string, error) {
	var taskID string
	err := s.ro.GetContext(ctx, &taskID, s.ro.Rebind(
		`SELECT task_id FROM automation_pr_continuations WHERE automation_id = ? AND pr_subject = ?`),
		automationID, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return taskID, err
}

// UpsertPRContinuation maps a PR subject to a task, replacing any prior task.
func (s *Store) UpsertPRContinuation(ctx context.Context, c *PRContinuation) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO automation_pr_continuations (
			automation_id, pr_subject, repo_owner, repo_name, pr_number, task_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(automation_id, pr_subject) DO UPDATE SET
			task_id = excluded.task_id, updated_at = excluded.updated_at`),
		c.AutomationID, c.PRSubject, c.RepoOwner, c.RepoName, c.PRNumber, c.TaskID, now, now)
	return err
}

// ListPRContinuationsForRepo returns an automation's mappings for one repository.
func (s *Store) ListPRContinuationsForRepo(ctx context.Context, automationID, owner, repo string) ([]PRContinuation, error) {
	var rows []PRContinuation
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT automation_id, pr_subject, repo_owner, repo_name, pr_number, task_id
		FROM automation_pr_continuations
		WHERE automation_id = ? AND repo_owner = ? AND repo_name = ?
		ORDER BY pr_number`), automationID, owner, repo)
	return rows, err
}

// DeletePRContinuation removes a mapping only while it still names taskID, so
// a concurrent replacement is never lost.
func (s *Store) DeletePRContinuation(ctx context.Context, automationID, subject, taskID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM automation_pr_continuations
		WHERE automation_id = ? AND pr_subject = ? AND task_id = ?`), automationID, subject, taskID)
	return err
}

// HasActiveRunForTask reports whether the automation has an open run bound
// to taskID, using the same predicate as CountActiveRuns.
func (s *Store) HasActiveRunForTask(ctx context.Context, automationID, taskID string) (bool, error) {
	if taskID == "" {
		return false, nil
	}
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM automation_runs ar
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE ar.automation_id = ? AND ar.task_id = ? AND `+openRunPredicateSQL),
		append([]any{automationID, taskID}, openRunArgs()...)...)
	if db.IsMissingTableError(err) {
		err = s.ro.GetContext(ctx, &count, s.ro.Rebind(`
			SELECT COUNT(*) FROM automation_runs
			WHERE automation_id = ? AND task_id = ? AND status IN (?, ?)`),
			automationID, taskID, string(RunStatusTriggered), string(RunStatusTaskCreated))
	}
	return count > 0, err
}

// validatePRContinuation checks the per-PR settings and that a continuation
// step belongs to the automation's workflow.
func (s *Service) validatePRContinuation(
	ctx context.Context, workspaceID, workflowID string,
	policy ContinuationPolicy, taskMode TaskMode, settings PRContinuationSettings,
) error {
	if policy == "" {
		policy = ContinuationPolicyNewTask
	}
	if taskMode == "" {
		taskMode = TaskModeAutomationRun
	}
	if err := validatePRContinuationSettings(policy, taskMode, settings); err != nil {
		return err
	}
	if err := s.authorizeWorkflowStepOwnership(ctx, workspaceID, workflowID, settings.PRContinuationStepID); err != nil {
		return fmt.Errorf("%w: pr_continuation_step_id: %v", ErrInvalidPRContinuationSettings, err)
	}
	return nil
}

// mergedPRContinuationSettings returns the per-PR settings an update leaves.
func mergedPRContinuationSettings(existing *Automation, req *UpdateAutomationRequest) PRContinuationSettings {
	merged := *existing
	applyPRContinuationUpdate(&merged, req)
	return merged.PRContinuationSettings
}

// errAdmissionDeferred postpones a firing without creating a run or
// consuming its dedup key.
var errAdmissionDeferred = errors.New("automation admission deferred")

// PRTaskController exposes the task operations the per-PR policy needs
// without importing the task service. Busy reports whether the task's
// primary session is running, starting, or has a queued turn.
type PRTaskController interface {
	AutomationPRTaskBusy(ctx context.Context, taskID string) (bool, error)
	ArchiveAutomationPRTask(ctx context.Context, taskID string) error
	DeleteAutomationPRTask(ctx context.Context, taskID, reason string) error
}

// SetPRTaskController wires the per-PR task operations.
func (s *Service) SetPRTaskController(c PRTaskController) {
	s.prTasks = c
}

// prTaskBusy reports whether a mapped PR task has an open run or a busy
// session. A checker error counts as busy so the firing is retried.
func (s *Service) prTaskBusy(ctx context.Context, automationID, taskID string) (bool, error) {
	active, err := s.store.HasActiveRunForTask(ctx, automationID, taskID)
	if err != nil || active {
		return true, err
	}
	if s.prTasks == nil {
		return false, nil
	}
	busy, err := s.prTasks.AutomationPRTaskBusy(ctx, taskID)
	if err != nil {
		s.logger.Warn("PR task busy check failed; deferring",
			zap.String("automation_id", automationID), zap.String("task_id", taskID), zap.Error(err))
		return true, nil
	}
	return busy, nil
}

// deferBusyPRContinuation returns errAdmissionDeferred when a reuse_per_pr
// firing's PR task is busy. It runs under the per-automation admission lock.
func (s *Service) deferBusyPRContinuation(ctx context.Context, a *Automation, triggerType TriggerType, data json.RawMessage) error {
	subject := PRSubjectForFiring(a, triggerType, data)
	if subject == "" {
		return nil
	}
	taskID, err := s.store.GetPRContinuationTaskID(ctx, a.ID, subject)
	if err != nil {
		return fmt.Errorf("load PR continuation: %w", err)
	}
	if taskID == "" {
		return nil
	}
	busy, err := s.prTaskBusy(ctx, a.ID, taskID)
	if err != nil {
		s.logger.Warn("PR task run check failed; deferring",
			zap.String("automation_id", a.ID), zap.String("task_id", taskID), zap.Error(err))
	}
	if !busy {
		return nil
	}
	s.logger.Info("deferred github_pr firing while the PR task is busy",
		zap.String("automation_id", a.ID), zap.String("pr_subject", subject), zap.String("task_id", taskID))
	return errAdmissionDeferred
}

// buildExportFollowUpPrompt encodes the follow-up prompt with the main
// prompt's fidelity rules. An empty prompt omits the key.
func buildExportFollowUpPrompt(a *Automation, warnings *[]exportWarning) (*yaml.Node, error) {
	if a.FollowUpPrompt == "" {
		return nil, nil
	}
	node, warning, err := buildPromptNode(a.FollowUpPrompt)
	if err != nil {
		return nil, fmt.Errorf("build follow-up prompt for automation %q: %w", a.ID, err)
	}
	if warning != "" {
		*warnings = append(*warnings, exportWarning{AutomationName: a.Name, AutomationID: a.ID, DedupKey: a.ID + ":follow_up_prompt", Message: warning})
	}
	return node, nil
}

// exportPREndActions emits end-of-PR actions only for reuse_per_pr.
func exportPREndActions(a *Automation) (PREndAction, PREndAction) {
	if a.ContinuationPolicy != ContinuationPolicyReusePerPR {
		return "", ""
	}
	return normalizePREndAction(a.PRMergedAction), normalizePREndAction(a.PRClosedAction)
}

// ParsePRSubject splits `<owner>/<repo>#<number>`.
func ParsePRSubject(subject string) (owner, repo string, number int, ok bool) {
	repoPart, numberPart, found := strings.Cut(subject, "#")
	if !found {
		return "", "", 0, false
	}
	owner, repo, found = strings.Cut(repoPart, "/")
	if !found || owner == "" || repo == "" {
		return "", "", 0, false
	}
	number, err := strconv.Atoi(numberPart)
	if err != nil || number <= 0 {
		return "", "", 0, false
	}
	return owner, repo, number, true
}

// GetPRContinuationTaskID returns the task mapped to a PR subject, or "".
func (s *Service) GetPRContinuationTaskID(ctx context.Context, automationID, subject string) (string, error) {
	return s.store.GetPRContinuationTaskID(ctx, automationID, subject)
}

// SetPRContinuationTaskID maps a PR subject to the task that handles it.
func (s *Service) SetPRContinuationTaskID(ctx context.Context, automationID, subject, taskID string) error {
	owner, repo, number, ok := ParsePRSubject(subject)
	if !ok {
		return fmt.Errorf("invalid PR subject %q", subject)
	}
	return s.store.UpsertPRContinuation(ctx, &PRContinuation{
		AutomationID: automationID, PRSubject: subject, RepoOwner: owner, RepoName: repo,
		PRNumber: number, TaskID: taskID,
	})
}

// PRSubjectForFiring returns the PR subject a firing continues under the
// per-PR policy, or "" when the firing uses new-task behavior.
func PRSubjectForFiring(a *Automation, triggerType TriggerType, data json.RawMessage) string {
	if a == nil || a.ContinuationPolicy != ContinuationPolicyReusePerPR ||
		triggerType != TriggerTypeGitHubPR || len(data) == 0 {
		return ""
	}
	var payload struct {
		Subject string `json:"pr_subject"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return payload.Subject
}
