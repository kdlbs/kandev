// Package service orchestrates persisted user-terminal metadata with the
// agentctl PTY backend.
//
// Scope: only ordinary user terminals are managed here. The hardcoded
// `bottom-panel` terminal (cmd+J) and script terminals (id prefix `script-`)
// are explicitly out of scope — guards reject them, and the WS layer passes
// those calls through to the original (agentctl-direct) code path unchanged.
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/terminal/models"
	"github.com/kandev/kandev/internal/terminal/repository"
)

// BottomPanelID is the fixed terminal id used by the cmd+J panel. Never
// renamed, never persisted in the user_terminals table.
const BottomPanelID = "bottom-panel"

// PTYStatus values mirror the wire DTO. "running" if the PTY is alive on
// the agentctl side; "stopped" otherwise.
const (
	PTYStatusRunning = "running"
	PTYStatusStopped = "stopped"
)

// Kind discriminator for list items. Only "ordinary" is owned by this
// service; the WS layer adds "fixed" (bottom-panel) and "script" items
// alongside what this service returns.
const (
	KindOrdinary = "ordinary"
)

// ErrNotManaged is returned by rename/park/resume/destroy when the caller
// hands in a non-ordinary terminal id (bottom-panel or script-*). The WS
// layer should never call those methods for those ids; this error catches
// misuse loudly.
var ErrNotManaged = errors.New("terminal id is not managed by the terminal service")

// ErrTaskMismatch is returned by mutating ops when the supplied task_id
// does not own the supplied terminal_id. Defends against cross-task
// rename/park/destroy by raw terminal id.
var ErrTaskMismatch = errors.New("terminal does not belong to the supplied task")

// PTYBackend is the slice of agentctl's interactive runner that the service
// needs. Register pre-creates the agentctl-side entry so the next WS stream
// connection can lazily start the PTY; Stop tears down the PTY; IsAlive is
// a cheap probe.
type PTYBackend interface {
	Register(scopeID, terminalID string)
	Stop(ctx context.Context, scopeID, terminalID string) error
	IsAlive(scopeID, terminalID string) bool
}

// Service is the single entry point for all user-terminal RPCs.
type Service struct {
	repo *repository.Repository
	pty  PTYBackend
	log  *logger.Logger
}

// New constructs a Service.
func New(repo *repository.Repository, pty PTYBackend, log *logger.Logger) *Service {
	return &Service{repo: repo, pty: pty, log: log}
}

// IsManaged reports whether the terminal id corresponds to an ordinary
// user terminal (i.e. one this service is responsible for).
func IsManaged(id string) bool {
	if id == BottomPanelID {
		return false
	}
	if strings.HasPrefix(id, "script-") {
		return false
	}
	return true
}

// ListItem is the wire shape returned to the frontend.
type ListItem struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Seq            int     `json:"seq"`
	DisplayName    string  `json:"display_name"`
	CustomName     *string `json:"custom_name"`
	State          string  `json:"state"`
	PTYStatus      string  `json:"pty_status"`
	InitialCommand string  `json:"initial_command,omitempty"`
}

// Create inserts a new ordinary user terminal for taskID. Generates the
// stable id (also used as the agentctl PTY id) and pre-registers it on the
// PTY backend so the lazy-start path on first WS stream connect finds the
// entry.
func (s *Service) Create(ctx context.Context, taskID, envID, initialCommand string) (*models.Terminal, error) {
	id := "shell-" + uuid.New().String()
	term, err := s.repo.Create(ctx, taskID, envID, id, initialCommand)
	if err != nil {
		return nil, err
	}
	if s.pty != nil {
		s.pty.Register(envID, id)
	}
	return term, nil
}

// List returns ordinary terminals for the task, with PTY liveness merged in.
// includeParked controls whether parked rows appear. Frontend filters them
// out of the main strip but uses includeParked=true for the "Parked
// terminals" submenu.
func (s *Service) List(ctx context.Context, taskID string, includeParked bool) ([]ListItem, error) {
	rows, err := s.repo.ListByTask(ctx, taskID, includeParked)
	if err != nil {
		return nil, err
	}
	items := make([]ListItem, 0, len(rows))
	for _, r := range rows {
		ptyStatus := PTYStatusStopped
		if s.pty != nil && s.pty.IsAlive(r.EnvironmentID, r.ID) {
			ptyStatus = PTYStatusRunning
		}
		items = append(items, ListItem{
			ID:             r.ID,
			Kind:           KindOrdinary,
			Seq:            r.Seq,
			DisplayName:    r.DisplayName(),
			CustomName:     r.CustomName,
			State:          string(r.State),
			PTYStatus:      ptyStatus,
			InitialCommand: r.InitialCommand,
		})
	}
	return items, nil
}

// requireOwnership loads the terminal and verifies it belongs to taskID.
// Returns ErrTaskMismatch on a cross-task attempt and ErrNotManaged when
// the id falls outside the managed-id set.
//
// taskID is required; the previous "empty taskID skips" carve-out was
// dropped because the WS handlers pass user-controlled task_id directly
// to this helper, making the bypass an unauthenticated escape hatch.
// Internal task-scoped cleanup paths (CleanupTask) walk the repo
// directly and never call this method.
func (s *Service) requireOwnership(ctx context.Context, taskID, id string) (*models.Terminal, error) {
	if !IsManaged(id) {
		return nil, ErrNotManaged
	}
	if taskID == "" {
		return nil, ErrTaskMismatch
	}
	term, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if term.TaskID != taskID {
		return nil, ErrTaskMismatch
	}
	return term, nil
}

// Rename sets or clears the custom_name on an ordinary terminal. Pass nil
// to clear (revert to the derived "Terminal N" label). taskID is checked
// against the terminal's task_id; cross-task rename returns ErrTaskMismatch.
func (s *Service) Rename(ctx context.Context, taskID, id string, name *string) error {
	if _, err := s.requireOwnership(ctx, taskID, id); err != nil {
		return err
	}
	return s.repo.Rename(ctx, id, name)
}

// Park hides a terminal tab; the PTY is intentionally left running.
func (s *Service) Park(ctx context.Context, taskID, id string) error {
	if _, err := s.requireOwnership(ctx, taskID, id); err != nil {
		return err
	}
	return s.repo.SetState(ctx, id, models.StateParked)
}

// Resume returns a parked terminal to the visible state. PTY status is
// returned by a subsequent List call.
func (s *Service) Resume(ctx context.Context, taskID, id string) error {
	if _, err := s.requireOwnership(ctx, taskID, id); err != nil {
		return err
	}
	return s.repo.SetState(ctx, id, models.StateOpen)
}

// Destroy stops the PTY and deletes the DB row. Irreversible.
//
// PTY stop errors are not fatal — agentctl may have already torn the
// process down (e.g. after a container restart), so a missing PTY isn't
// reason to leave the DB row behind. The error is logged at Warn so
// operators can spot "terminal row gone, PTY still running" situations
// without it becoming an opaque 500 to the caller.
func (s *Service) Destroy(ctx context.Context, taskID, id string) error {
	term, err := s.requireOwnership(ctx, taskID, id)
	if err != nil {
		// Already-deleted rows are not a destroy failure — the caller
		// got what they asked for. Cross-task and not-managed errors
		// still surface.
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if s.pty != nil {
		if stopErr := s.pty.Stop(ctx, term.EnvironmentID, id); stopErr != nil {
			s.logWarn("pty stop on destroy (best-effort)",
				zap.String("terminal_id", id),
				zap.String("environment_id", term.EnvironmentID),
				zap.Error(stopErr))
		}
	}
	return s.repo.Delete(ctx, id)
}

// CleanupTask is called by the task.deleted/archived event subscriber. It
// stops every ordinary PTY for the task and deletes the rows. Stop
// failures are logged but don't block deletion — the DB row is the source
// of truth for "the user expects this terminal gone".
func (s *Service) CleanupTask(ctx context.Context, taskID string) (int, error) {
	rows, err := s.repo.ListByTask(ctx, taskID, true)
	if err != nil {
		return 0, err
	}
	if s.pty != nil {
		for _, r := range rows {
			if stopErr := s.pty.Stop(ctx, r.EnvironmentID, r.ID); stopErr != nil {
				s.logWarn("pty stop on cleanup (best-effort)",
					zap.String("task_id", taskID),
					zap.String("terminal_id", r.ID),
					zap.Error(stopErr))
			}
		}
	}
	return s.repo.DeleteByTask(ctx, taskID)
}

// logWarn is a nil-safe wrapper around s.log.Warn; service unit tests
// construct the service with a nil logger.
func (s *Service) logWarn(msg string, fields ...zap.Field) {
	if s.log == nil {
		return
	}
	s.log.Warn(msg, fields...)
}
