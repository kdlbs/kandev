package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteProfileMembershipMutationsSerializeWithReorder(t *testing.T) {
	for _, scenario := range profileOrderMutationScenarios() {
		for _, winner := range []string{"mutation-first", "reorder-first"} {
			t.Run(scenario.name+"/"+winner, func(t *testing.T) {
				repo := openSQLiteProfileOrderRaceRepo(t)
				runProfileOrderMutationRace(t, repo, scenario, winner)
			})
		}
	}
}

func TestSQLiteProfileUpdateRetriesAfterOwnerChangesDuringRead(t *testing.T) {
	runProfileOwnerChangedDuringRead(t, openSQLiteProfileOrderRaceRepo(t))
}

func TestPostgresProfileUpdateRetriesAfterOwnerChangesDuringRead(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize profile order schema: %v", err)
	}
	runProfileOwnerChangedDuringRead(t, repo)
}

func runProfileOwnerChangedDuringRead(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	a := &models.Agent{Name: "profile-owner-a"}
	b := &models.Agent{Name: "profile-owner-b"}
	c := &models.Agent{Name: "profile-owner-c"}
	for _, agent := range []*models.Agent{a, b, c} {
		if err := repo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	profile := &models.AgentProfile{AgentID: a.ID, Name: "owner-race", Model: "model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	staleUpdate, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	staleUpdate.AgentID = b.ID

	observedA := make(chan struct{})
	releaseOwnerRead := make(chan struct{})
	var paused atomic.Bool
	repo.profileOrderAfterOwnershipRead = func(id, ownerID, _ string) error {
		if id == profile.ID && ownerID == a.ID && paused.CompareAndSwap(false, true) {
			close(observedA)
			<-releaseOwnerRead
		}
		return nil
	}
	t.Cleanup(func() {
		select {
		case <-releaseOwnerRead:
		default:
			close(releaseOwnerRead)
		}
	})
	var mu sync.Mutex
	var lockedAgents []string
	repo.profileOrderAfterLock = func(operation, agentID string) error {
		if operation == "update-profile" {
			mu.Lock()
			lockedAgents = append(lockedAgents, agentID)
			mu.Unlock()
		}
		return nil
	}
	outerDone := make(chan error, 1)
	go func() { outerDone <- repo.UpdateAgentProfile(ctx, staleUpdate) }()
	awaitProfileOrderBarrier(t, observedA)

	concurrentUpdate, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	concurrentUpdate.AgentID = c.ID
	if err := repo.UpdateAgentProfile(ctx, concurrentUpdate); err != nil {
		t.Fatalf("concurrent A-to-C owner update: %v", err)
	}
	close(releaseOwnerRead)
	if err := <-outerDone; err != nil {
		t.Fatalf("retried A-to-B owner update: %v", err)
	}
	current, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.AgentID != b.ID {
		t.Fatalf("profile owner = %s, want final owner %s", current.AgentID, b.ID)
	}
	mu.Lock()
	gotLocks := append([]string(nil), lockedAgents...)
	mu.Unlock()
	wantLocks := []string{}
	for _, pair := range [][2]string{{a.ID, c.ID}, {a.ID, b.ID}, {b.ID, c.ID}} {
		if pair[0] < pair[1] {
			wantLocks = append(wantLocks, pair[0], pair[1])
		} else {
			wantLocks = append(wantLocks, pair[1], pair[0])
		}
	}
	if len(gotLocks) != len(wantLocks) {
		t.Fatalf("membership locks = %v, want sorted actual-owner lock sets %v", gotLocks, wantLocks)
	}
	for i := range gotLocks {
		if gotLocks[i] != wantLocks[i] {
			t.Fatalf("membership locks = %v, want sorted actual-owner lock sets %v", gotLocks, wantLocks)
		}
	}
}

func TestPostgresProfileMembershipMutationsSerializeWithReorder(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	for _, scenario := range profileOrderMutationScenarios() {
		for _, winner := range []string{"mutation-first", "reorder-first"} {
			t.Run(scenario.name+"/"+winner, func(t *testing.T) {
				db := testutil.OpenIsolatedPostgres(t, dsn)
				db.SetMaxOpenConns(8)
				repo, err := newSQLiteRepositoryWithDB(db, db, nil)
				if err != nil {
					t.Fatalf("initialize profile order schema: %v", err)
				}
				runProfileOrderMutationRace(t, repo, scenario, winner)
			})
		}
	}
}

type profileOrderMutationScenario struct {
	name      string
	operation string
	mutate    func(context.Context, *sqliteRepository, string, string, *models.AgentProfile) error
	missing   bool
}

func profileOrderMutationScenarios() []profileOrderMutationScenario {
	return []profileOrderMutationScenario{
		{
			name:      "create",
			operation: "create-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, _ *models.AgentProfile) error {
				return repo.CreateAgentProfile(ctx, &models.AgentProfile{AgentID: a, Name: "created", Model: "model"})
			},
		},
		{
			name:      "duplicate",
			operation: "duplicate-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, source *models.AgentProfile) error {
				return repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
					Source:  source,
					Profile: &models.AgentProfile{AgentID: a, Name: "duplicate", Model: source.Model},
				})
			},
		},
		{
			name:      "soft-delete",
			operation: "delete-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, _ string, source *models.AgentProfile) error {
				return repo.DeleteAgentProfile(ctx, source.ID)
			},
		},
		{
			name:      "agent-cascade-delete",
			operation: "delete-agent",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, _ *models.AgentProfile) error {
				return repo.DeleteAgent(ctx, a)
			},
			missing: true,
		},
		{
			name:      "global-owner-a-to-b",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, b string, source *models.AgentProfile) error {
				source.AgentID = b
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
		{
			name:      "global-to-workspace",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, _, _ string, source *models.AgentProfile) error {
				source.WorkspaceID = "workspace-order-test"
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
		{
			name:      "workspace-to-global",
			operation: "update-profile",
			mutate: func(ctx context.Context, repo *sqliteRepository, a, _ string, source *models.AgentProfile) error {
				source.AgentID = a
				source.WorkspaceID = ""
				return repo.UpdateAgentProfile(ctx, source)
			},
		},
	}
}

func openSQLiteProfileOrderRaceRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile-order-race.db")
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	writer, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open SQLite writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	writer.SetMaxOpenConns(8)
	reader, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open SQLite reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	reader.SetMaxOpenConns(8)
	repo, err := newSQLiteRepository(writer, reader, nil, false)
	if err != nil {
		t.Fatalf("initialize SQLite profile order schema: %v", err)
	}
	return repo
}

func runProfileOrderMutationRace(t *testing.T, repo *sqliteRepository, scenario profileOrderMutationScenario, winner string) {
	t.Helper()
	ctx := context.Background()
	a := &models.Agent{Name: "order-race-a"}
	b := &models.Agent{Name: "order-race-b"}
	if err := repo.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgent(ctx, b); err != nil {
		t.Fatal(err)
	}
	profiles := []*models.AgentProfile{
		{AgentID: a.ID, Name: "one", Model: "model"},
		{AgentID: a.ID, Name: "two", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	source, err := repo.GetAgentProfile(ctx, profiles[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if scenario.name == "workspace-to-global" {
		source = &models.AgentProfile{AgentID: a.ID, Name: "workspace-profile", Model: "model", WorkspaceID: "workspace-order-test"}
		if err := repo.CreateAgentProfile(ctx, source); err != nil {
			t.Fatal(err)
		}
		source, err = repo.GetAgentProfile(ctx, source.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	ordered, err := globalProfileOrderIDs(ctx, repo, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested := []string{ordered[1], ordered[0]}
	locked := make(chan struct{})
	release := make(chan struct{})
	repo.profileOrderAfterLock = func(operation, agentID string) error {
		shouldPause := (winner == "mutation-first" && operation == scenario.operation) ||
			(winner == "reorder-first" && operation == "reorder")
		if shouldPause && agentID == a.ID {
			select {
			case <-locked:
			default:
				close(locked)
			}
			<-release
		}
		return nil
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	reorderDone := make(chan profileOrderReorderResult, 1)
	mutationDone := make(chan error, 1)
	mutation := func() error {
		return scenario.mutate(ctx, repo, a.ID, b.ID, source)
	}
	reorder := func() {
		revision, changed, err := repo.ReorderAgentProfiles(ctx, a.ID, requested)
		reorderDone <- profileOrderReorderResult{revision: revision, changed: changed, err: err}
	}
	if winner == "mutation-first" {
		runMutationFirstOrderRace(t, scenario, mutation, reorder, locked, release, mutationDone, reorderDone)
	} else {
		runReorderFirstOrderRace(t, mutation, reorder, locked, release, mutationDone, reorderDone)
	}
	assertProfileOrderMutationResult(t, ctx, repo, scenario.name, a.ID, b.ID, profiles[0].ID)
}

type profileOrderReorderResult struct {
	revision int64
	changed  bool
	err      error
}

func runMutationFirstOrderRace(
	t *testing.T,
	scenario profileOrderMutationScenario,
	mutation func() error,
	reorder func(),
	locked, release chan struct{},
	mutationDone chan error,
	reorderDone <-chan profileOrderReorderResult,
) {
	t.Helper()
	go func() { mutationDone <- mutation() }()
	awaitProfileOrderBarrier(t, locked)
	started := make(chan struct{})
	go func() {
		close(started)
		reorder()
	}()
	awaitProfileOrderBarrier(t, started)
	close(release)
	if err := <-mutationDone; err != nil {
		t.Fatalf("membership mutation: %v", err)
	}
	result := <-reorderDone
	if scenario.missing {
		if !errors.Is(result.err, ErrProfileOrderAgentNotFound) {
			t.Fatalf("reorder after agent deletion error = %v, want missing-agent", result.err)
		}
		return
	}
	if !errors.Is(result.err, ErrProfileOrderSetMismatch) {
		t.Fatalf("reorder after %s error = %v, want stale membership", scenario.name, result.err)
	}
}

func runReorderFirstOrderRace(
	t *testing.T,
	mutation func() error,
	reorder func(),
	locked, release chan struct{},
	mutationDone chan error,
	reorderDone <-chan profileOrderReorderResult,
) {
	t.Helper()
	go reorder()
	awaitProfileOrderBarrier(t, locked)
	started := make(chan struct{})
	go func() {
		close(started)
		mutationDone <- mutation()
	}()
	awaitProfileOrderBarrier(t, started)
	close(release)
	result := <-reorderDone
	if result.err != nil || !result.changed || result.revision != 1 {
		t.Fatalf("reorder before mutation = revision %d changed %t err %v; want committed revision 1", result.revision, result.changed, result.err)
	}
	if err := <-mutationDone; err != nil {
		t.Fatalf("membership mutation after reorder: %v", err)
	}
}

func globalProfileOrderIDs(ctx context.Context, repo *sqliteRepository, agentID string) ([]string, error) {
	profiles, err := repo.ListAgentProfiles(ctx, agentID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.WorkspaceID == "" {
			ids = append(ids, profile.ID)
		}
	}
	return ids, nil
}

func awaitProfileOrderBarrier(t *testing.T, barrier <-chan struct{}) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for profile-order concurrency barrier")
	}
}

func assertProfileOrderMutationResult(t *testing.T, ctx context.Context, repo *sqliteRepository, scenario, aID, bID, sourceID string) {
	t.Helper()
	if scenario == "agent-cascade-delete" {
		if _, err := repo.GetAgent(ctx, aID); err == nil {
			t.Fatal("agent still exists after cascade delete")
		}
		return
	}
	profilesA, err := repo.ListAgentProfiles(ctx, aID)
	if err != nil {
		t.Fatal(err)
	}
	profilesB, err := repo.ListAgentProfiles(ctx, bID)
	if err != nil {
		t.Fatal(err)
	}
	contains := func(profiles []*models.AgentProfile, id string) bool {
		for _, profile := range profiles {
			if profile.ID == id && profile.WorkspaceID == "" {
				return true
			}
		}
		return false
	}
	switch scenario {
	case "create", "duplicate":
		ids, err := globalProfileOrderIDs(ctx, repo, aID)
		if err != nil || len(ids) != 3 || ids[0] == sourceID {
			t.Fatalf("new profile membership/order = %v, err %v; want three profiles with new profile first", ids, err)
		}
	case "soft-delete", "global-to-workspace":
		if contains(profilesA, sourceID) {
			t.Fatalf("profile %s remains in agent %s global membership", sourceID, aID)
		}
	case "global-owner-a-to-b":
		if contains(profilesA, sourceID) || !contains(profilesB, sourceID) {
			t.Fatalf("profile ownership: agent A contains=%t, agent B contains=%t", contains(profilesA, sourceID), contains(profilesB, sourceID))
		}
	case "workspace-to-global":
		if !contains(profilesA, sourceID) {
			t.Fatal("promoted workspace profile is absent from global membership")
		}
	}
}

func TestSQLiteWorkspaceProfileDeleteRetriesAfterPromotionAndReorder(t *testing.T) {
	runWorkspaceProfileDeletePromotionRace(t, openSQLiteProfileOrderRaceRepo(t))
}

func TestPostgresWorkspaceProfileDeleteRetriesAfterPromotionAndReorder(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize profile order schema: %v", err)
	}
	runWorkspaceProfileDeletePromotionRace(t, repo)
}

func runWorkspaceProfileDeletePromotionRace(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	oldOwner := &models.Agent{Name: "workspace-delete-old-owner"}
	newOwner := &models.Agent{Name: "workspace-delete-new-owner"}
	for _, agent := range []*models.Agent{oldOwner, newOwner} {
		if err := repo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	profiles := []*models.AgentProfile{
		{AgentID: newOwner.ID, Name: "first", Model: "model"},
		{AgentID: newOwner.ID, Name: "second", Model: "model"},
		{AgentID: oldOwner.ID, WorkspaceID: "workspace-delete-race", Name: "promoted", Model: "model"},
	}
	for _, profile := range profiles {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	promotedID := profiles[2].ID

	deleteObserved := make(chan struct{})
	releaseDelete := make(chan struct{})
	deleteRetryObserved := make(chan struct{})
	releaseDeleteRetry := make(chan struct{})
	reorderLocked := make(chan struct{})
	releaseReorder := make(chan struct{})
	var ownershipReads []struct {
		agentID     string
		workspaceID string
	}
	var ownershipMu sync.Mutex
	var deleteLocks []string
	var lockMu sync.Mutex
	repo.profileOrderAfterOwnershipRead = func(profileID, agentID, workspaceID string) error {
		if profileID != promotedID {
			return nil
		}
		ownershipMu.Lock()
		ownershipReads = append(ownershipReads, struct {
			agentID     string
			workspaceID string
		}{agentID, workspaceID})
		readCount := len(ownershipReads)
		ownershipMu.Unlock()
		switch readCount {
		case 1:
			close(deleteObserved)
			<-releaseDelete
		case 3:
			close(deleteRetryObserved)
			<-releaseDeleteRetry
		}
		return nil
	}
	repo.profileOrderAfterLock = func(operation, agentID string) error {
		if operation == "delete-profile" {
			lockMu.Lock()
			deleteLocks = append(deleteLocks, agentID)
			lockMu.Unlock()
		}
		if operation == "reorder" && agentID == newOwner.ID {
			close(reorderLocked)
			<-releaseReorder
		}
		return nil
	}
	t.Cleanup(func() {
		select {
		case <-releaseDelete:
		default:
			close(releaseDelete)
		}
		select {
		case <-releaseDeleteRetry:
		default:
			close(releaseDeleteRetry)
		}
		select {
		case <-releaseReorder:
		default:
			close(releaseReorder)
		}
	})

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- repo.DeleteAgentProfile(ctx, promotedID) }()
	select {
	case <-deleteObserved:
	case err := <-deleteDone:
		t.Fatalf("delete completed without entering the ownership barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for delete ownership barrier")
	}

	promoted, err := repo.GetAgentProfile(ctx, promotedID)
	if err != nil {
		t.Fatal(err)
	}
	promoted.AgentID = newOwner.ID
	promoted.WorkspaceID = ""
	if err := repo.UpdateAgentProfile(ctx, promoted); err != nil {
		t.Fatalf("promote workspace profile while delete is paused: %v", err)
	}
	ids, err := globalProfileOrderIDs(ctx, repo, newOwner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("promoted global membership = %v, want three profiles", ids)
	}
	reordered := []string{ids[2], ids[1], ids[0]}
	reorderDone := make(chan profileOrderReorderResult, 1)
	close(releaseDelete)
	awaitProfileOrderBarrier(t, deleteRetryObserved)
	go func() {
		revision, changed, err := repo.ReorderAgentProfiles(ctx, newOwner.ID, reordered)
		reorderDone <- profileOrderReorderResult{revision: revision, changed: changed, err: err}
	}()
	awaitProfileOrderBarrier(t, reorderLocked)
	close(releaseDeleteRetry)
	close(releaseReorder)
	reorderResult := <-reorderDone
	if reorderResult.err != nil || !reorderResult.changed || reorderResult.revision != 1 {
		t.Fatalf("reorder after promotion = revision %d changed %t err %v; want committed revision 1", reorderResult.revision, reorderResult.changed, reorderResult.err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("delete after promotion and reorder: %v", err)
	}
	if _, err := repo.GetAgentProfile(ctx, promotedID); err == nil {
		t.Fatal("promoted profile remains in global membership after delete")
	}
	finalIDs, err := globalProfileOrderIDs(ctx, repo, newOwner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalIDs) != 2 || finalIDs[0] != reordered[0] || finalIDs[1] != reordered[1] {
		t.Fatalf("global profile order after delete = %v, want %v", finalIDs, reordered[:2])
	}
	lockMu.Lock()
	gotDeleteLocks := append([]string(nil), deleteLocks...)
	lockMu.Unlock()
	if len(gotDeleteLocks) != 2 || gotDeleteLocks[0] != oldOwner.ID || gotDeleteLocks[1] != newOwner.ID {
		t.Fatalf("delete membership locks = %v, want retry locks [%s %s]", gotDeleteLocks, oldOwner.ID, newOwner.ID)
	}
	ownershipMu.Lock()
	gotReads := append([]struct {
		agentID     string
		workspaceID string
	}(nil), ownershipReads...)
	ownershipMu.Unlock()
	if len(gotReads) != 3 ||
		gotReads[0].agentID != oldOwner.ID || gotReads[0].workspaceID != profiles[2].WorkspaceID ||
		gotReads[1].agentID != oldOwner.ID || gotReads[1].workspaceID != profiles[2].WorkspaceID ||
		gotReads[2].agentID != newOwner.ID || gotReads[2].workspaceID != "" {
		t.Fatalf("ownership reads = %+v, want delete stale owner, promotion source, then delete refreshed owner", gotReads)
	}
}
