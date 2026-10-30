package orchestrator

import (
	"context"
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// ValidateBuiltinIntent checks typed queue admission without parsing prompt text.
func (s *Service) ValidateBuiltinIntent(ctx context.Context, taskID, sessionID, value string) error {
	_, err := s.PrepareBuiltinIntent(ctx, taskID, sessionID, value)
	return err
}

// PrepareBuiltinIntent binds a typed request to the execution authorized to receive it.
func (s *Service) PrepareBuiltinIntent(ctx context.Context, taskID, sessionID, value string) (context.Context, error) {
	intent := builtinintent.Intent(value)
	if err := intent.Validate(); err != nil {
		return ctx, err
	}
	if intent == "" {
		return ctx, nil
	}
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return ctx, err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ctx, err
	}
	if session == nil || session.TaskID != taskID {
		return ctx, ErrTaskSessionPairMismatch
	}
	binding := builtinintent.Binding{Intent: intent, TaskID: taskID, SessionID: sessionID}
	if session.State == models.TaskSessionStateCreated {
		profile, prepareErr := s.executor.PrepareTaskSessionMCPProfile(ctx, taskID, session, false)
		if prepareErr != nil {
			return ctx, prepareErr
		}
		if !builtinWalkthroughSelected(profile.BuiltinPackages) {
			return ctx, builtinintent.ErrUnavailable
		}
		return builtinintent.Bind(ctx, binding), nil
	}
	binding.ExecutionID, err = s.builtinIntentExecution(ctx, taskID, session)
	if err != nil {
		return ctx, err
	}
	return builtinintent.Bind(ctx, binding), nil
}

func (s *Service) builtinIntentExecution(ctx context.Context, taskID string, session *models.TaskSession) (string, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		return "", err
	}
	if running == nil || running.TaskID != taskID || running.SessionID != session.ID || running.AgentExecutionID == "" {
		return "", builtinintent.ErrUnavailable
	}
	history, ok := runtime.LoadMCPAttachmentHistory(session.Metadata[models.SessionMetaKeyMCPAttachmentState])
	if !ok {
		return "", builtinintent.ErrUnavailable
	}
	if binding := history.BuiltinBinding; binding != nil {
		task, loadErr := s.repo.GetTask(ctx, taskID)
		if loadErr != nil {
			return "", loadErr
		}
		if task != nil && binding.ExecutionID == running.AgentExecutionID && binding.Manifest.WorkspaceID == task.WorkspaceID && builtinWalkthroughSelected(binding.Manifest) {
			return running.AgentExecutionID, nil
		}
		return "", builtinintent.ErrUnavailable
	}
	if legacyWalkthroughObserved(history.Current, running.AgentExecutionID) {
		return running.AgentExecutionID, nil
	}
	return "", builtinintent.ErrUnavailable
}

func builtinWalkthroughSelected(manifest *p.BuiltinManifest) bool {
	return manifest != nil && manifest.Validate() == nil && slices.Contains(manifest.Packages, p.PackageWalkthroughs)
}

func (s *Service) prepareQueuedBuiltinIntent(ctx context.Context, queued *messagequeue.QueuedMessage) (context.Context, error) {
	intent, err := builtinintent.FromMetadata(queued.Metadata)
	if err != nil {
		return ctx, err
	}
	prepared, err := s.PrepareBuiltinIntent(ctx, queued.TaskID, queued.SessionID, string(intent))
	if err != nil && intent != "" {
		return ctx, errors.Join(builtinintent.ErrUnavailable, err)
	}
	return prepared, err
}

func legacyWalkthroughObserved(attempt streams.MCPAttachmentAttempt, executionID string) bool {
	if attempt.BuiltinPackages != nil || attempt.ExecutionID != executionID {
		return false
	}
	for _, server := range attempt.Servers {
		if server.Name != "kandev" || server.Source != streams.MCPServerSourceKandev || server.ToolsListedAt == nil {
			continue
		}
		for _, tool := range server.Tools {
			if tool.Name == "show_walkthrough_kandev" {
				return true
			}
		}
	}
	return false
}

func isBuiltinIntentError(err error) bool {
	return errors.Is(err, builtinintent.ErrUnavailable) || errors.Is(err, builtinintent.ErrInvalid)
}

// retainBuiltinIntent carries the requirement into a fallback queue, not its old execution grant.
func retainBuiltinIntent(ctx context.Context, taskID, sessionID string, metadata map[string]interface{}) error {
	binding, present := builtinintent.FromContext(ctx)
	if !present {
		return nil
	}
	if err := binding.Intent.Validate(); err != nil {
		return err
	}
	if binding.TaskID != taskID || binding.SessionID != sessionID || binding.Intent == "" {
		return builtinintent.ErrUnavailable
	}
	metadata[builtinintent.MetadataKey] = string(binding.Intent)
	return nil
}
