package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// Only canonical typed events become observations. Free-form error messages and
// question bodies are deliberately absent from this projection.
func (s *Service) observeFriction(ctx context.Context, b *models.AssistantBinding, task string, sources []models.AttentionSource) error {
	if !s.AssistantEnabled {
		return nil
	}
	sessions, err := s.Tasks.ListTaskSessions(ctx, task)
	if err != nil {
		return err
	}
	byID := map[string]*taskmodels.TaskSession{}
	for _, session := range sessions {
		if session.TaskID == task {
			byID[session.ID] = session
		}
	}
	if native, readErr := s.Tasks.GetTask(ctx, task); readErr == nil && native.WorkspaceID == b.WorkspaceID {
		byID[""] = &taskmodels.TaskSession{TaskID: task, ExecutionProfileID: native.AssigneeAgentProfileID}
	}
	var failures []error
	for _, source := range sources {
		row, ok := nativeFriction(source, byID[source.SessionID])
		if !ok {
			continue
		}
		row.TaskID = task
		row.AccountRevision, err = s.contextProfileRevision(ctx, b.WorkspaceID, row.ProfileID)
		if err != nil || !frictionMatchesProfile(row, byID[source.SessionID]) {
			continue // A deleted or inaccessible execution identity is not regrouped.
		}
		if err = s.Repo.RecordFriction(ctx, b, row, s.attentionNow()); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func frictionMatchesProfile(row models.Friction, session *taskmodels.TaskSession) bool {
	revision, err := time.Parse(time.RFC3339Nano, row.AccountRevision)
	if err != nil || row.ObservedAt.IsZero() || row.ObservedAt.Before(revision) {
		return false
	}
	return session == nil || session.StartedAt.IsZero() || !session.StartedAt.Before(revision)
}

func nativeFriction(source models.AttentionSource, session *taskmodels.TaskSession) (models.Friction, bool) {
	if session == nil || (source.State != models.AttentionPending && (source.State != statusResolved || source.Friction == nil)) {
		return models.Friction{}, false
	}
	row := models.Friction{SessionID: session.ID, ProfileID: session.ExecutionProfileID,
		Origin: "native", PolicyVersion: statusUnknown, Outcome: "blocked"}
	if row.ProfileID == "" {
		row.ProfileID = session.AgentProfileID
	}
	if source.ObservedAt != nil {
		row.ObservedAt = *source.ObservedAt
	}
	switch source.Kind {
	case attentionKindQuestion:
		row.Operation, row.Reason, row.Cause = "question", "pending_approval", "question_request"
	case attentionKindPermission:
		row.Origin, row.Operation, row.Reason, row.Cause = "provider", attentionKindPermission, "pending_approval", "permission_request"
	case attentionKindAuthentication:
		row.Origin, row.Operation, row.Reason, row.Cause = attentionKindAuthentication, "launch", "authentication_required", "authentication_required"
	default:
		if source.Friction == nil {
			return models.Friction{}, false
		}
	}
	if cause := source.Friction; cause != nil {
		row.Origin, row.Operation, row.Reason, row.Cause = cause.Origin, cause.Operation, cause.Reason, cause.Cause
	}
	identity := session.TaskID + ":" + session.ID + ":" + source.Kind + ":" + source.SourceID + ":" + row.Cause
	if source.Kind == attentionKindAuthentication || source.Kind == attentionKindFailure {
		identity += ":" + source.SourceRevision
	}
	row.OccurrenceID = fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	return row, row.ProfileID != ""
}
