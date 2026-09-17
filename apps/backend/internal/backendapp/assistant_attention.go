package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/redaction"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const attentionQuestion = "question"

type attentionTasks interface {
	AuthorizeTaskAccess(context.Context, string) error
	GetTask(context.Context, string) (*taskmodels.Task, error)
	ListTaskSessions(context.Context, string) ([]*taskmodels.TaskSession, error)
	ListPendingInteractions(context.Context, taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error)
	GetInteraction(context.Context, string) (*taskmodels.Interaction, error)
}
type attentionPermissions interface {
	ListPendingAgentPermissions(context.Context, string, string) ([]streams.PendingAgentPermission, error)
}
type attentionQuestions interface {
	GetRequest(string) (*clarification.Request, bool)
}
type assistantAttentionReader struct {
	tasks       attentionTasks
	permissions attentionPermissions
	questions   attentionQuestions
}

func (a *assistantAttentionReader) ReadAttention(ctx context.Context, b *shared.AssistantBinding, taskID string, previous []shared.Attention) ([]shared.AttentionSource, error) {
	task, err := a.authorizedTask(ctx, b, taskID)
	if err != nil {
		return nil, err
	}
	sessions, err := a.tasks.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	pending, err := a.tasks.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{TaskIDs: []string{taskID}})
	if err != nil {
		return nil, err
	}
	live, err := a.permissions.ListPendingAgentPermissions(ctx, taskID, "")
	if err != nil {
		return nil, err
	}
	sources := []shared.AttentionSource{}
	for _, request := range pending {
		sources = append(sources, a.interactionSource(request, live))
	}
	sources = append(sources, taskAttentionSources(task, sessions)...)
	return a.completeAttentionSnapshot(ctx, taskID, sources, previous)
}
func (a *assistantAttentionReader) authorizedTask(ctx context.Context, b *shared.AssistantBinding, id string) (*taskmodels.Task, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID != b.OwnerUserID || a.tasks == nil || a.permissions == nil {
		return nil, fmt.Errorf("attention scope unavailable")
	}
	if err := a.tasks.AuthorizeTaskAccess(ctx, id); err != nil {
		return nil, err
	}
	task, err := a.tasks.GetTask(ctx, id)
	if err != nil || task == nil || task.WorkspaceID != b.WorkspaceID {
		return nil, fmt.Errorf("attention task unavailable")
	}
	return task, nil
}
func attentionDigest(value any) string {
	raw, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func attentionSafeSummary(value string) string {
	text := []rune(redaction.NewRedactor().String(value))
	return string(text[:min(len(text), 400)])
}
func (a *assistantAttentionReader) interactionSource(r *taskmodels.Interaction, live []streams.PendingAgentPermission) shared.AttentionSource {
	source := shared.AttentionSource{SourceID: r.ID, SessionID: r.SessionID, Kind: attentionQuestion, State: shared.AttentionPending, SourceRevision: attentionDigest(r), Summary: attentionSafeSummary(r.Title)}
	if r.Kind == taskmodels.InteractionKindPermission {
		source.Kind = "permission"
		source.Summary = "A native tool permission needs your decision."
	}
	if r.Status.IsTerminal() {
		source.State = attentionTerminalState(r.Status)
		return source
	}
	if r.Kind == taskmodels.InteractionKindPermission {
		source.State = shared.AttentionExpired
		for _, p := range live {
			if p.PendingID == r.ID && p.SessionID == r.SessionID && p.RequestID == r.RequestID {
				source.State = shared.AttentionPending
				break
			}
		}
	} else if a.questions == nil {
		source.State = "unknown"
	} else if request, ok := a.questions.GetRequest(r.ID); !ok || request.TaskID != r.TaskID || request.SessionID != r.SessionID {
		source.State = shared.AttentionExpired
	}
	return source
}
func attentionTerminalState(status taskmodels.InteractionStatus) string {
	switch status {
	case taskmodels.InteractionStatusAnswered, taskmodels.InteractionStatusApproved, taskmodels.InteractionStatusRejected:
		return "resolved"
	case taskmodels.InteractionStatusExpired:
		return shared.AttentionExpired
	case taskmodels.InteractionStatusCancelled:
		return "inactive"
	default:
		return "unknown"
	}
}
func (a *assistantAttentionReader) completeAttentionSnapshot(ctx context.Context, task string, sources []shared.AttentionSource, previous []shared.Attention) ([]shared.AttentionSource, error) {
	seen := map[string]bool{}
	for _, s := range sources {
		seen[s.SessionID+":"+s.Kind+":"+s.SourceID] = true
	}
	for _, old := range previous {
		if seen[old.SessionID+":"+old.Kind+":"+old.SourceID] {
			continue
		}
		source := old.AttentionSource
		if old.Kind == attentionQuestion || old.Kind == "permission" {
			canonical, err := a.tasks.GetInteraction(ctx, old.SourceID)
			if err != nil {
				return nil, err
			}
			source.State = shared.AttentionExpired
			if canonical != nil && canonical.TaskID == task && canonical.SessionID == old.SessionID {
				source.State = attentionTerminalState(canonical.Status)
				source.SourceRevision = attentionDigest(canonical)
			}
		} else {
			source.State = "inactive"
		}
		sources = append(sources, source)
	}
	return sources, nil
}
func taskAttentionSources(task *taskmodels.Task, sessions []*taskmodels.TaskSession) []shared.AttentionSource {
	sources := []shared.AttentionSource{}
	if task.State == "REVIEW" || task.State == v1.TaskStateCompleted {
		kind := "review"
		if task.State == v1.TaskStateCompleted {
			kind = "result"
		}
		sources = append(sources, shared.AttentionSource{SourceID: "task-state", Kind: kind, State: shared.AttentionPending, SourceRevision: attentionDigest(string(task.State)), Summary: "Task status changed; inspect current acceptance and review evidence."})
	}
	if e, ok := taskmodels.LoadTaskLaunchError(task.Metadata); ok {
		sources = append(sources, errorAttentionSource("", e.Stamp(), e.Code))
	}
	for _, session := range sessions {
		if session.TaskID != task.ID {
			continue
		}
		if e, ok := taskmodels.LoadLastAgentError(session.Metadata); ok && !e.IsDismissed() {
			sources = append(sources, errorAttentionSource(session.ID, e.Stamp(), e.Code))
			continue
		}
		switch session.State {
		case taskmodels.TaskSessionStateCompleted:
			sources = append(sources, shared.AttentionSource{SourceID: "session-result", SessionID: session.ID, Kind: "result", State: shared.AttentionPending, SourceRevision: attentionDigest(session.UpdatedAt), Summary: "A worker session finished. Review its result against the objective."})
		case taskmodels.TaskSessionStateFailed:
			sources = append(sources, errorAttentionSource(session.ID, session.UpdatedAt.String(), "session_failed"))
		}
	}
	return sources
}
func errorAttentionSource(session, stamp, code string) shared.AttentionSource {
	kind, summary := "failure", "A native task or session error needs attention. Open the task for details."
	if code == "auth_required" || code == "authentication_required" || code == "missing_credentials" {
		kind, summary = "authentication", "The selected provider requires authentication. Use the native account controls."
	}
	return shared.AttentionSource{SourceID: "active-error", SessionID: session, Kind: kind, State: shared.AttentionPending, SourceRevision: attentionDigest(stamp), Summary: summary}
}
