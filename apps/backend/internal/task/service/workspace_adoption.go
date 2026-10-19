package service

import (
	"context"
	"errors"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

type workspaceAdoptionRepository interface {
	AcquireTaskEnvironmentRecoveryClaim(context.Context, models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error)
	ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error
	CommitWorkspaceAdoption(context.Context, models.WorkspaceAdoptionPreview, string, *models.TaskEnvironmentRecoveryClaim, func() error) (*models.WorkspaceAdoptionReceipt, error)
	GetWorkspaceAdoption(context.Context, string) (*models.WorkspaceAdoptionReceipt, error)
	ValidateWorkspaceAdoption(context.Context, *models.WorkspaceAdoptionReceipt) error
	FinalizeWorkspaceAdoption(context.Context, *models.WorkspaceAdoptionReceipt) error
}

type WorkspaceAdoptionRequest struct {
	SessionID    string `json:"session_id"`
	SlotID       string `json:"slot_id"`
	CheckoutPath string `json:"checkout_path"`
}

func (s *Service) adoptionDependencies(ctx context.Context, taskID string) (workspaceAdoptionRepository, *worktree.Manager, error) {
	if _, agent := mcpscope.PrincipalFromContext(ctx); agent {
		return nil, nil, ErrForbidden
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || !identity.IsAdmin() {
		return nil, nil, ErrForbidden
	}
	if err := s.AuthorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceManage); err != nil {
		return nil, nil, err
	}
	repo, ok := s.taskEnvironments.(workspaceAdoptionRepository)
	if !ok {
		return nil, nil, errors.New("workspace adoption repository unavailable")
	}
	manager, ok := s.worktreeCleanup.(*worktree.Manager)
	if !ok {
		return nil, nil, errors.New("workspace adoption inspector unavailable")
	}
	return repo, manager, nil
}

func (s *Service) workspaceAdoptionSelection(ctx context.Context, taskID, sessionID, slotID string) (models.WorkspaceAdoptionSelection, error) {
	if err := s.workspaceAdoptionQuiescent(taskID); err != nil {
		return models.WorkspaceAdoptionSelection{}, err
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return models.WorkspaceAdoptionSelection{}, err
	}
	if session == nil || session.TaskID != taskID || session.State != models.TaskSessionStateWaitingForInput || session.QueueIncarnationID == "" {
		return models.WorkspaceAdoptionSelection{}, models.ErrWorkspaceAdoptionStale
	}
	env, task, err := s.adoptionEnvironment(ctx, taskID, session)
	if err != nil {
		return models.WorkspaceAdoptionSelection{}, err
	}
	repositories, err := s.adoptionRepositories(ctx, env, task.WorkspaceID)
	if err != nil {
		return models.WorkspaceAdoptionSelection{}, err
	}
	selection := models.WorkspaceAdoptionSelection{Snapshot: models.NewWorkspaceRecoverySelectionSnapshot(session, env, repositories), WorkspaceID: task.WorkspaceID, SessionIncarnation: session.QueueIncarnationID, SessionWorkspacePath: session.WorkspacePath}
	if !selection.Snapshot.Complete() {
		return selection, models.ErrWorkspaceAdoptionStale
	}
	if err := s.selectAdoptionAttachment(ctx, &selection, slotID); err != nil {
		return selection, err
	}
	return selection, nil
}

func (s *Service) selectAdoptionAttachment(ctx context.Context, selection *models.WorkspaceAdoptionSelection, slotID string) error {
	slot, err := (models.WorkspaceAdoptionPreview{Selection: *selection, SlotID: slotID}).Slot()
	if err != nil {
		return err
	}
	links, err := s.taskRepos.ListTaskRepositories(ctx, selection.Snapshot.TaskID)
	if err != nil {
		return err
	}
	repositories, err := s.resolveBaseBranchRepositories(ctx, links)
	if err != nil {
		return err
	}
	inputs := make([]worktree.BranchIdentityInput, len(links))
	attachments := make([]models.WorkspaceAdoptionAttachment, 0, len(links))
	for index, link := range links {
		defaultBranch := ""
		if repositories[index] != nil {
			defaultBranch = repositories[index].DefaultBranch
		}
		inputs[index] = worktree.BranchIdentityInput{RepositoryID: link.RepositoryID, BaseBranch: link.BaseBranch,
			CheckoutBranch: link.CheckoutBranch, DefaultBranch: defaultBranch,
			PRNumber: taskRepositoryPRNumber(link.Metadata), Position: link.Position}
		attachments = append(attachments, models.WorkspaceAdoptionAttachment{ID: link.ID, TaskID: link.TaskID,
			RepositoryID: link.RepositoryID, BaseBranch: link.BaseBranch, CheckoutBranch: link.CheckoutBranch,
			MetadataDigest: models.WorkspaceAdoptionMetadataDigest(link.Metadata), Position: link.Position})
	}
	plans := worktree.BuildBranchIdentityPlans(inputs)
	matches := 0
	for index, link := range links {
		if link.RepositoryID == slot.RepositoryID && plans[index].IdentitySlug == slot.BranchSlug {
			selection.AttachmentID = link.ID
			selection.AttachmentCheckoutBranch = link.CheckoutBranch
			selection.AttachmentBaseBranch = link.BaseBranch
			matches++
		}
	}
	if matches != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].ID < attachments[j].ID })
	selection.AttachmentsDigest = models.WorkspaceAdoptionDigest(attachments)
	return nil
}

func (s *Service) PreviewWorkspaceAdoption(ctx context.Context, taskID string, req WorkspaceAdoptionRequest) (*models.WorkspaceAdoptionPreview, error) {
	_, manager, err := s.adoptionDependencies(ctx, taskID)
	if err != nil {
		return nil, err
	}
	selection, err := s.workspaceAdoptionSelection(ctx, taskID, req.SessionID, req.SlotID)
	if err != nil {
		return nil, err
	}
	inspection, err := manager.InspectWorkspaceAdoption(ctx, selection, req.SlotID, req.CheckoutPath)
	if err != nil {
		return nil, err
	}
	defer inspection.Close()
	if err := inspection.Verify(ctx); err != nil {
		return nil, err
	}
	preview := &models.WorkspaceAdoptionPreview{Selection: selection, SlotID: req.SlotID, Proof: inspection.Proof}
	preview.Token = preview.Digest()
	return preview, nil
}

func (s *Service) ApplyWorkspaceAdoption(ctx context.Context, taskID string, p models.WorkspaceAdoptionPreview, operationID string) (result *models.WorkspaceAdoptionReceipt, resultErr error) {
	repo, manager, err := s.adoptionDependencies(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(operationID); err != nil {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	if p.Selection.Snapshot.TaskID != taskID || p.Token == "" || p.Token != p.Digest() {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	unlock, err := s.lockWorkspaceAdoption(ctx, taskID, p, manager)
	if err != nil {
		return nil, err
	}
	defer unlock()
	prior, err := repo.GetWorkspaceAdoption(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if prior != nil {
		if prior.Preview.Token != p.Token {
			return nil, models.ErrWorkspaceAdoptionStale
		}
		return s.finalizeWorkspaceAdoptionReplay(ctx, taskID, operationID, repo, manager)
	}
	snapshot := p.Selection.Snapshot
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{TaskEnvironmentID: snapshot.TaskEnvironmentID, OwnerTaskID: taskID, OwnershipGeneration: snapshot.OwnershipGeneration, SessionID: snapshot.SessionID, OperationID: operationID, ExecutorType: "worktree"})
	if err != nil {
		return nil, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if releaseErr := repo.ReleaseTaskEnvironmentRecoveryClaim(releaseCtx, claim); releaseErr != nil && !errors.Is(releaseErr, recoveryclaim.ErrClaimNotFound) {
			s.logger.Error("release workspace adoption claim", zap.Error(releaseErr))
			resultErr = errors.Join(resultErr, releaseErr)
		}
	}()
	return s.publishWorkspaceAdoption(ctx, taskID, p, operationID, repo, manager, claim)
}

func (s *Service) ValidateWorkspaceAdoption(ctx context.Context, taskID, operationID string) (*models.WorkspaceAdoptionReceipt, error) {
	repo, manager, err := s.adoptionDependencies(ctx, taskID)
	if err != nil {
		return nil, err
	}
	receipt, err := repo.GetWorkspaceAdoption(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if receipt == nil || receipt.Preview.Selection.Snapshot.TaskID != taskID {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	if err := repo.ValidateWorkspaceAdoption(ctx, receipt); err != nil {
		return nil, err
	}
	if err := manager.ValidateWorkspaceAdoptionProof(ctx, receipt.Preview); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (s *Service) workspaceAdoptionQuiescent(taskID string) error {
	if s.sessionExecutionRegistry != nil && len(s.sessionExecutionRegistry.LiveSessionIDsForTask(taskID)) != 0 {
		return ErrActiveTaskSessions
	}
	return nil
}

func (s *Service) adoptionEnvironment(ctx context.Context, taskID string, session *models.TaskSession) (*models.TaskEnvironment, *models.Task, error) {
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if env == nil || task == nil || task.ArchivedAt != nil || env.TaskID != taskID || env.ExecutorType != "worktree" || env.Status != models.TaskEnvironmentStatusReady || env.WorkspacePath != session.WorkspacePath {
		return nil, nil, models.ErrWorkspaceAdoptionStale
	}

	return env, task, nil
}

func (s *Service) adoptionRepositories(ctx context.Context, env *models.TaskEnvironment, workspaceID string) (map[string]*models.Repository, error) {
	repositories := make(map[string]*models.Repository)
	for _, row := range models.SelectedWorkspaceRecoveryRows(env) {
		repository, err := s.repoEntities.GetRepository(ctx, row.RepositoryID)
		if err != nil {
			return nil, err
		}
		if repository == nil || repository.DeletedAt != nil || repository.WorkspaceID != workspaceID {
			return nil, models.ErrWorkspaceAdoptionStale
		}
		repositories[row.RepositoryID] = repository
	}

	return repositories, nil
}

func (s *Service) publishWorkspaceAdoption(ctx context.Context, taskID string, p models.WorkspaceAdoptionPreview, operationID string, repo workspaceAdoptionRepository, manager *worktree.Manager, claim *models.TaskEnvironmentRecoveryClaim) (*models.WorkspaceAdoptionReceipt, error) {
	current, err := s.workspaceAdoptionSelection(ctx, taskID, p.Selection.Snapshot.SessionID, p.SlotID)
	if err != nil {
		return nil, err
	}
	if models.WorkspaceAdoptionDigest(current) != models.WorkspaceAdoptionDigest(p.Selection) {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	inspection, err := manager.InspectWorkspaceAdoption(ctx, current, p.SlotID, p.Proof.CandidatePath)
	if err != nil {
		return nil, err
	}
	defer inspection.Close()
	if models.WorkspaceAdoptionDigest(inspection.Proof) != models.WorkspaceAdoptionDigest(p.Proof) {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	receipt, err := repo.CommitWorkspaceAdoption(ctx, p, operationID, claim, func() error {
		if err := s.workspaceAdoptionQuiescent(taskID); err != nil {
			return err
		}
		return inspection.Verify(ctx)
	})
	if err == nil {
		slot, _ := p.Slot()
		manager.InvalidateWorkspaceAdoptionCache(slot.WorktreeID)
	}
	return receipt, err
}

func (s *Service) lockWorkspaceAdoption(ctx context.Context, taskID string, p models.WorkspaceAdoptionPreview, manager *worktree.Manager) (func(), error) {
	session, err := s.GetTaskSession(ctx, p.Selection.Snapshot.SessionID)
	if err != nil {
		return nil, err
	}
	if session.TaskID != taskID || session.TaskEnvironmentID != p.Selection.Snapshot.TaskEnvironmentID {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	return manager.TryLockWorkspaceAdoption(ctx, session.TaskEnvironmentID)
}

func (s *Service) finalizeWorkspaceAdoptionReplay(ctx context.Context, taskID, operationID string, repo workspaceAdoptionRepository, manager *worktree.Manager) (*models.WorkspaceAdoptionReceipt, error) {
	receipt, err := s.ValidateWorkspaceAdoption(ctx, taskID, operationID)
	if err != nil {
		return nil, err
	}
	slot, err := receipt.Preview.Slot()
	if err != nil {
		return nil, err
	}
	manager.InvalidateWorkspaceAdoptionCache(slot.WorktreeID)
	if err := repo.FinalizeWorkspaceAdoption(ctx, receipt); err != nil {
		return nil, err
	}
	return receipt, nil
}
