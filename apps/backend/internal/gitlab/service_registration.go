package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

type TaskMRObservationCommitter func(context.Context, *TaskMR, bool, int64) (*TaskMR, bool, error)

// GetTaskMRByRegistrationIdentityIncludingDetached reads the durable
// association row used to reconcile message status after a missed detach
// event.
func (s *Service) GetTaskMRByRegistrationIdentityIncludingDetached(
	ctx context.Context, taskID, connectionScope, providerRepositoryID, host string, iid int,
) (*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errors.New("gitlab store not configured")
	}
	return store.GetTaskMRByProviderIdentity(ctx, taskID, connectionScope, providerRepositoryID, host, iid)
}

// RegisterExistingMRByURL records an exact, verified merge request as an
// observation-only task association. Explicit restoration is revision-fenced.
func (s *Service) RegisterExistingMRByURL(
	ctx context.Context,
	workspaceID, taskID, repositoryID, mrURL string,
	restoreDetached bool,
	operationID string,
) (*TaskMR, error) {
	return s.RegisterExistingMRByURLWithCommit(ctx, workspaceID, taskID, repositoryID, mrURL,
		restoreDetached, operationID, func(ctx context.Context, registration *TaskMR, restore bool, expectedRevision int64) (*TaskMR, bool, error) {
			store := s.requireStore()
			if store == nil {
				return nil, false, errors.New("gitlab store not configured")
			}
			return store.RegisterTaskMRObservation(ctx, registration, restore, expectedRevision)
		})
}

// RegisterExistingMRByURLWithCommit separates provider reads from the
// caller-owned association commit. The committer runs only after the MR
// identity has been verified and must return after its transaction commits.
func (s *Service) RegisterExistingMRByURLWithCommit(
	ctx context.Context,
	workspaceID, taskID, repositoryID, mrURL string,
	restoreDetached bool,
	operationID string,
	commit TaskMRObservationCommitter,
) (*TaskMR, error) {
	return s.registerExistingMRByURLWithCommit(ctx, workspaceID, taskID, repositoryID, mrURL,
		restoreDetached, operationID, nil, commit)
}

// RegisterExistingMRByURLAtRevision keeps the association fence captured when
// a user selected a message reference. Recovery cannot advance the fence after
// a concurrent unlink.
func (s *Service) RegisterExistingMRByURLAtRevision(
	ctx context.Context, workspaceID, taskID, repositoryID, mrURL string,
	expectedRevision int64, restoreDetached bool, operationID string, commit TaskMRObservationCommitter,
) (*TaskMR, error) {
	return s.registerExistingMRByURLWithCommit(ctx, workspaceID, taskID, repositoryID, mrURL,
		restoreDetached, operationID, &expectedRevision, commit)
}

func (s *Service) registerExistingMRByURLWithCommit(
	ctx context.Context,
	workspaceID, taskID, repositoryID, mrURL string,
	restoreDetached bool,
	operationID string,
	acceptedRevision *int64,
	commit TaskMRObservationCommitter,
) (*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errors.New("gitlab store not configured")
	}
	if commit == nil {
		return nil, errors.New("GitLab registration committer is required")
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	target, err := resolveTaskMRRegistrationTarget(ctx, store, client, workspaceID, taskID, repositoryID, mrURL)
	if err != nil {
		return nil, err
	}
	if restoreDetached && strings.TrimSpace(operationID) == "" {
		return nil, errors.New("explicit restoration requires an operation ID")
	}
	current, expectedRevision, err := taskMRRegistrationFence(ctx, store, taskID, target, acceptedRevision, restoreDetached)
	if err != nil {
		return nil, err
	}
	if current != nil && current.DetachedAt == nil {
		return current, nil
	}
	status, err := fetchTaskMRForRegistration(ctx, client, target)
	if err != nil {
		return nil, err
	}
	if operationID == "" {
		operationID = fmt.Sprintf("agent-registration:%s:%d", taskID, time.Now().UTC().UnixNano())
	}
	registration := taskMRObservationFromStatus(taskID, target, operationID, expectedRevision, status)
	registered, changed, err := commit(ctx, registration, restoreDetached, expectedRevision)
	if err != nil {
		return nil, err
	}
	if changed {
		s.publishRegisteredTaskMR(ctx, workspaceID, registered)
	}
	return registered, nil
}

type taskMRRegistrationTarget struct {
	repositoryID         string
	projectPath          string
	host                 string
	providerRepositoryID string
	connectionScope      string
	iid                  int
}

func resolveTaskMRRegistrationTarget(
	ctx context.Context,
	store *Store,
	client Client,
	workspaceID, taskID, repositoryID, mrURL string,
) (taskMRRegistrationTarget, error) {
	projectPath, iid, err := parseMRURLForHost(mrURL, client.Host())
	if err != nil {
		return taskMRRegistrationTarget{}, err
	}
	repositoryID, err = store.ResolveTaskMRRepository(ctx, workspaceID, taskID, repositoryID)
	if err != nil {
		return taskMRRegistrationTarget{}, err
	}
	if err := store.ValidateTaskMRRepositoryIdentity(ctx, workspaceID, taskID, repositoryID, client.Host(), projectPath); err != nil {
		return taskMRRegistrationTarget{}, err
	}
	providerRepositoryID, err := store.TaskMRProviderRepositoryID(ctx, workspaceID, taskID, repositoryID)
	if err != nil {
		return taskMRRegistrationTarget{}, err
	}
	if providerRepositoryID == "" {
		return taskMRRegistrationTarget{}, ErrTaskMRRegistrationIdentityUnresolved
	}
	return taskMRRegistrationTarget{
		repositoryID: repositoryID, projectPath: projectPath, host: client.Host(),
		providerRepositoryID: providerRepositoryID, connectionScope: "builtin:" + workspaceID, iid: iid,
	}, nil
}

func taskMRRegistrationFence(
	ctx context.Context,
	store *Store,
	taskID string,
	target taskMRRegistrationTarget,
	acceptedRevision *int64,
	restoreDetached bool,
) (*TaskMR, int64, error) {
	current, err := store.GetTaskMRByProviderIdentity(
		ctx, taskID, target.connectionScope, target.providerRepositoryID, target.host, target.iid)
	if err != nil {
		return nil, 0, fmt.Errorf("read GitLab registration fence: %w", err)
	}
	if current == nil {
		if (acceptedRevision != nil && *acceptedRevision != 0) || restoreDetached {
			return nil, 0, ErrTaskMRRegistrationRevisionChanged
		}
		return nil, acceptedMRRegistrationRevision(acceptedRevision), nil
	}
	if current.ProviderRepositoryID == "" || current.ConnectionScope == "" ||
		current.RepositoryID != target.repositoryID ||
		!strings.EqualFold(strings.Trim(current.ProjectPath, "/"), target.projectPath) {
		return nil, 0, ErrTaskMRRegistrationIdentityUnresolved
	}
	if acceptedRevision != nil && current.AssociationRevision != *acceptedRevision {
		return nil, 0, ErrTaskMRRegistrationRevisionChanged
	}
	if current.DetachedAt != nil && !restoreDetached {
		return nil, 0, ErrTaskMRDetached
	}
	return current, current.AssociationRevision, nil
}

func fetchTaskMRForRegistration(ctx context.Context, client Client, target taskMRRegistrationTarget) (*MRStatus, error) {
	status, err := client.GetMRStatus(ctx, target.projectPath, target.iid)
	if err != nil {
		return nil, fmt.Errorf("fetch GitLab merge request for registration: %w", err)
	}
	if err := validateReturnedMRIdentity(status, target.host, target.projectPath, target.iid); err != nil {
		return nil, ErrTaskMRNotFound
	}
	remoteRepositoryID := firstGitLabID(status.MR.TargetProjectID, status.MR.ProjectID)
	if remoteRepositoryID <= 0 ||
		(status.MR.TargetProjectID > 0 && status.MR.ProjectID > 0 && status.MR.TargetProjectID != status.MR.ProjectID) ||
		positiveGitLabID(remoteRepositoryID) != target.providerRepositoryID {
		return nil, ErrTaskMRRepositoryMismatch
	}
	return status, nil
}

func taskMRObservationFromStatus(
	taskID string,
	target taskMRRegistrationTarget,
	operationID string,
	expectedRevision int64,
	status *MRStatus,
) *TaskMR {
	registration := taskMRFromStatus(taskID, target.repositoryID, target.host, target.projectPath, status)
	registration.ProviderRepositoryID = target.providerRepositoryID
	registration.ConnectionScope = target.connectionScope
	registration.RepositoryPath = target.projectPath
	registration.Source = TaskMRSourceAgentRegistration
	registration.ComparisonPolicy = TaskMRComparisonPolicyPreserve
	registration.AssociationRevision = expectedRevision
	registration.FirstOriginKind = "agent_registration"
	registration.FirstOriginID = operationID
	registration.FirstOriginRevision = strconv.FormatInt(expectedRevision, 10)
	return registration
}

func acceptedMRRegistrationRevision(revision *int64) int64 {
	if revision == nil {
		return 0
	}
	return *revision
}

func (s *Service) publishRegisteredTaskMR(ctx context.Context, workspaceID string, registered *TaskMR) {
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitLabTaskMRUpdated, eventSource, &TaskMRUpdatedEvent{
		WorkspaceID: workspaceID,
		TaskMR:      registered,
	})
	if err := s.eventBus.Publish(ctx, events.GitLabTaskMRUpdated, event); err != nil && s.logger != nil {
		s.logger.Debug("failed to publish registered GitLab MR update", zap.Error(err))
	}
}

func (s *Service) RegisterTaskMRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *TaskMR, restoreDetached bool, expectedRevision int64,
) (*TaskMR, bool, error) {
	store := s.requireStore()
	if store == nil {
		return nil, false, errors.New("gitlab store not configured")
	}
	return store.RegisterTaskMRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
}
