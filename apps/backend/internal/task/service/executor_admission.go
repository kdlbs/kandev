package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const legacyLocalExecutorID = "local_pc"

type executorUseAdmitter interface {
	AdmitExecutorUse(ctx context.Context, executorID, profileID, parentOwner, profileOwner, sessionID, actorID string) error
}

func (s *Service) SetExecutorIdentityResolver(resolve func(context.Context, string) (authn.Identity, bool)) {
	s.executorIdentityResolver = resolve
}

// ExecutorActorContext restores only explicit durable attribution. Unknown
// creators cannot inherit the workspace owner's or scheduler's authority.
func (s *Service) ExecutorActorContext(ctx context.Context, userID string) (context.Context, error) {
	if s.executorIdentityResolver == nil {
		return ctx, fmt.Errorf("executor actor resolver is unavailable")
	}
	identity, ok := s.executorIdentityResolver(ctx, userID)
	if !ok {
		return ctx, fmt.Errorf("executor actor is unavailable")
	}
	return authn.WithIdentity(ctx, identity), nil
}

// PrepareExecutorAdmission resolves the current user before authorizing the
// final profile and serializes that decision against ownership transfers.
func (s *Service) PrepareExecutorAdmission(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
	session, err := s.executorAdmissionSession(ctx, sessionID)
	if err != nil {
		return ctx, err
	}
	executorID, profileID, err = s.executorAdmissionTargets(ctx, session, executorID, profileID)
	if err != nil {
		return ctx, err
	}
	ctx = s.executorAdmissionActor(ctx, session)
	executorID, profileOwner, err := s.executorAdmissionProfile(ctx, executorID, profileID)
	if err != nil {
		return ctx, err
	}
	if executorID == "" {
		return ctx, nil
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil {
		return ctx, err
	}
	if err := s.authorizeExecutorOwners(ctx, executor.OwnerUserID, profileOwner); err != nil {
		return ctx, err
	}
	if session != nil {
		if err := s.authorizeTaskID(ctx, session.TaskID); err != nil {
			return ctx, err
		}
		if err := s.authorizeSessionExecutor(ctx, session); err != nil {
			return ctx, err
		}
		ctx, err = s.admitSessionEnvironment(ctx, session)
		if err != nil {
			return ctx, err
		}
	}
	admitter, ok := s.executors.(executorUseAdmitter)
	if !ok {
		if executor.OwnerUserID != "" || profileOwner != "" {
			return ctx, fmt.Errorf("executor ownership admission is unavailable")
		}
		return ctx, nil
	}
	identity, _ := s.executorCaller(ctx)
	actorID := identity.UserID
	if identity.Synthetic {
		actorID = ""
	}
	err = admitter.AdmitExecutorUse(ctx, executorID, profileID, executor.OwnerUserID, profileOwner, sessionID, actorID)
	return ctx, err
}

func (s *Service) executorAdmissionSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if sessionID == "" {
		return nil, nil
	}
	return s.sessions.GetTaskSession(ctx, sessionID)
}

func (s *Service) PrepareAutonomousExecutor(ctx context.Context, creatorID, profileID string) (context.Context, error) {
	executorID, err := s.autonomousExecutorID(ctx, profileID)
	if err != nil {
		return ctx, err
	}
	if _, interactive := authn.IdentityFromContext(ctx); interactive {
		if err := s.AuthorizeExecutorConfiguration(ctx, executorID, profileID); err != nil {
			return ctx, err
		}
	}
	creatorCtx := models.WithExecutorActor(ctx, creatorID)
	if restored, err := s.ExecutorActorContext(creatorCtx, creatorID); err == nil {
		creatorCtx = restored
	}
	if err := s.AuthorizeExecutorConfiguration(creatorCtx, executorID, profileID); err != nil {
		return creatorCtx, err
	}
	return creatorCtx, nil
}

func (s *Service) executorAdmissionActor(ctx context.Context, session *models.TaskSession) context.Context {
	if actor, bound := models.ExecutorActorFromContext(ctx); bound && actor == "" && session != nil {
		ctx = models.WithExecutorActor(ctx, session.ExecutorActorUserID)
	}
	if _, present := authn.IdentityFromContext(ctx); !present {
		actor := ""
		if session != nil {
			actor = session.ExecutorActorUserID
		}
		if restored, restoreErr := s.ExecutorActorContext(ctx, actor); restoreErr == nil {
			ctx = restored
		}
	}
	if identity, ok := s.executorCaller(ctx); ok {
		ctx = authn.WithIdentity(ctx, identity)
	}
	return ctx
}

func (s *Service) autonomousExecutorID(ctx context.Context, profileID string) (string, error) {
	if profileID != "" {
		return "", nil
	}
	workspaceID := models.ExecutorWorkspaceFromContext(ctx)
	if workspaceID == "" {
		return "", nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if workspace.DefaultExecutorID == nil {
		return "", nil
	}
	return *workspace.DefaultExecutorID, nil
}

func (s *Service) admitSessionEnvironment(ctx context.Context, session *models.TaskSession) (context.Context, error) {
	if session.TaskEnvironmentID == "" {
		return ctx, nil
	}
	environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return ctx, err
	}
	if environment == nil {
		return ctx, repoerrors.ErrTaskNotFound
	}
	return s.PrepareExecutorAdmission(ctx, "", environment.ExecutorID, environment.ExecutorProfileID)
}

func (s *Service) executorAdmissionTargets(ctx context.Context, session *models.TaskSession, executorID, profileID string) (string, string, error) {
	if session == nil {
		return executorID, profileID, nil
	}
	if executorID == "" {
		executorID = session.ExecutorID
	}
	if profileID == "" {
		profileID = session.ExecutorProfileID
	}
	if executorID != "" || profileID != "" || session.TaskEnvironmentID == "" {
		return executorID, profileID, nil
	}
	environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return "", "", err
	}
	if environment == nil {
		return "", "", repoerrors.ErrTaskNotFound
	}
	return environment.ExecutorID, environment.ExecutorProfileID, nil
}

func (s *Service) executorAdmissionProfile(ctx context.Context, executorID, profileID string) (string, string, error) {
	var profileOwner string
	executorID, err := s.executorAdmissionID(ctx, executorID)
	if err != nil {
		return "", "", err
	}
	if profileID != "" {
		profile, err := s.GetExecutorProfile(ctx, profileID)
		if err != nil {
			return "", "", err
		}
		profileOwner = profile.OwnerUserID
		if executorID != "" && executorID != profile.ExecutorID {
			return "", "", models.ErrExecutorOwnershipInvalid
		}
		executorID = profile.ExecutorID
	}
	return executorID, profileOwner, nil
}

// Legacy Office selections name a runtime kind, while ownership belongs to
// the built-in row. A real row with that ID remains authoritative.
func (s *Service) executorAdmissionID(ctx context.Context, id string) (string, error) {
	if id != legacyLocalExecutorID {
		return id, nil
	}
	_, readErr := s.executors.GetExecutor(ctx, id)
	if readErr == nil {
		return id, nil
	}
	if !errors.Is(readErr, models.ErrExecutorNotFound) {
		return "", readErr
	}
	history, ok := s.executors.(interface {
		HasExecutorRecord(context.Context, string) (bool, error)
	})
	if !ok {
		return "", readErr
	}
	exists, err := history.HasExecutorRecord(ctx, id)
	if err != nil {
		return "", err
	}
	if exists {
		return "", readErr
	}
	return models.ExecutorIDLocal, nil
}
