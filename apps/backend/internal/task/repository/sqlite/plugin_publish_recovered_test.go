package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	publishOperationID = "op-publish"
	publishRuntimeID   = "execution-descriptor-1"
	publishWorkspace   = "/srv/work"
)

// seedCreatingEnvironment leaves the environment creating under the session
// and the envelope ready, the state a recovered launch publishes from.
func seedCreatingEnvironment(t *testing.T, repo *Repository, seed descriptorSeeder, extra map[string]interface{}) uint64 {
	t.Helper()
	seed(t, repo, 1)
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
		`UPDATE task_environments SET status = ?, materialization_session_id = ? WHERE id = ?`),
		string(models.TaskEnvironmentStatusCreating), "session-descriptor-1", "environment-descriptor-1"); err != nil {
		t.Fatalf("mark creating: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "allocating", "", ""), 0); err != nil {
		t.Fatalf("allocating: %v", err)
	}
	ready := descriptorFixture(1, "install-a", "ready", publishWorkspace, "worker-1")
	envelope, _ := ready.Metadata["plugin_executor"].(map[string]interface{})
	envelope["operation_id"], envelope["runtime_identity"] = publishOperationID, publishRuntimeID
	for key, value := range extra {
		envelope[key] = value
	}
	if err := checkpointDescriptor(t, repo, ready, 1); err != nil {
		t.Fatalf("ready: %v", err)
	}
	_, revision := storedEnvelope(t, repo)
	return revision
}

func publishParams(claim *models.TaskEnvironmentRecoveryClaim, revision uint64) models.PublishRecoveredTaskEnvironmentParams {
	return models.PublishRecoveredTaskEnvironmentParams{
		Claim: claim, SessionID: "session-descriptor-1", EnvironmentID: "environment-descriptor-1",
		OperationID: publishOperationID, RuntimeIdentity: publishRuntimeID, OwnershipGeneration: 7, ExpectedRevision: revision,
	}
}

func publishedEnvironment(t *testing.T, repo *Repository) *models.TaskEnvironment {
	t.Helper()
	env, err := repo.GetTaskEnvironment(context.Background(), "environment-descriptor-1")
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	return env
}

func exercisePublishIsGuardedAndIdempotent(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repo, seed, nil)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || !published {
		t.Fatalf("publish = %v, %v; want published", published, err)
	}
	env := publishedEnvironment(t, repo)
	if env.Status != models.TaskEnvironmentStatusReady || env.WorkspacePath != publishWorkspace || env.MaterializationSessionID != "" {
		t.Fatalf("environment after publish = %+v", env)
	}
	again, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || again {
		t.Fatalf("repeat publish = %v, %v; want a no-op", again, err)
	}
}

func exercisePublishMismatchPublishesNothing(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repo, seed, nil)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	cases := []struct {
		name   string
		mutate func(*models.PublishRecoveredTaskEnvironmentParams)
		reason string
	}{
		{"operation", func(p *models.PublishRecoveredTaskEnvironmentParams) { p.OperationID = "op-other" }, models.PluginPublishReasonOperation},
		{"runtime identity", func(p *models.PublishRecoveredTaskEnvironmentParams) { p.RuntimeIdentity = "execution-other" }, models.PluginPublishReasonRuntime},
		{"generation", func(p *models.PublishRecoveredTaskEnvironmentParams) { p.OwnershipGeneration = 8 }, models.PluginPublishReasonGeneration},
		{"revision", func(p *models.PublishRecoveredTaskEnvironmentParams) { p.ExpectedRevision = revision - 1 }, models.PluginPublishReasonRevision},
		{"session", func(p *models.PublishRecoveredTaskEnvironmentParams) { p.SessionID = "session-other" }, models.PluginPublishReasonSession},
	}
	for _, tc := range cases {
		params := publishParams(claim, revision)
		tc.mutate(&params)
		published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
		var mismatch *models.PluginPublishMismatchError
		if published || !errors.Is(err, models.ErrExecutorPublishMismatch) || !errors.As(err, &mismatch) {
			t.Fatalf("%s: publish = %v, %v; want a named mismatch", tc.name, published, err)
		}
		if mismatch.Reason != tc.reason {
			t.Fatalf("%s: reason = %q, want %q", tc.name, mismatch.Reason, tc.reason)
		}
		if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusCreating {
			t.Fatalf("%s: a refused publication changed the environment to %q", tc.name, env.Status)
		}
	}
}

func exercisePublishRefusesWhenTheEnvelopeIsNotReady(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	seed(t, repo, 1)
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
		`UPDATE task_environments SET status = ?, materialization_session_id = ? WHERE id = ?`),
		string(models.TaskEnvironmentStatusCreating), "session-descriptor-1", "environment-descriptor-1"); err != nil {
		t.Fatalf("mark creating: %v", err)
	}
	if err := checkpointDescriptor(t, repo, descriptorFixture(1, "install-a", "allocating", "", ""), 0); err != nil {
		t.Fatalf("allocating: %v", err)
	}
	provisioned := descriptorFixture(1, "install-a", "provisioned", publishWorkspace, "worker-1")
	envelope, _ := provisioned.Metadata["plugin_executor"].(map[string]interface{})
	envelope["operation_id"], envelope["runtime_identity"] = publishOperationID, publishRuntimeID
	if err := checkpointDescriptor(t, repo, provisioned, 1); err != nil {
		t.Fatalf("provisioned: %v", err)
	}
	_, revision := storedEnvelope(t, repo)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), publishParams(claim, revision))
	var mismatch *models.PluginPublishMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != models.PluginPublishReasonPhase {
		t.Fatalf("err = %v, want the phase mismatch", err)
	}
}

func exercisePublishChecksTheRepositoryInventory(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repo, seed, map[string]interface{}{
		"repositories": []interface{}{map[string]interface{}{"id": "repo-1", "relative_path": "a", "head_commit": "abc"}},
	})
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	params.Repos = []*models.TaskEnvironmentRepo{{RepositoryID: "repo-2", WorktreePath: publishWorkspace + "/b"}}
	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	var mismatch *models.PluginPublishMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != models.PluginPublishReasonRepos {
		t.Fatalf("err = %v, want the repository inventory mismatch", err)
	}
}

func exercisePublishNeedsTheHeldClaim(t *testing.T, repo *Repository, seed descriptorSeeder) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repo, seed, nil)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	stale := *claim
	stale.OperationID = "op-released:recovery"
	if _, err := repo.PublishRecoveredTaskEnvironment(context.Background(), publishParams(&stale, revision)); !errors.Is(err, models.ErrExecutorClaimNotFound) {
		t.Fatalf("publish under a claim that is not held = %v, want not_found", err)
	}
	if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusCreating {
		t.Fatalf("status = %q after a refused publication", env.Status)
	}
}

func exercisePublishRace(t *testing.T, repoA, repoB *Repository, seed descriptorSeeder) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repoA, seed, nil)
	claim := acquireClaimForDescriptorEnvironment(t, repoA, publishOperationID+":recovery")
	type outcome struct {
		published bool
		err       error
	}
	results := make([]outcome, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, repo := range []*Repository{repoA, repoB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i].published, results[i].err = repo.PublishRecoveredTaskEnvironment(context.Background(), publishParams(claim, revision))
		}()
	}
	close(start)
	wg.Wait()
	published := 0
	for _, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent publication failed: %v", result.err)
		}
		if result.published {
			published++
		}
	}
	if published != 1 {
		t.Fatalf("results = %+v; two concurrent publications must write exactly once", results)
	}
}

// exerciseArchiveRacingRecoveryHoldsOneClaim: of a recovery and a cleanup
// claim for one environment, exactly one is granted.
func exerciseArchiveRacingRecoveryHoldsOneClaim(t *testing.T, repoA, repoB *Repository, seed descriptorSeeder) {
	t.Helper()
	seedCreatingEnvironment(t, repoA, seed, nil)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, entry := range []struct {
		repo *Repository
		op   string
	}{{repoA, publishOperationID + ":recovery"}, {repoB, publishOperationID + ":cleanup"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = entry.repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
				TaskEnvironmentID: "environment-descriptor-1", OwnerTaskID: "task-descriptor-1", OwnershipGeneration: 7,
				SessionID: "session-descriptor-1", OperationID: entry.op, ExecutorType: string(models.ExecutorTypePluginRemote),
				AllowCurrentSessionRuntime: true,
			})
		}()
	}
	close(start)
	wg.Wait()
	granted := 0
	for _, err := range errs {
		if err == nil {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("claim results = %v; a racing recovery and cleanup must hold exactly one claim", errs)
	}
}

func TestPluginPublishIsGuardedAndIdempotent(t *testing.T) {
	exercisePublishIsGuardedAndIdempotent(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginPublishMismatchPublishesNothing(t *testing.T) {
	exercisePublishMismatchPublishesNothing(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginPublishRefusesWhenTheEnvelopeIsNotReady(t *testing.T) {
	exercisePublishRefusesWhenTheEnvelopeIsNotReady(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginPublishChecksTheRepositoryInventory(t *testing.T) {
	exercisePublishChecksTheRepositoryInventory(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginPublishNeedsTheHeldClaim(t *testing.T) {
	exercisePublishNeedsTheHeldClaim(t, newRepoForSessionTests(t), seedDescriptorEnvironmentSQLite)
}

func TestPluginPublishRace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exercisePublishRace(t, repo, repo, seedDescriptorEnvironmentSQLite)
}

func TestPluginArchiveRacingRecoveryHoldsOneClaim(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exerciseArchiveRacingRecoveryHoldsOneClaim(t, repo, repo, seedDescriptorEnvironmentSQLite)
}
