package service

import "context"

// ShellLaunchInfo is the trusted server-side metadata needed to open an
// ordinary terminal. InitialCommand stays out of terminal-list wire DTOs.
type ShellLaunchInfo struct {
	Label              string
	InitialCommand     string
	WorkingDir         string
	InitialCommandOnce bool
}

// OwnerForTerminal returns the persisted task owner of an ordinary terminal
// after validating that the terminal exists and belongs to environmentID.
// It never consumes a one-shot launch command, so authorization can run
// before the command is claimed.
//
// The terminal ID alone is never sufficient to cross task or environment
// boundaries: a missing row, an empty owner, and a different environment all
// produce ErrTaskMismatch.
func (s *Service) OwnerForTerminal(ctx context.Context, environmentID, id string) (string, error) {
	if !IsManaged(id) {
		return "", ErrNotManaged
	}
	if environmentID == "" {
		return "", ErrTaskMismatch
	}
	term, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if term.TaskID == "" || term.EnvironmentID != environmentID {
		return "", ErrTaskMismatch
	}
	return term.TaskID, nil
}

// ShellLaunchInfo returns launch metadata for an ordinary terminal owned by
// taskID and environmentID. The terminal ID alone is never sufficient to
// cross task or environment boundaries.
func (s *Service) ShellLaunchInfo(ctx context.Context, taskID, environmentID, id string) (ShellLaunchInfo, error) {
	term, err := s.requireOwnership(ctx, taskID, id)
	if err != nil {
		return ShellLaunchInfo{}, err
	}
	if term.EnvironmentID != environmentID {
		return ShellLaunchInfo{}, ErrTaskMismatch
	}
	return ShellLaunchInfo{
		Label: term.DisplayName(), InitialCommand: term.InitialCommand,
		WorkingDir: term.WorkingDir, InitialCommandOnce: term.InitialCommandOnce,
	}, nil
}

// ClaimShellLaunchInfo returns launch metadata and atomically claims an
// explicitly one-shot command. Reconnects still receive the terminal label,
// but never receive a consumed authentication command.
func (s *Service) ClaimShellLaunchInfo(ctx context.Context, taskID, environmentID, id string) (ShellLaunchInfo, error) {
	term, err := s.requireOwnership(ctx, taskID, id)
	if err != nil {
		return ShellLaunchInfo{}, err
	}
	if term.EnvironmentID != environmentID {
		return ShellLaunchInfo{}, ErrTaskMismatch
	}
	initialCommand := term.InitialCommand
	if term.InitialCommandOnce {
		claimed, err := s.repo.ConsumeInitialCommandOnce(ctx, taskID, environmentID, id)
		if err != nil {
			return ShellLaunchInfo{}, err
		}
		if !claimed {
			initialCommand = ""
		}
	}
	return ShellLaunchInfo{
		Label: term.DisplayName(), InitialCommand: initialCommand,
		WorkingDir: term.WorkingDir, InitialCommandOnce: term.InitialCommandOnce,
	}, nil
}
