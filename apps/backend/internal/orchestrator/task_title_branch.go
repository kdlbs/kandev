package orchestrator

import (
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// TitleBranchRename is one successful or already-satisfied generated-branch
// outcome returned by the title handoff.
type TitleBranchRename struct {
	RepositoryID string `json:"repository_id"`
	From         string `json:"from"`
	To           string `json:"to"`
}

// TitleBranchPreservation records a repository that intentionally keeps its
// current branch because Kandev does not own that checkout.
type TitleBranchPreservation struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Reason       string `json:"reason"`
}

// TitleBranchFailure records a generated-branch rename that could not be
// completed after the task title itself was accepted.
type TitleBranchFailure struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Message      string `json:"message"`
}

const (
	TitleBranchStatusRenamed       = "renamed"
	TitleBranchStatusPreserved     = "preserved"
	TitleBranchStatusPartial       = "partial"
	TitleBranchStatusFailed        = "failed"
	TitleBranchStatusNotApplicable = "not_applicable"
)

// TitleBranchRenameResult is the branch side-effect portion of an accepted
// generated title update. It deliberately reports partial Git outcomes rather
// than making title persistence appear to have failed.
type TitleBranchRenameResult struct {
	Status    string                    `json:"status"`
	Renamed   []TitleBranchRename       `json:"renamed,omitempty"`
	Preserved []TitleBranchPreservation `json:"preserved,omitempty"`
	Failed    []TitleBranchFailure      `json:"failed,omitempty"`
}

func renderTitleBranchNameForTaskRepository(
	title string,
	task *models.Task,
	repository *models.Repository,
	taskRepository *models.TaskRepository,
	suffix string,
) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is required")
	}
	if repository == nil {
		return "", fmt.Errorf("repository is required")
	}
	template := repository.WorktreeBranchTemplate
	if taskRepository != nil && taskRepository.BranchPolicyBranchTemplate != "" {
		template = taskRepository.BranchPolicyBranchTemplate
	}
	return worktree.RenderTaskBranchName(worktree.BranchNameTemplateInput{
		Template: template,
		TaskID:   task.ID,
		Title:    title,
		Ticket:   worktree.TicketForBranchName(task.Identifier, task.Metadata),
		Suffix:   suffix,
	})
}

func aggregateTitleBranchRenameStatus(
	renamed []TitleBranchRename,
	preserved []TitleBranchPreservation,
	failed []TitleBranchFailure,
) string {
	if len(renamed) == 0 && len(preserved) == 0 && len(failed) == 0 {
		return TitleBranchStatusNotApplicable
	}
	if len(failed) == 0 {
		if len(renamed) > 0 {
			return TitleBranchStatusRenamed
		}
		return TitleBranchStatusPreserved
	}
	if len(renamed) > 0 {
		return TitleBranchStatusPartial
	}
	return TitleBranchStatusFailed
}
