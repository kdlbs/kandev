// Package pause implements the Office workspace kill switch
// (docs/specs/office/requirements/workspace-kill-switch.md): a durable,
// workspace-scoped pause record that is authoritative on read, one
// exported gate predicate consulted at every Office launch site, and the
// best-effort halt sweep a confirmed pause triggers.
package pause

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// maxReasonCodePoints is the trimmed-reason length bound (Unicode code
// points, not bytes — a CJK reason would otherwise cap near 166 characters
// at a byte limit).
const maxReasonCodePoints = 500

// ErrReasonRequired is returned by Pause when the (trimmed) reason is empty.
var ErrReasonRequired = errors.New("pause reason is required")

// ErrReasonTooLong is returned by Pause or Resume when the trimmed reason
// exceeds maxReasonCodePoints Unicode code points.
var ErrReasonTooLong = errors.New("reason exceeds maximum length")

// ErrWorkspaceNotFound is returned by PauseState, Pause, and Resume when
// no workspace exists for the given id (AC-006.9).
var ErrWorkspaceNotFound = errors.New("workspace not found")

// ErrPauseContended is returned by Pause when a resume commits between
// this request's own insert-retry-once attempts, leaving no active pause
// record for either attempt to observe or return (AC-001.3's residual
// three-way race). The caller maps this to 409 with paused:false and no
// reason, distinct from the ordinary blocked-caller 409.
var ErrPauseContended = errors.New("pause request lost to a concurrent resume")

// Repository is the persistence surface Service needs. Defined locally
// (rather than importing a concrete type) per this codebase's narrow-
// consumer-interface convention; satisfied structurally by
// *office/repository/sqlite.Repository.
type Repository interface {
	GetActiveWorkspacePause(ctx context.Context, workspaceID string) (*models.WorkspacePause, error)
	CreateWorkspacePauseWithActivity(ctx context.Context, pause *models.WorkspacePause, activity *models.ActivityEntry) error
	ReleaseWorkspacePauseWithActivity(ctx context.Context, id, workspaceID, releasedBy, releasedByKind, releasedReason string) (bool, error)
	CreateActivityEntry(ctx context.Context, entry *models.ActivityEntry) error
	ListInflightRunsForWorkspace(ctx context.Context, workspaceID string) ([]models.InflightRun, error)
	ListLiveRoutineTaskIDsForWorkspace(ctx context.Context, workspaceID string) ([]string, error)
	CancelRunsForWorkspace(ctx context.Context, runIDs []string, reason string) (int64, error)
	ReleaseCheckoutsForWorkspace(ctx context.Context, runIDs []string) error
}

// TaskCanceller requests cancellation of a task's in-flight execution.
// Already declared as a consumer-side one-method interface twice elsewhere
// in this codebase (office/dashboard, office/service); this is the third.
type TaskCanceller interface {
	CancelTaskExecution(ctx context.Context, taskID string, reason string, force bool) error
}

// WorkspaceChecker resolves a workspace by id, used only for the
// existence check every pause/resume/read performs (AC-006.9). No Office
// table carries a foreign key to workspaces(id), so this check-then-act is
// the only enforcement of workspace identity in this feature.
type WorkspaceChecker interface {
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
}

// Service owns the office_workspace_pauses record, the gate read, the
// halt sweep, and the HTTP handlers (handler.go). It is the only writer
// of office_workspace_pauses.
type Service struct {
	repo       Repository
	canceller  TaskCanceller
	workspaces WorkspaceChecker
	logger     *logger.Logger
}

// NewService constructs the pause service.
func NewService(repo Repository, canceller TaskCanceller, workspaces WorkspaceChecker, log *logger.Logger) *Service {
	return &Service{repo: repo, canceller: canceller, workspaces: workspaces, logger: log}
}

// PauseState is the exported gate predicate every launch site consults.
// It satisfies office/shared.PauseGate structurally. Returns (nil, nil)
// when the workspace is running, the record when paused, and a non-nil
// error when the read itself failed — callers must fail closed on the
// error case, never treat it as "running".
func (s *Service) PauseState(ctx context.Context, workspaceID string) (*models.WorkspacePause, error) {
	return s.repo.GetActiveWorkspacePause(ctx, workspaceID)
}

// normalizeReason trims a reason and enforces the shared length bound.
// required controls whether an empty trimmed value is rejected (pause) or
// accepted as "" (resume, AC-004.8/-004.9).
func normalizeReason(raw string, required bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if required {
			return "", ErrReasonRequired
		}
		return "", nil
	}
	if utf8.RuneCountInString(trimmed) > maxReasonCodePoints {
		return "", ErrReasonTooLong
	}
	return trimmed, nil
}

// PauseResult is what Pause returns on every non-error path: the active
// pause record (the caller's own request if it won, or the pre-existing
// one on a repeat pause) and the halt sweep's counts.
type PauseResult struct {
	Pause *models.WorkspacePause
	Sweep SweepResult
}

// Pause verifies the workspace exists, validates the reason, resolves the
// actor, creates the pause record (or discovers it is already paused),
// and runs the halt sweep. See docs/specs/office/system-design/
// workspace-kill-switch-01.md "### Pause" for the full control flow this
// mirrors step for step, including the insert-retry-once contended path.
func (s *Service) Pause(ctx context.Context, workspaceID, reason, actorID, actorKind string) (*PauseResult, error) {
	if err := s.checkWorkspaceExists(ctx, workspaceID); err != nil {
		return nil, err
	}
	trimmedReason, err := normalizeReason(reason, true)
	if err != nil {
		return nil, err
	}

	active, err := s.attemptPause(ctx, workspaceID, trimmedReason, actorID, actorKind)
	if err != nil {
		return nil, err
	}

	sweep := s.runHaltSweep(ctx, workspaceID)
	return &PauseResult{Pause: active, Sweep: sweep}, nil
}

// attemptPause performs the insert-retry-once dance: try the insert; on a
// uniqueness violation, re-read the active record. A re-read that itself
// finds nothing means a resume raced in between, so retry the whole
// sequence exactly once more before surfacing ErrPauseContended.
func (s *Service) attemptPause(ctx context.Context, workspaceID, reason, actorID, actorKind string) (*models.WorkspacePause, error) {
	for attempt := 0; attempt < 2; attempt++ {
		active, done, err := s.tryPauseOnce(ctx, workspaceID, reason, actorID, actorKind)
		if err != nil {
			return nil, err
		}
		if done {
			return active, nil
		}
	}
	return nil, ErrPauseContended
}

// tryPauseOnce is one insert-or-discover attempt. done=false means the
// caller should retry (a resume raced the re-read); done=true with a nil
// error and non-nil pause means a result was reached.
func (s *Service) tryPauseOnce(ctx context.Context, workspaceID, reason, actorID, actorKind string) (*models.WorkspacePause, bool, error) {
	candidate := &models.WorkspacePause{
		WorkspaceID:   workspaceID,
		Reason:        reason,
		CreatedBy:     actorID,
		CreatedByKind: actorKind,
	}
	activity := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   models.ActivityActorType(actorKind),
		ActorID:     actorID,
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    workspaceID,
		Details:     reason,
	}
	err := s.repo.CreateWorkspacePauseWithActivity(ctx, candidate, activity)
	if err == nil {
		pauseCreatedTotal.Add(1)
		return candidate, true, nil
	}
	if !errors.Is(err, sqlite.ErrWorkspaceAlreadyPaused) {
		return nil, false, fmt.Errorf("create workspace pause: %w", err)
	}

	active, readErr := s.repo.GetActiveWorkspacePause(ctx, workspaceID)
	if readErr != nil {
		return nil, false, fmt.Errorf("re-read active pause: %w", readErr)
	}
	if active != nil {
		s.logNoop(ctx, workspaceID, actorID, actorKind, "pause")
		return active, true, nil
	}
	// No active record: a resume raced this insert. The caller retries once.
	s.logNoop(ctx, workspaceID, actorID, actorKind, "pause")
	return nil, false, nil
}

// logNoop writes a standalone (non-transactional, best-effort)
// workspace_pause_noop activity entry for a request that changed nothing:
// a repeat pause, or an insert that lost to a concurrent resume.
func (s *Service) logNoop(ctx context.Context, workspaceID, actorID, actorKind, requestedOp string) {
	entry := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   models.ActivityActorType(actorKind),
		ActorID:     actorID,
		Action:      models.ActivityActionWorkspacePauseNoop,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    workspaceID,
		Details:     requestedOp,
	}
	if err := s.repo.CreateActivityEntry(ctx, entry); err != nil {
		s.logger.Error("failed to log pause no-op activity",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

// ResumeResult is what Resume returns on every non-error path.
type ResumeResult struct {
	Released bool
}

// Resume verifies the workspace exists, validates the optional reason,
// reads the active record, and (if present) CAS-releases it. Idempotent:
// resuming an unpaused workspace, or losing a release race, both report
// success with Released=false and still write their audit entry
// (AC-005.6, F49 — mirrors Pause's "log, don't return" policy for a
// failed standalone audit write).
func (s *Service) Resume(ctx context.Context, workspaceID, reason, actorID, actorKind string) (*ResumeResult, error) {
	if err := s.checkWorkspaceExists(ctx, workspaceID); err != nil {
		return nil, err
	}
	trimmedReason, err := normalizeReason(reason, false)
	if err != nil {
		return nil, err
	}

	active, err := s.repo.GetActiveWorkspacePause(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("read active pause: %w", err)
	}
	if active == nil {
		s.logNoop(ctx, workspaceID, actorID, actorKind, "resume")
		return &ResumeResult{Released: false}, nil
	}

	released, err := s.repo.ReleaseWorkspacePauseWithActivity(
		ctx, active.ID, workspaceID, actorID, actorKind, trimmedReason,
	)
	if err != nil {
		return nil, fmt.Errorf("release workspace pause: %w", err)
	}
	if released {
		pauseReleasedTotal.Add(1)
	}
	return &ResumeResult{Released: released}, nil
}

// checkWorkspaceExists is the AC-006.9 guard every one of the three
// endpoints performs, because the scope middleware short-circuits when
// authentication is disabled (the default in every shipped profile).
func (s *Service) checkWorkspaceExists(ctx context.Context, workspaceID string) error {
	if _, err := s.workspaces.GetWorkspace(ctx, workspaceID); err != nil {
		return ErrWorkspaceNotFound
	}
	return nil
}
