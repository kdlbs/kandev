package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/clarification"
)

// codeTargetReadFailed is the code of the HTTP 500 body returned when an edited
// answer cannot be checked because the live target could not be read.
const codeTargetReadFailed = "target_read_failed"

// ErrAnswerTargetRead reports that a task's answerable interactions could not
// be read. Nothing was changed by the failed read.
var ErrAnswerTargetRead = errors.New("coordinator: answer target read failed")

const targetReadDeadline = 5 * time.Second

// ClarificationTarget is a task's unresolved clarification bundle: its pending
// id and its questions in delivery order.
type ClarificationTarget struct {
	PendingID string
	SessionID string
	Questions []clarification.Question
}

// PermissionOptionTarget is one option of a pending permission request.
type PermissionOptionTarget struct {
	ID   string
	Name string
	Kind string
}

// PermissionTarget is a task's answerable pending permission request.
type PermissionTarget struct {
	PendingID string
	RequestID string
	SessionID string
	Title     string
	Summary   string
	Options   []PermissionOptionTarget
}

// TargetSet is one read of what a task is waiting for; each member is nil when
// absent.
type TargetSet struct {
	Clarification *ClarificationTarget
	Permission    *PermissionTarget
}

// AnswerTargets reads a task's current answerable interactions. It has no
// authorization of its own: callers check the task first.
type AnswerTargets interface {
	CurrentTargets(ctx context.Context, workspaceID, taskID string) (TargetSet, error)
}

// CurrentTargets implements AnswerTargets over the relay readers. A task that
// does not exist or is in another workspace is ErrNotFound; any other failure,
// including a bundle without readable questions, is ErrAnswerTargetRead.
func (s *Service) CurrentTargets(ctx context.Context, workspaceID, taskID string) (TargetSet, error) {
	if s.relayReader == nil || s.relayTasks == nil {
		return TargetSet{}, fmt.Errorf("%w: relay readers are not wired", ErrAnswerTargetRead)
	}
	ctx, cancel := context.WithTimeout(ctx, targetReadDeadline)
	defer cancel()
	if err := s.checkRelayTask(ctx, workspaceID, taskID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return TargetSet{}, ErrNotFound
		}
		return TargetSet{}, fmt.Errorf("%w: %v", ErrAnswerTargetRead, err)
	}
	targets, err := s.readTargets(ctx, taskID, true)
	if err != nil {
		return TargetSet{}, fmt.Errorf("%w: %v", ErrAnswerTargetRead, err)
	}
	var set TargetSet
	if targets.bundle != nil {
		questions := clarification.BundleQuestions(targets.messages)
		if len(questions) == 0 {
			return TargetSet{}, fmt.Errorf("%w: bundle %s has no readable questions", ErrAnswerTargetRead, targets.bundle.PendingID)
		}
		set.Clarification = &ClarificationTarget{PendingID: targets.bundle.PendingID, SessionID: targets.sessionID, Questions: questions}
	}
	if targets.permission != nil {
		set.Permission = permissionTarget(targets.permission.Content, targets.permission.Metadata, targets.sessionID)
	}
	return set, nil
}

func permissionTarget(title string, meta map[string]any, sessionID string) *PermissionTarget {
	requestID, _ := meta["request_id"].(string)
	pendingID, _ := meta["pending_id"].(string)
	t := &PermissionTarget{PendingID: pendingID, RequestID: requestID, SessionID: sessionID, Title: title, Summary: permissionSummary(meta)}
	raw, _ := meta["options"].([]any)
	for _, entry := range raw {
		data, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id, _ := data["option_id"].(string)
		if id == "" {
			continue
		}
		name, _ := data["name"].(string)
		kind, _ := data["kind"].(string)
		t.Options = append(t.Options, PermissionOptionTarget{ID: id, Name: name, Kind: kind})
	}
	return t
}

// permissionSummary is the compact JSON of the request's action type and
// details, empty when it has neither.
func permissionSummary(meta map[string]any) string {
	summary := map[string]any{}
	if v, ok := meta["action_type"].(string); ok && v != "" {
		summary["action_type"] = v
	}
	if v, ok := meta["action_details"]; ok && v != nil {
		summary["action_details"] = v
	}
	if len(summary) == 0 {
		return ""
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return ""
	}
	return string(raw)
}
