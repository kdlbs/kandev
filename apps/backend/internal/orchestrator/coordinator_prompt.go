package orchestrator

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// SetCoordinatorStandingInstructionsReader installs the coordinator Standing
// Instructions content builder
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
// Wired in internal/backendapp/main.go with a closure over
// coordinator.Service.CoordinatorStandingInstructionsData and
// coordinator.StandingInstructions: this package cannot import
// internal/coordinator directly, since internal/coordinator already imports
// internal/orchestrator.
func (s *Service) SetCoordinatorStandingInstructionsReader(
	reader func(ctx context.Context, coordinatorID, workspaceName, workspaceID string) (string, error),
) {
	s.coordinatorStandingInstructions = reader
}

// coordinatorMetadataID reads the coordinator id a coordinator-origin task's
// conversation was opened for, mirroring
// coordinator.conversationTaskCoordinatorID's defensive metadata read.
func coordinatorMetadataID(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	if purpose, _ := task.Metadata[models.MetaKeyCoordinatorPurpose].(string); purpose == models.CoordinatorPurposeDream {
		return ""
	}
	id, _ := task.Metadata[models.MetaKeyCoordinatorID].(string)
	return id
}

// wrapCoordinatorStandingInstructions attaches the Standing Instructions
// system block ahead of a coordinator conversation's first prompt
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
// Falls back to the bare prompt whenever the block cannot be built (reader
// unwired, no coordinator id on the task, or a lookup failure): the
// coordinator's tool surface and attended-only restriction are enforced
// server-side regardless, so a missing informational block is a degraded
// prompt, not a security gap.
func (s *Service) wrapCoordinatorStandingInstructions(ctx context.Context, prompt string, dbTask *models.Task) string {
	if s.coordinatorStandingInstructions == nil {
		return prompt
	}
	coordinatorID := coordinatorMetadataID(dbTask)
	if coordinatorID == "" {
		return prompt
	}
	workspaceName := ""
	if s.repo != nil {
		if workspace, err := s.repo.GetWorkspace(ctx, dbTask.WorkspaceID); err == nil && workspace != nil {
			workspaceName = workspace.Name
		}
	}
	content, err := s.coordinatorStandingInstructions(ctx, coordinatorID, workspaceName, dbTask.WorkspaceID)
	if err != nil || content == "" {
		return prompt
	}
	return sysprompt.Wrap(content) + "\n\n" + prompt
}

// CoordinatorNotesDelivery is the coordinator-side note index seam. Prepare
// returns the hidden section frozen for a session turn (empty when none) and
// Accepted records its delivery once the turn's prompt is accepted. It is a
// closure pair because this package cannot import internal/coordinator.
type CoordinatorNotesDelivery struct {
	Prepare  func(ctx context.Context, coordinatorID, sessionTurnID string) string
	Accepted func(ctx context.Context, coordinatorID, sessionTurnID string)
}

// SetCoordinatorNotesDelivery installs the per-turn note index seam.
func (s *Service) SetCoordinatorNotesDelivery(d CoordinatorNotesDelivery) {
	s.coordinatorNotes = d
}

// withCoordinatorNotes adds the note index to the prompt of a coordinator
// conversation turn that is dispatched as a new session turn. afterStanding
// places it after a leading standing-instructions block. It returns the
// prompt unchanged, and a nil accepted callback, when the task is not a
// coordinator conversation, is a dream episode, has no turn identity or the
// prompt is empty (an index alone never makes an empty prompt dispatchable).
func (s *Service) withCoordinatorNotes(
	ctx context.Context, dbTask *models.Task, turnID, prompt string, afterStanding bool,
) (string, func()) {
	if s.coordinatorNotes.Prepare == nil || turnID == "" || prompt == "" ||
		dbTask == nil || dbTask.Origin != models.TaskOriginCoordinator {
		return prompt, nil
	}
	coordinatorID := coordinatorMetadataID(dbTask)
	if coordinatorID == "" {
		return prompt, nil
	}
	section := s.coordinatorNotes.Prepare(ctx, coordinatorID, turnID)
	if section == "" {
		return prompt, nil
	}
	accepted := func() {
		if s.coordinatorNotes.Accepted == nil {
			return
		}
		acceptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		s.coordinatorNotes.Accepted(acceptCtx, coordinatorID, turnID)
	}
	return insertNotesSection(prompt, section, afterStanding), accepted
}

// insertNotesSection places section at the start of prompt, or after the
// prompt's leading system block when afterStanding is set.
func insertNotesSection(prompt, section string, afterStanding bool) string {
	if afterStanding && strings.HasPrefix(prompt, sysprompt.TagStart) {
		if end := strings.Index(prompt, sysprompt.TagEnd); end >= 0 {
			cut := end + len(sysprompt.TagEnd)
			return prompt[:cut] + "\n\n" + section + prompt[cut:]
		}
	}
	return section + "\n\n" + prompt
}

// attachPromptNotes adds the coordinator note index to a prompt dispatched as
// session turn turnID and chains the delivery record onto onDispatched, which
// the executor calls once the prompt is accepted.
func (s *Service) attachPromptNotes(
	ctx context.Context, taskID string, session *models.TaskSession, turnID, prompt string,
	options promptTaskOptions, onDispatched func(),
) (string, func()) {
	if options.internalContinuation || session.IsPassthrough || s.coordinatorNotes.Prepare == nil {
		return prompt, onDispatched
	}
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return prompt, onDispatched
	}
	prompt, accepted := s.withCoordinatorNotes(ctx, dbTask, turnID, prompt, options.firstLaunchPromptContext)
	if accepted == nil {
		return prompt, onDispatched
	}
	return prompt, func() {
		onDispatched()
		accepted()
	}
}

// chainInitialPromptAccepted runs next after prev when the provider accepts
// the initial prompt.
func chainInitialPromptAccepted(prev func(string), next func()) func(string) {
	return func(executionID string) {
		if prev != nil {
			prev(executionID)
		}
		next()
	}
}
