package coordinator

import (
	"context"
	"errors"
	"strings"
	"time"
)

// WakeSources is the narrow read surface the recorder and the backstop use for
// task, session and message state. A read error is never "absent".
type WakeSources interface {
	// PrimarySessionID is "" when the task has no primary session.
	PrimarySessionID(ctx context.Context, taskID string) (string, error)
	// PendingQuestionID is the pending id of the session's clarification bundle, "" when none.
	PendingQuestionID(ctx context.Context, sessionID string) (string, error)
	// PendingPermissionIDs are the pending permission ids in message created_at, id order.
	PendingPermissionIDs(ctx context.Context, sessionID string) ([]string, error)
	// ActiveErrorStamp is the stamp of the session's active error, "" when none.
	ActiveErrorStamp(ctx context.Context, sessionID string) (string, error)
	TaskState(ctx context.Context, taskID string) (string, error)
	// LastActivityAt is nil when the task's status summary has none.
	LastActivityAt(ctx context.Context, taskID string) (*time.Time, error)
}

const (
	taskStateCompleted  = "COMPLETED"
	completedEpisodeKey = "completed"
	// stallKeyLayout is fixed width: nine fraction digits, never trimmed.
	stallKeyLayout = "2006-01-02T15:04:05.000000000Z"
)

// wakeEpisode is one condition read back from stored state.
type wakeEpisode struct {
	Kind WakeKind
	Key  string
}

// readEpisodes reads the named kinds for one task from stored state, in the
// order given, keeping only episodes with a non-blank key. A task with no
// primary session has no question, permission or error episode. Any read error
// is returned and no partial result is used.
func (s *Service) readEpisodes(ctx context.Context, src WakeSources, workspaceID, taskID string, kinds []WakeKind) ([]wakeEpisode, error) {
	sessionID, err := s.primarySessionFor(ctx, src, taskID, kinds)
	if err != nil {
		return nil, err
	}
	var out []wakeEpisode
	for _, kind := range kinds {
		keys, err := s.readKind(ctx, src, kind, workspaceID, taskID, sessionID)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if strings.TrimSpace(key) == "" {
				s.logger.Debug("blank wake episode key ignored")
				continue
			}
			out = append(out, wakeEpisode{Kind: kind, Key: key})
		}
	}
	return out, nil
}

func (s *Service) primarySessionFor(ctx context.Context, src WakeSources, taskID string, kinds []WakeKind) (string, error) {
	for _, kind := range kinds {
		if kind == WakeKindQuestion || kind == WakeKindPermission || kind == WakeKindError {
			return src.PrimarySessionID(ctx, taskID)
		}
	}
	return "", nil
}

func (s *Service) readKind(ctx context.Context, src WakeSources, kind WakeKind, workspaceID, taskID, sessionID string) ([]string, error) {
	switch kind {
	case WakeKindQuestion:
		if sessionID == "" {
			return nil, nil
		}
		key, err := src.PendingQuestionID(ctx, sessionID)
		return []string{key}, err
	case WakeKindPermission:
		if sessionID == "" {
			return nil, nil
		}
		return src.PendingPermissionIDs(ctx, sessionID)
	case WakeKindError:
		if sessionID == "" {
			return nil, nil
		}
		key, err := src.ActiveErrorStamp(ctx, sessionID)
		return []string{key}, err
	case WakeKindStall:
		key, err := s.currentStallKey(ctx, src, workspaceID, taskID)
		return []string{key}, err
	case WakeKindCompleted:
		state, err := src.TaskState(ctx, taskID)
		if err != nil || state != taskStateCompleted {
			return nil, err
		}
		return []string{completedEpisodeKey}, nil
	}
	return nil, nil
}

// currentStallKey is the key of the task's stall row while it is current: the
// task's last activity is absent or not later than the row's detected_at. No
// row, or a row the task has since resumed from, is "" (no episode); every
// other error is a read error.
func (s *Service) currentStallKey(ctx context.Context, src WakeSources, workspaceID, taskID string) (string, error) {
	stall, err := s.store.GetStall(ctx, workspaceID, taskID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	last, err := src.LastActivityAt(ctx, taskID)
	if err != nil {
		return "", err
	}
	if last != nil && last.After(stall.DetectedAt) {
		return "", nil
	}
	return stall.LastEventAt.UTC().Format(stallKeyLayout), nil
}
