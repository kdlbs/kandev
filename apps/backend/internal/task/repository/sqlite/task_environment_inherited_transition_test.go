package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedInheritedTransition(t *testing.T, repo *Repository, suffix string, sourceStatus models.TaskEnvironmentStatus) (*models.TaskEnvironment, []*models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-transition-" + suffix
	seedWorkspace(t, repo, workspaceID)
	parentID := "task-parent-" + suffix
	childID := "task-child-" + suffix
	for _, task := range []*models.Task{
		{ID: parentID, WorkspaceID: workspaceID, Title: parentID},
		{ID: childID, WorkspaceID: workspaceID, ParentID: parentID, Title: childID},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	source := &models.TaskEnvironment{
		ID: "environment-parent-" + suffix, TaskID: parentID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: sourceStatus,
		WorkspacePath: "/parent/" + suffix,
	}
	if source.Status == models.TaskEnvironmentStatusCreating {
		source.MaterializationSessionID = "session-parent-" + suffix
	}
	if err := repo.CreateTaskEnvironment(ctx, source); err != nil {
		t.Fatalf("create source environment: %v", err)
	}
	sessions := []*models.TaskSession{
		{ID: "session-child-a-" + suffix, TaskID: childID, TaskEnvironmentID: source.ID, WorkspacePath: source.WorkspacePath},
		{ID: "session-child-b-" + suffix, TaskID: childID, TaskEnvironmentID: source.ID, WorkspacePath: source.WorkspacePath},
	}
	for _, session := range sessions {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create inherited session %s: %v", session.ID, err)
		}
	}
	return source, sessions
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5
func TestClaimInheritedTaskEnvironmentTransitionDetachesFailedForeignEnvironment(t *testing.T) {
	for _, sourceStatus := range []models.TaskEnvironmentStatus{
		models.TaskEnvironmentStatusFailed,
		models.TaskEnvironmentStatusCreating,
	} {
		t.Run(string(sourceStatus), func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			source, sessions := seedInheritedTransition(t, repo, "foreign-"+string(sourceStatus), sourceStatus)
			owner := sessions[0]

			environment, handled, rebound, err := repo.ClaimInheritedTaskEnvironmentTransition(
				ctx, owner.TaskID, owner.ID, source.ID,
				string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
			)
			if err != nil {
				t.Fatalf("claim detached child environment: %v", err)
			}
			if !handled || !rebound || environment == nil {
				t.Fatalf("transition = environment %v, handled %v, rebound %v; want a newly bound child environment", environment, handled, rebound)
			}
			if environment.TaskID != owner.TaskID || environment.ExecutorType != string(models.ExecutorTypeLocalDocker) ||
				environment.Status != models.TaskEnvironmentStatusCreating || environment.MaterializationSessionID != owner.ID {
				t.Fatalf("child environment = %+v, want creating child-owned claim for %s", environment, owner.ID)
			}
			bound, err := repo.GetTaskSession(ctx, owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if bound.TaskEnvironmentID != environment.ID || bound.WorkspacePath != "" {
				t.Fatalf("persisted child binding = environment %q path %q, want %q and empty path", bound.TaskEnvironmentID, bound.WorkspacePath, environment.ID)
			}
			unchangedSource, err := repo.GetTaskEnvironment(ctx, source.ID)
			if err != nil {
				t.Fatal(err)
			}
			if unchangedSource.Status != source.Status || unchangedSource.MaterializationSessionID != source.MaterializationSessionID ||
				unchangedSource.WorkspacePath != source.WorkspacePath || unchangedSource.TaskID != source.TaskID {
				t.Fatalf("foreign %s environment changed during detach: %+v", sourceStatus, unchangedSource)
			}
		})
	}
}

func TestClaimInheritedTaskEnvironmentTransitionClassifiesLegacyImplicitLocalSource(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source, sessions := seedInheritedTransition(t, repo, "legacy-implicit-local", models.TaskEnvironmentStatusReady)
	source.ExecutorType = ""
	if err := repo.UpdateTaskEnvironment(ctx, source); err != nil {
		t.Fatalf("clear legacy source executor type: %v", err)
	}

	childEnvironment, handled, rebound, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, sessions[0].TaskID, sessions[0].ID, source.ID,
		string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if err != nil || !handled || !rebound || childEnvironment == nil {
		t.Fatalf("legacy implicit-local transition = %+v, handled %v rebound %v error %v; want a detached child environment", childEnvironment, handled, rebound, err)
	}
	if childEnvironment.TaskID != sessions[0].TaskID || childEnvironment.ExecutorType != string(models.ExecutorTypeLocalDocker) {
		t.Fatalf("child environment = %+v, want child-owned Docker environment", childEnvironment)
	}
	unchangedSource, err := repo.GetTaskEnvironment(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchangedSource.ExecutorType != "" || unchangedSource.TaskID != source.TaskID || unchangedSource.WorkspacePath != source.WorkspacePath {
		t.Fatalf("legacy parent environment changed during transition: %+v", unchangedSource)
	}
}

func TestClaimInheritedTaskEnvironmentTransitionRejectsUnverifiableLegacySource(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.TaskEnvironment)
	}{
		{name: "executor identity", mutate: func(environment *models.TaskEnvironment) { environment.ExecutorID = "missing-executor" }},
		{name: "profile identity", mutate: func(environment *models.TaskEnvironment) { environment.ExecutorProfileID = "missing-profile" }},
		{name: "container handle", mutate: func(environment *models.TaskEnvironment) { environment.ContainerID = "container-123" }},
		{name: "sandbox handle", mutate: func(environment *models.TaskEnvironment) { environment.SandboxID = "sandbox-123" }},
		{name: "container secret handles", mutate: func(environment *models.TaskEnvironment) {
			environment.ContainerBootstrapNonceSecretID = "bootstrap-secret"
			environment.ContainerControlAuthTokenSecretID = "control-secret"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			source, sessions := seedInheritedTransition(t, repo, "legacy-unknown-"+strings.ReplaceAll(test.name, " ", "-"), models.TaskEnvironmentStatusReady)
			source.ExecutorType = ""
			test.mutate(source)
			if err := repo.UpdateTaskEnvironment(ctx, source); err != nil {
				t.Fatalf("update legacy source identity: %v", err)
			}

			_, handled, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
				ctx, sessions[0].TaskID, sessions[0].ID, source.ID,
				string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
			)
			if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
				t.Fatalf("transition error = %v, handled %v; want ErrWorkspaceReuseUnsafe", err, handled)
			}
			if environment, getErr := repo.GetTaskEnvironmentByTaskID(ctx, sessions[0].TaskID); getErr != nil || environment != nil {
				t.Fatalf("child environment = %+v, %v; want no claim", environment, getErr)
			}
			bound, getErr := repo.GetTaskSession(ctx, sessions[0].ID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if bound.TaskEnvironmentID != source.ID || bound.WorkspacePath != source.WorkspacePath {
				t.Fatalf("session binding = %q path %q; want original inherited binding", bound.TaskEnvironmentID, bound.WorkspacePath)
			}
		})
	}
}

func TestClaimInheritedTaskEnvironmentTransitionClassifiesLegacyCanonicalEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source, sessions := seedInheritedTransition(t, repo, "legacy-canonical", models.TaskEnvironmentStatusReady)
	canonical := &models.TaskEnvironment{
		ID: "environment-child-legacy-canonical", TaskID: sessions[0].TaskID,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/child/legacy",
	}
	if err := repo.CreateTaskEnvironment(ctx, canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET task_environment_id = ?, workspace_path = ? WHERE id = ?
	`), canonical.ID, canonical.WorkspacePath, sessions[0].ID); err != nil {
		t.Fatal(err)
	}

	_, handled, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, sessions[0].TaskID, sessions[0].ID, source.ID,
		string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if !errors.Is(err, models.ErrWorkspaceExecutorMismatch) {
		t.Fatalf("canonical legacy executor classification = handled %v error %v; want mismatch against implicit local", handled, err)
	}
	bound, err := repo.GetTaskSession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.TaskEnvironmentID != canonical.ID || bound.WorkspacePath != canonical.WorkspacePath {
		t.Fatalf("refused session binding = %q path %q; want canonical %q and path", bound.TaskEnvironmentID, bound.WorkspacePath, canonical.ID)
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8
func TestClaimInheritedTaskEnvironmentTransitionDifferentExecutorsShareOneCanonicalEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source, sessions := seedInheritedTransition(t, repo, "different-executors", models.TaskEnvironmentStatusReady)
	start := make(chan struct{})
	type result struct {
		environment *models.TaskEnvironment
		handled     bool
		rebound     bool
		err         error
	}
	results := make(chan result, len(sessions))
	executorTypes := []string{string(models.ExecutorTypeLocalDocker), string(models.ExecutorTypeSSH)}
	var wait sync.WaitGroup
	for index, session := range sessions {
		wait.Add(1)
		go func(index int, session *models.TaskSession) {
			defer wait.Done()
			<-start
			environment, handled, rebound, err := repo.ClaimInheritedTaskEnvironmentTransition(
				ctx, session.TaskID, session.ID, source.ID, executorTypes[index], "executor-"+executorTypes[index], "profile-"+executorTypes[index],
			)
			results <- result{environment: environment, handled: handled, rebound: rebound, err: err}
		}(index, session)
	}
	close(start)
	wait.Wait()
	close(results)

	var elected *models.TaskEnvironment
	var claims, mismatches int
	for outcome := range results {
		switch {
		case outcome.err == nil && outcome.handled && outcome.environment != nil:
			claims++
			if elected == nil {
				elected = outcome.environment
			} else if elected.ID != outcome.environment.ID {
				t.Fatalf("sessions elected different child environments %q and %q", elected.ID, outcome.environment.ID)
			}
		case errors.Is(outcome.err, models.ErrWorkspaceExecutorMismatch):
			mismatches++
		default:
			t.Fatalf("transition result = %+v, want one claim and one executor mismatch", outcome)
		}
	}
	if claims != 1 || mismatches != 1 || elected == nil {
		t.Fatalf("transition results: claims=%d mismatches=%d environment=%+v", claims, mismatches, elected)
	}
	if elected.TaskID != sessions[0].TaskID || elected.Status != models.TaskEnvironmentStatusCreating || elected.MaterializationSessionID == "" {
		t.Fatalf("elected environment = %+v, want one creating child-owned claim", elected)
	}
	canonical, err := repo.GetTaskEnvironmentByTaskID(ctx, sessions[0].TaskID)
	if err != nil || canonical == nil || canonical.ID != elected.ID {
		t.Fatalf("canonical child environment = %+v, %v, want %q", canonical, err, elected.ID)
	}
	var childEnvironmentCount int
	if err := repo.db.Get(&childEnvironmentCount, repo.db.Rebind(`SELECT COUNT(*) FROM task_environments WHERE task_id = ?`), sessions[0].TaskID); err != nil {
		t.Fatal(err)
	}
	if childEnvironmentCount != 1 {
		t.Fatalf("child task environments = %d, want exactly one", childEnvironmentCount)
	}
	for _, session := range sessions {
		bound, err := repo.GetTaskSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if bound.TaskEnvironmentID != elected.ID {
			t.Fatalf("session %s environment = %q, want canonical child environment %q even when its executor selection is refused", session.ID, bound.TaskEnvironmentID, elected.ID)
		}
		if bound.WorkspacePath != elected.WorkspacePath {
			t.Fatalf("session %s workspace path = %q, want canonical path %q", session.ID, bound.WorkspacePath, elected.WorkspacePath)
		}
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8
func TestClaimInheritedTaskEnvironmentTransitionRecoversCancelledOwnerWithoutStalePublication(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source, sessions := seedInheritedTransition(t, repo, "cancelled-owner", models.TaskEnvironmentStatusReady)
	owner, successor := sessions[0], sessions[1]
	first, handled, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, owner.TaskID, owner.ID, source.ID, string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if err != nil || !handled || first == nil {
		t.Fatalf("initial transition claim = %+v, %v, handled %v", first, err, handled)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateCancelled, "cancelled during workspace preparation"); err != nil {
		t.Fatal(err)
	}

	second, handled, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, successor.TaskID, successor.ID, source.ID, string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if err != nil || !handled || second == nil {
		t.Fatalf("successor transition claim = %+v, %v, handled %v", second, err, handled)
	}
	if second.ID != first.ID || second.MaterializationSessionID != successor.ID || second.Status != models.TaskEnvironmentStatusCreating {
		t.Fatalf("successor environment = %+v, want same creating row owned by %s", second, successor.ID)
	}

	stale := *first
	stale.Status = models.TaskEnvironmentStatusReady
	stale.WorkspacePath = "/stale-owner/workspace"
	if err := repo.FinalizeTaskEnvironmentMaterialization(ctx, &stale, nil, owner.ID); err == nil {
		t.Fatal("former owner finalized the successor's workspace claim")
	}
	current, err := repo.GetTaskEnvironment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != models.TaskEnvironmentStatusCreating || current.MaterializationSessionID != successor.ID || current.WorkspacePath != "" {
		t.Fatalf("environment after stale publication attempt = %+v, want successor claim unchanged", current)
	}
}
