package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type acceptedGitHubProviderStub struct {
	fakeGitHubChangeLinks
	store           *github.Store
	registerErr     error
	registerCalls   int
	alreadyActivePR *github.TaskPR
	beforeCommit    func()
}

func (p *acceptedGitHubProviderStub) RegisterExistingPRByURLForWorkspaceAtRevision(
	ctx context.Context,
	workspaceID, userID, taskID, repositoryID, prURL string,
	expectedRevision int64,
	restoreDetached bool,
	operationID string,
	commit github.TaskPRObservationCommitter,
) (*github.TaskPR, error) {
	p.registerCalls++
	if p.registerErr != nil {
		return nil, p.registerErr
	}
	if p.alreadyActivePR != nil {
		return p.alreadyActivePR, nil
	}
	if p.beforeCommit != nil {
		p.beforeCommit()
	}
	registration := &github.TaskPR{
		TaskID: taskID, RepositoryID: repositoryID,
		Owner: "acme", Repo: "api", PRNumber: 17, PRURL: prURL, PRTitle: "Accepted PR",
		HeadBranch: "feature/registration", BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
		Source: github.TaskPRSourceAgentRegistration, ProviderHost: "https://github.com",
		ConnectionScope: "builtin:" + workspaceID, ProviderRepositoryID: "provider-repo-17",
		RepositoryPath: "acme/api", ComparisonPolicy: github.TaskPRComparisonPolicyPreserve,
		FirstOriginKind: "agent_registration", FirstOriginID: operationID,
	}
	registered, _, err := commit(ctx, registration, restoreDetached, expectedRevision)
	return registered, err
}

type acceptedGitLabProviderStub struct {
	fakeGitLabChangeLinks
	store         *gitlab.Store
	registerCalls int
	mutationCalls int
	beforeCommit  func()
}

func (p *acceptedGitLabProviderStub) RegisterExistingMRByURLAtRevision(
	ctx context.Context,
	workspaceID, taskID, repositoryID, mrURL string,
	expectedRevision int64,
	restoreDetached bool,
	operationID string,
	commit gitlab.TaskMRObservationCommitter,
) (*gitlab.TaskMR, error) {
	p.registerCalls++
	if p.beforeCommit != nil {
		p.beforeCommit()
	}
	registration := &gitlab.TaskMR{
		TaskID: taskID, RepositoryID: repositoryID,
		Host: "https://gitlab.example.test", ProjectPath: "group/project", MRIID: 17,
		MRURL: mrURL, MRTitle: "Accepted MR", HeadBranch: "feature/registration", BaseBranch: "main",
		State: "opened", Source: gitlab.TaskMRSourceAgentRegistration,
		ConnectionScope: "builtin:" + workspaceID, ProviderRepositoryID: "provider-repo-17",
		RepositoryPath: "group/project", ComparisonPolicy: gitlab.TaskMRComparisonPolicyPreserve,
		FirstOriginKind: "agent_registration", FirstOriginID: operationID, CreatedAt: time.Now().UTC(),
	}
	registered, _, err := commit(ctx, registration, restoreDetached, expectedRevision)
	return registered, err
}

func (p *acceptedGitLabProviderStub) RegisterTaskMRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *gitlab.TaskMR, restoreDetached bool, expectedRevision int64,
) (*gitlab.TaskMR, bool, error) {
	p.mutationCalls++
	if p.store != nil {
		return p.store.RegisterTaskMRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
	}
	registration.ID = "registered-gitlab-mr"
	registration.AssociationRevision = expectedRevision + 1
	return registration, true, nil
}

func (p *acceptedGitLabProviderStub) GetTaskMRByRegistrationIdentityIncludingDetached(
	ctx context.Context, taskID, connectionScope, providerRepositoryID, host string, iid int,
) (*gitlab.TaskMR, error) {
	if p.store == nil {
		return nil, nil
	}
	return p.store.GetTaskMRByProviderIdentity(ctx, taskID, connectionScope, providerRepositoryID, host, iid)
}

func testRegistrationActorResolver(_ context.Context, actorID string) (authn.Identity, bool) {
	return authn.Identity{UserID: actorID, Role: authn.RoleMember}, true
}

func TestTaskChangeRegistrationProcessorCanRestart(t *testing.T) {
	taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
	processor := newTaskChangeRegistrationProcessor(
		repository, taskSvc, taskChangeLinkCoordinator{}, db, nil, testRegistrationActorResolver,
	)

	processor.Start(context.Background())
	processor.Stop()
	processor.Start(context.Background())
	processor.Stop()
}

func (p *acceptedGitHubProviderStub) RegisterTaskPRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *github.TaskPR, restoreDetached bool, expectedRevision int64,
) (*github.TaskPR, bool, error) {
	return p.store.RegisterTaskPRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
}

func (p *acceptedGitHubProviderStub) GetTaskPRByRegistrationIdentityIncludingDetached(
	ctx context.Context, taskID, providerHost, connectionScope, providerRepositoryID string, prNumber int,
) (*github.TaskPR, error) {
	return p.store.GetTaskPRByRegistrationIdentityIncludingDetached(ctx, taskID, providerHost, connectionScope, providerRepositoryID, prNumber)
}

func TestTaskChangeRegistrationProcessorRetriesAcceptedMessage(t *testing.T) {
	ctx := context.Background()
	taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
	seedTaskChangeCoordinatorTask(t, repository, "ws-registration-worker", "task-registration-worker", "repo-registration-worker",
		"https://github.com", "acme", "api")
	if _, err := db.ExecContext(ctx, `UPDATE repositories SET provider_repo_id = 'provider-repo-17' WHERE id = ?`, "repo-registration-worker"); err != nil {
		t.Fatalf("set provider repository identity: %v", err)
	}
	store, err := github.NewStore(db, db)
	if err != nil {
		t.Fatalf("initialize GitHub store: %v", err)
	}
	message := acceptRegistrationTestMessage(t, ctx, repository, "task-registration-worker", "selection-worker")
	intent, err := repository.GetTaskChangeRegistrationIntentBySelection(ctx, message.TaskID, "selection-worker")
	if err != nil || intent == nil || intent.ActorID != "user-registration" || intent.ExpectedRevision == nil || *intent.ExpectedRevision != 0 {
		t.Fatalf("accepted intent = (%+v, err=%v)", intent, err)
	}

	provider := &acceptedGitHubProviderStub{store: store, registerErr: errors.New("temporary provider failure")}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, github: provider}
	processor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
	if err := processor.processIntent(ctx, intent); err != nil {
		t.Fatalf("process first registration attempt: %v", err)
	}
	failed, err := repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if err != nil || failed == nil || failed.State != "failed" || failed.ErrorCode != "provider_unavailable" {
		t.Fatalf("failed intent = (%+v, err=%v)", failed, err)
	}
	assertMessageRegistrationStatus(t, ctx, repository, message.ID, "failed")

	provider.registerErr = nil
	retried, replayed, err := repository.ActTaskChangeRegistration(
		ctx, intent.TaskID, intent.SelectionID, "retry-operation-1", intent.ActorID, "retry",
	)
	if err != nil || replayed || retried == nil || retried.State != "retryable" || retried.OperationID != intent.OperationID ||
		retried.ExpectedRevision == nil || *retried.ExpectedRevision != 0 {
		t.Fatalf("retried intent changed its accepted operation or fence: (%+v, replayed=%v, err=%v)", retried, replayed, err)
	}
	processor.processPending(ctx)
	succeeded, err := repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if err != nil || succeeded == nil || succeeded.State != "succeeded" || succeeded.AssociationRevision == nil || *succeeded.AssociationRevision != 1 {
		t.Fatalf("completed intent = (%+v, err=%v)", succeeded, err)
	}
	assertMessageRegistrationStatus(t, ctx, repository, message.ID, "linked")
	linked, err := store.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, message.TaskID, "https://github.com", "builtin:ws-registration-worker", "provider-repo-17", 17,
	)
	if err != nil || linked == nil || linked.Source != github.TaskPRSourceSelectedMention ||
		linked.ComparisonPolicy != github.TaskPRComparisonPolicyPreserve || linked.FirstOriginKind != "selected_mention" ||
		linked.FirstOriginID != message.ID || linked.AssociationRevision != 1 {
		t.Fatalf("committed observation = (%+v, err=%v)", linked, err)
	}
	if provider.registerCalls != 2 {
		t.Fatalf("provider registration attempts = %d, want one failed attempt and one retry", provider.registerCalls)
	}
}

func TestTaskChangeRegistrationProcessorPersistsRevisionForAlreadyActiveAssociation(t *testing.T) {
	ctx := context.Background()
	taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
	seedTaskChangeCoordinatorTask(t, repository, "ws-registration-existing", "task-registration-existing", "repo-registration-existing",
		"https://github.com", "acme", "api")
	if _, err := db.ExecContext(ctx, `UPDATE repositories SET provider_repo_id = 'provider-repo-17' WHERE id = ?`, "repo-registration-existing"); err != nil {
		t.Fatalf("set provider repository identity: %v", err)
	}
	store, err := github.NewStore(db, db)
	if err != nil {
		t.Fatalf("initialize GitHub store: %v", err)
	}
	message := acceptRegistrationTestMessage(t, ctx, repository, "task-registration-existing", "selection-existing")
	intent, err := repository.GetTaskChangeRegistrationIntentBySelection(ctx, message.TaskID, "selection-existing")
	if err != nil || intent == nil {
		t.Fatalf("accepted intent = (%+v, err=%v)", intent, err)
	}

	provider := &acceptedGitHubProviderStub{store: store, alreadyActivePR: &github.TaskPR{
		ID: "already-active", TaskID: message.TaskID, RepositoryID: "repo-registration-existing",
		Owner: "acme", Repo: "api", PRNumber: 17, PRURL: "https://github.com/acme/api/pull/17",
		ProviderHost: "https://github.com", ConnectionScope: "builtin:ws-registration-existing",
		ProviderRepositoryID: "provider-repo-17", AssociationRevision: 7,
	}}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, github: provider}
	processor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
	if err := processor.processIntent(ctx, intent); err != nil {
		t.Fatalf("process already-active registration: %v", err)
	}

	succeeded, err := repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if err != nil || succeeded == nil || succeeded.State != "succeeded" || succeeded.AssociationRevision == nil ||
		*succeeded.AssociationRevision != 7 {
		t.Fatalf("completed intent = (%+v, err=%v), want succeeded at association revision 7", succeeded, err)
	}
	if provider.registerCalls != 1 {
		t.Fatalf("provider registration calls = %d, want 1", provider.registerCalls)
	}
}

func TestTaskChangeRegistrationProcessorPreservesLinkAfterOriginDeletion(t *testing.T) {
	ctx := context.Background()
	taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
	seedTaskChangeCoordinatorTask(t, repository, "ws-registration-deleted", "task-registration-deleted", "repo-registration-deleted",
		"https://github.com", "acme", "api")
	if _, err := db.ExecContext(ctx, `UPDATE repositories SET provider_repo_id = 'provider-repo-17' WHERE id = ?`, "repo-registration-deleted"); err != nil {
		t.Fatalf("set provider repository identity: %v", err)
	}
	store, err := github.NewStore(db, db)
	if err != nil {
		t.Fatalf("initialize GitHub store: %v", err)
	}
	message := acceptRegistrationTestMessage(t, ctx, repository, "task-registration-deleted", "selection-deleted")
	intent, err := repository.GetTaskChangeRegistrationIntentBySelection(ctx, message.TaskID, "selection-deleted")
	if err != nil || intent == nil {
		t.Fatalf("read accepted intent: intent=%+v err=%v", intent, err)
	}
	if err := repository.DeleteMessage(ctx, message.ID); err != nil {
		t.Fatalf("delete origin message: %v", err)
	}
	provider := &acceptedGitHubProviderStub{store: store}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, github: provider}
	processor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
	if err := processor.processIntent(ctx, intent); err != nil {
		t.Fatalf("process accepted intent after origin deletion: %v", err)
	}
	completed, err := repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if err != nil || completed == nil || completed.State != "succeeded" {
		t.Fatalf("intent after origin deletion = (%+v, err=%v)", completed, err)
	}
	linked, err := store.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, message.TaskID, "https://github.com", "builtin:ws-registration-deleted", "provider-repo-17", 17,
	)
	if err != nil || linked == nil || linked.DetachedAt != nil {
		t.Fatalf("association after origin deletion = (%+v, err=%v)", linked, err)
	}
}

func TestTaskChangeRegistrationProcessorReauthorizesAcceptedActor(t *testing.T) {
	for _, providerName := range []string{"github", "gitlab"} {
		for _, revokePoint := range []string{"before_processing", "before_provider_commit"} {
			t.Run(providerName+"/"+revokePoint, func(t *testing.T) {
				ctx := context.Background()
				taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
				host, owner, name := "https://github.com", "acme", "api"
				if providerName == "gitlab" {
					host, owner, name = "https://gitlab.example.test", "group", "project"
				}
				workspaceID := "ws-registration-authz-" + providerName + "-" + revokePoint
				taskID := "task-registration-authz-" + providerName + "-" + revokePoint
				repositoryID := "repo-registration-authz-" + providerName + "-" + revokePoint
				seedTaskChangeCoordinatorTask(t, repository, workspaceID, taskID, repositoryID, host, owner, name)
				if _, err := db.ExecContext(ctx, `UPDATE repositories SET provider_repo_id = ? WHERE id = ?`, "provider-repo-17", repositoryID); err != nil {
					t.Fatalf("set provider repository identity: %v", err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE workspaces SET owner_id = 'workspace-owner' WHERE id = ?`, workspaceID); err != nil {
					t.Fatalf("set private workspace owner: %v", err)
				}
				if err := repository.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
					WorkspaceID: workspaceID, UserID: "user-registration", Role: "collaborator", CreatedAt: time.Now().UTC(),
				}); err != nil {
					t.Fatalf("grant admitting member access: %v", err)
				}
				if providerName == "github" {
					if _, err := github.NewStore(db, db); err != nil {
						t.Fatalf("initialize GitHub registration schema: %v", err)
					}
				} else if _, err := gitlab.NewStore(db, db); err != nil {
					t.Fatalf("initialize GitLab registration schema: %v", err)
				}
				message := acceptRegistrationTestMessageForProvider(t, ctx, repository, taskID, "selection-authz", providerName)
				intent, err := repository.GetTaskChangeRegistrationIntentBySelection(ctx, taskID, "selection-authz")
				if err != nil || intent == nil || intent.State != "pending" || intent.ActorID != "user-registration" {
					t.Fatalf("accepted member intent = (%+v, err=%v)", intent, err)
				}

				if revokePoint == "before_processing" {
					if err := repository.DeleteWorkspaceMember(ctx, workspaceID, "user-registration"); err != nil {
						t.Fatalf("revoke member access before processing: %v", err)
					}
				}

				coordinator := taskChangeLinkCoordinator{tasks: taskSvc}
				var githubProvider *acceptedGitHubProviderStub
				var gitlabProvider *acceptedGitLabProviderStub
				beforeCommit := func() {
					if revokePoint != "before_provider_commit" {
						return
					}
					if err := repository.DeleteWorkspaceMember(ctx, workspaceID, "user-registration"); err != nil {
						t.Fatalf("revoke member access before provider commit: %v", err)
					}
				}
				if providerName == "github" {
					store, err := github.NewStore(db, db)
					if err != nil {
						t.Fatalf("initialize GitHub store: %v", err)
					}
					githubProvider = &acceptedGitHubProviderStub{store: store, beforeCommit: beforeCommit}
					coordinator.github = githubProvider
				} else {
					gitlabProvider = &acceptedGitLabProviderStub{beforeCommit: beforeCommit}
					coordinator.gitlab = gitlabProvider
				}
				processor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
				if err := processor.processIntent(ctx, intent); err != nil {
					t.Fatalf("process accepted registration after access change: %v", err)
				}

				completed, err := repository.GetTaskChangeRegistrationIntent(ctx, taskID, intent.ID)
				if err != nil || completed == nil || completed.State != "failed" || completed.ErrorCode != "authorization_revoked" {
					t.Fatalf("revoked registration result = (%+v, err=%v), want truthful authorization failure", completed, err)
				}
				assertMessageRegistrationStatus(t, ctx, repository, message.ID, "failed")
				storedMessage, err := repository.GetMessage(ctx, message.ID)
				if err != nil || storedMessage == nil || storedMessage.Content != message.Content {
					t.Fatalf("accepted message was not preserved: message=%+v err=%v", storedMessage, err)
				}
				switch {
				case revokePoint == "before_processing":
					if githubProvider != nil && githubProvider.registerCalls != 0 {
						t.Fatalf("GitHub provider dispatched %d calls after revocation", githubProvider.registerCalls)
					}
					if gitlabProvider != nil && gitlabProvider.registerCalls != 0 {
						t.Fatalf("GitLab provider dispatched %d calls after revocation", gitlabProvider.registerCalls)
					}
				case githubProvider != nil:
					if githubProvider.registerCalls != 1 {
						t.Fatalf("GitHub registration dispatches = %d, want one provider read", githubProvider.registerCalls)
					}
					linked, err := storeTaskPRByAcceptedRegistration(ctx, db, taskID, host, workspaceID, repositoryID)
					if err != nil || linked {
						t.Fatalf("revoked GitHub actor created an association: linked=%v err=%v", linked, err)
					}
				case gitlabProvider.registerCalls != 1 || gitlabProvider.mutationCalls != 0:
					t.Fatalf("GitLab provider activity after commit-boundary revocation = reads %d, mutations %d", gitlabProvider.registerCalls, gitlabProvider.mutationCalls)
				}
			})
		}
	}
}

func TestTaskChangeRegistrationReconcilesMissedDetachAfterRestart(t *testing.T) {
	for _, providerName := range []string{"github", "gitlab"} {
		t.Run(providerName, func(t *testing.T) {
			ctx := context.Background()
			taskSvc, repository, db := newTaskChangeCoordinatorHarnessWithDB(t)
			host, owner, name := "https://github.com", "acme", "api"
			if providerName == "gitlab" {
				host, owner, name = "https://gitlab.example.test", "group", "project"
			}
			workspaceID := "ws-registration-detach-recovery-" + providerName
			taskID := "task-registration-detach-recovery-" + providerName
			repositoryID := "repo-registration-detach-recovery-" + providerName
			seedTaskChangeCoordinatorTask(t, repository, workspaceID, taskID, repositoryID, host, owner, name)
			if _, err := db.ExecContext(ctx, `UPDATE repositories SET provider_repo_id = 'provider-repo-17' WHERE id = ?`, repositoryID); err != nil {
				t.Fatalf("set provider repository identity: %v", err)
			}
			var err error
			var githubStore *github.Store
			var gitlabStore *gitlab.Store
			if providerName == "github" {
				githubStore, err = github.NewStore(db, db)
			} else {
				gitlabStore, err = gitlab.NewStore(db, db)
			}
			if err != nil {
				t.Fatalf("initialize %s store: %v", providerName, err)
			}
			message := acceptRegistrationTestMessageForProvider(t, ctx, repository, taskID, "selection-detach-recovery", providerName)
			intent, err := repository.GetTaskChangeRegistrationIntentBySelection(ctx, taskID, "selection-detach-recovery")
			if err != nil || intent == nil || intent.State != "pending" {
				t.Fatalf("accepted intent = (%+v, err=%v)", intent, err)
			}

			coordinator := taskChangeLinkCoordinator{tasks: taskSvc}
			if providerName == "github" {
				coordinator.github = &acceptedGitHubProviderStub{store: githubStore}
			} else {
				coordinator.gitlab = &acceptedGitLabProviderStub{store: gitlabStore}
			}
			processor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
			if err := processor.processIntent(ctx, intent); err != nil {
				t.Fatalf("process accepted registration: %v", err)
			}
			linked, err := repository.GetTaskChangeRegistrationIntent(ctx, taskID, intent.ID)
			if err != nil || linked == nil || linked.State != "succeeded" || linked.AssociationRevision == nil {
				t.Fatalf("linked intent = (%+v, err=%v)", linked, err)
			}

			associationID := linked.OperationReceipt
			if providerName == "github" {
				association, changed, detachErr := githubStore.DetachTaskPR(ctx, associationID)
				if detachErr != nil || !changed || association == nil || association.DetachedAt == nil {
					t.Fatalf("commit GitHub detach without event: association=%+v changed=%v err=%v", association, changed, detachErr)
				}
			} else {
				association, changed, detachErr := gitlabStore.DetachTaskMRForWorkspace(ctx, workspaceID, associationID)
				if detachErr != nil || !changed || association == nil || association.DetachedAt == nil {
					t.Fatalf("commit GitLab detach without event: association=%+v changed=%v err=%v", association, changed, detachErr)
				}
			}

			// A newly constructed processor models a service restart after the
			// detach committed but its notification was lost.
			restartedProcessor := newTaskChangeRegistrationProcessor(repository, taskSvc, coordinator, db, nil, testRegistrationActorResolver)
			restartedProcessor.reconcileSucceeded(ctx)
			unlinked, err := repository.GetTaskChangeRegistrationIntent(ctx, taskID, intent.ID)
			if err != nil || unlinked == nil || unlinked.State != "unlinked" {
				t.Fatalf("reconciled registration = (%+v, err=%v), want unlinked after restart", unlinked, err)
			}
			assertMessageRegistrationStatus(t, ctx, repository, message.ID, "unlinked")

			relinked, replayed, err := repository.ActTaskChangeRegistration(
				ctx, taskID, intent.SelectionID, "relink-after-recovery", intent.ActorID, "relink",
			)
			if err != nil || replayed || relinked == nil || relinked.State != "pending" || relinked.SelectionID != "relink-after-recovery" {
				t.Fatalf("fresh relink action = (%+v, replayed=%v, err=%v)", relinked, replayed, err)
			}
		})
	}
}

func storeTaskPRByAcceptedRegistration(
	ctx context.Context, db *sqlx.DB, taskID, host, workspaceID, repositoryID string,
) (bool, error) {
	store, err := github.NewStore(db, db)
	if err != nil {
		return false, err
	}
	var providerRepositoryID string
	if err := db.GetContext(ctx, &providerRepositoryID, db.Rebind(`SELECT provider_repo_id FROM repositories WHERE id = ?`), repositoryID); err != nil {
		return false, err
	}
	registered, err := store.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, taskID, host, "builtin:"+workspaceID, providerRepositoryID, 17,
	)
	return registered != nil, err
}

func acceptRegistrationTestMessage(
	t *testing.T,
	ctx context.Context,
	repository *sqlite.Repository,
	taskID string,
	selectionID string,
) *models.Message {
	return acceptRegistrationTestMessageForProvider(t, ctx, repository, taskID, selectionID, "github")
}

func acceptRegistrationTestMessageForProvider(
	t *testing.T,
	ctx context.Context,
	repository *sqlite.Repository,
	taskID string,
	selectionID string,
	provider string,
) *models.Message {
	t.Helper()
	sessionID := "session-" + taskID
	turnID := "turn-" + taskID
	messageID := "message-" + taskID
	if err := repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repository.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: sessionID,
	}); err != nil {
		t.Fatalf("create task turn: %v", err)
	}
	kind, id, key, title, url, scope, content := "pull_request", "PR_kwDOAA", "acme/api#17", "Accepted PR", "https://github.com/acme/api/pull/17", "github.com/acme/api", "Review this PR"
	if provider == "gitlab" {
		kind, id, key, title, url, scope, content = "merge_request", "17", "group/project!17", "Accepted MR",
			"https://gitlab.example.test/group/project/-/merge_requests/17", "gitlab.example.test/group/project", "Review this change request"
	}
	reference := v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef(provider, kind, scope, id),
		Provider: provider, Kind: kind, ID: id, Key: key,
		Title: title, URL: url, Scope: scope,
		SelectionID: selectionID, AssociationMode: v1.EntityReferenceModeLinkOnSend,
	}
	message := &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID,
		TurnID: turnID, AuthorType: models.MessageAuthorUser, AuthorID: "user-registration",
		Type: models.MessageTypeMessage, Content: content, CreatedAt: time.Now().UTC(),
		Metadata: map[string]any{"entity_references": []v1.EntityReference{reference}},
	}
	if err := repository.CreateMessage(ctx, message); err != nil {
		t.Fatalf("accept direct message: %v", err)
	}
	return message
}

func assertMessageRegistrationStatus(
	t *testing.T,
	ctx context.Context,
	repository *sqlite.Repository,
	messageID string,
	want string,
) {
	t.Helper()
	message, err := repository.GetMessage(ctx, messageID)
	if err != nil || message == nil {
		t.Fatalf("read message registration projection: message=%+v err=%v", message, err)
	}
	references, ok := message.Metadata["entity_references"].([]interface{})
	if !ok || len(references) != 1 {
		t.Fatalf("message references = %#v, want one reference", message.Metadata["entity_references"])
	}
	encoded, err := json.Marshal(references)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []v1.EntityReference
	if err := json.Unmarshal(encoded, &decoded); err != nil || len(decoded) != 1 {
		t.Fatalf("decode message reference status: references=%+v err=%v", decoded, err)
	}
	wantStatus := v1.EntityReferenceRegistrationFailed
	switch want {
	case "linked":
		wantStatus = v1.EntityReferenceRegistrationLinked
	case "unlinked":
		wantStatus = v1.EntityReferenceRegistrationUnlinked
	}
	if decoded[0].RegistrationStatus != wantStatus {
		t.Fatalf("message registration status = %q, want %q", decoded[0].RegistrationStatus, wantStatus)
	}
}
