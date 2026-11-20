package changes

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-003.6
func TestCoordinatorStartPersistsCapturedCheckoutsWhenBudgetExpires(t *testing.T) {
	admission := admissionWithStartCaptureCheckouts(2)
	coordinator, repo := newSQLiteStartBudgetCoordinator(t, admission)
	coordinator.admissionTimeout = time.Second
	coordinator.startPersistenceReserve = 700 * time.Millisecond
	client := newCoordinatorStartCaptureClient(admission)
	client.blockedCheckout = "checkout-2"

	err := coordinator.Admit(context.Background(), admission, client)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	set, getErr := repo.GetTurnChangeSet(context.Background(), admission.TaskID, admission.SessionID, turnChangeSetID(admission.TurnID))
	require.NoError(t, getErr)
	require.True(t, set.StartAccepted, "admit error: %v", err)
	rows, listErr := repo.ListTurnRepositoryChanges(context.Background(), set.ID)
	require.NoError(t, listErr)
	require.Len(t, rows, 2)
	byCheckout := make(map[string]*models.TurnRepositoryChangeSet, len(rows))
	for _, row := range rows {
		byCheckout[row.CheckoutID] = row
	}
	first := byCheckout["checkout-1"]
	require.NotNil(t, first)
	require.Equal(t, models.TurnChangeAvailabilityPending, first.Availability)
	require.Equal(t, "commit-checkout-1", first.StartCommitOID)
	require.Equal(t, "tree-checkout-1", first.StartTreeOID)
	second := byCheckout["checkout-2"]
	require.NotNil(t, second)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, second.Availability)
	require.Equal(t, models.TurnChangeReasonCaptureFailed, second.Reason)
	require.Empty(t, second.StartCommitOID)
	require.ErrorContains(t, err, `checkout "checkout-2"`)
	require.NotContains(t, err.Error(), "persist turn change start endpoints")
}

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-003.6
func TestCoordinatorStartCapturesCheckoutsConcurrently(t *testing.T) {
	admission := admissionWithStartCaptureCheckouts(4)
	store := newCoordinatorStore()
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, nil, nil)
	release := make(chan struct{})
	client := newCoordinatorStartCaptureClient(admission)
	client.started = make(chan string, len(admission.Checkouts))
	client.release = release
	result := make(chan error, 1)
	go func() { result <- coordinator.Admit(context.Background(), admission, client) }()

	concurrent := true
waitForTwo:
	for range 2 {
		select {
		case <-client.started:
		case <-time.After(5 * time.Second):
			concurrent = false
			break waitForTwo
		}
	}
	close(release)
	require.NoError(t, <-result)
	require.True(t, concurrent, "two captures should be active at the same time")
	require.Equal(t, 2, client.maximumInFlight())
	rows := store.repositoryRows[turnChangeSetID(admission.TurnID)]
	gotOrder := make([]string, len(rows))
	for i, row := range rows {
		gotOrder[i] = row.CheckoutID
	}
	require.Equal(t, []string{"checkout-1", "checkout-2", "checkout-3", "checkout-4"}, gotOrder)
}

// @covers AC-TASKS-TURN-CHANGES-003.1
func TestCoordinatorStartCaptureConcurrencyIsSharedPerExecution(t *testing.T) {
	firstAdmission := admissionWithStartCaptureCheckouts(2)
	coordinator, repo := newSQLiteStartBudgetCoordinator(t, firstAdmission)
	coordinator.admissionTimeout = 15 * time.Second
	coordinator.startPersistenceReserve = 100 * time.Millisecond

	secondAdmission := firstAdmission
	secondAdmission.TurnID = "turn-2"
	secondAdmission.StartupAttemptID = "startup-2"
	require.NoError(t, repo.CreateTurn(context.Background(), &models.Turn{
		ID: secondAdmission.TurnID, TaskID: secondAdmission.TaskID,
		TaskSessionID: secondAdmission.SessionID, StartedAt: time.Now().UTC(),
	}))
	coordinator.turns = coordinatorTurnReaderMap{
		firstAdmission.TurnID:  {ID: firstAdmission.TurnID, TaskID: firstAdmission.TaskID, TaskSessionID: firstAdmission.SessionID},
		secondAdmission.TurnID: {ID: secondAdmission.TurnID, TaskID: secondAdmission.TaskID, TaskSessionID: secondAdmission.SessionID},
	}

	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	client := newCoordinatorStartCaptureClient(firstAdmission)
	client.started = make(chan string, 8)
	client.release = release
	firstResult := make(chan error, 1)
	go func() { firstResult <- coordinator.Admit(context.Background(), firstAdmission, client) }()

	for range 2 {
		select {
		case <-client.started:
		case <-time.After(5 * time.Second):
			t.Fatal("first admission did not start both captures")
		}
	}

	client.scopesStarted = make(chan struct{}, 1)
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- coordinator.Admit(secondCtx, secondAdmission, client) }()
	select {
	case <-client.scopesStarted:
	case err := <-secondResult:
		t.Fatalf("second admission ended before registering repository scopes: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("second admission did not reach repository scope lookup")
	}
	require.ErrorIs(t, <-secondResult, context.DeadlineExceeded)
	require.Equal(t, 2, client.maximumInFlight(), "overlapping admissions must share the execution capture bound")

	close(release)
	require.NoError(t, <-firstResult)
}

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-003.6
func TestCoordinatorStartCapturePersistenceReserveExceedsSQLiteBusyTimeout(t *testing.T) {
	require.Greater(t, turnChangeStartPersistenceReserve, 5*time.Second)
}

// @covers AC-TASKS-TURN-CHANGES-003.1, AC-TASKS-TURN-CHANGES-003.6
func TestCoordinatorStartPersistFailureReportsCaptureFailures(t *testing.T) {
	admission := admissionWithStartCaptureCheckouts(2)
	store := newCoordinatorStore()
	failedRepo := &failedStartPersistence{TurnChangesRepository: store}
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	coordinator := NewCoordinator(failedRepo, coordinatorTurnReader{}, policy, nil, nil)
	client := newCoordinatorStartCaptureClient(admission)
	client.captureErrors = map[string]error{
		"checkout-1": errors.New("capture rejected"),
		"checkout-2": errors.New("capture failed"),
	}

	err := coordinator.Admit(context.Background(), admission, client)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "persist turn change start endpoints")
	require.ErrorContains(t, err, `checkout "checkout-1"`)
	require.ErrorContains(t, err, `checkout "checkout-2"`)
}

type coordinatorStartCaptureClient struct {
	*coordinatorCheckpointClient
	mu              sync.Mutex
	inFlight        int
	maxInFlight     int
	blockedCheckout string
	captureErrors   map[string]error
	started         chan string
	scopesStarted   chan struct{}
	release         <-chan struct{}
}

func newCoordinatorStartCaptureClient(admission Admission) *coordinatorStartCaptureClient {
	scopes := make([]string, 0, len(admission.Checkouts))
	for _, checkout := range admission.Checkouts {
		scopes = append(scopes, checkout.RepositorySubpath)
	}
	return &coordinatorStartCaptureClient{coordinatorCheckpointClient: &coordinatorCheckpointClient{scopes: scopes}}
}

func (c *coordinatorStartCaptureClient) TurnCheckpointRepositoryScopes(ctx context.Context) ([]string, error) {
	if c.scopesStarted != nil {
		select {
		case c.scopesStarted <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return c.coordinatorCheckpointClient.TurnCheckpointRepositoryScopes(ctx)
}

func (c *coordinatorStartCaptureClient) CaptureTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.maxInFlight {
		c.maxInFlight = c.inFlight
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.inFlight--
		c.mu.Unlock()
	}()
	if c.started != nil {
		c.started <- request.CheckoutID
	}
	if request.CheckoutID == c.blockedCheckout {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := c.captureErrors[request.CheckoutID]; err != nil {
		return nil, err
	}
	return &turnchanges.CheckpointResult{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID, Boundary: request.Boundary,
		CommitOID: "commit-" + request.CheckoutID, TreeOID: "tree-" + request.CheckoutID,
		HashAlgorithm: "sha1", ReachabilityRef: "refs/kandev/turn-changes/" + request.CheckoutID,
		CapturedAt: time.Now().UTC(),
	}, nil
}

func (c *coordinatorStartCaptureClient) maximumInFlight() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxInFlight
}

func admissionWithStartCaptureCheckouts(count int) Admission {
	admission := coordinatorAdmission()
	first := admission.Checkouts[0]
	admission.Checkouts = make([]Checkout, count)
	for i := range count {
		checkout := first
		checkout.ID = fmt.Sprintf("checkout-%d", i+1)
		checkout.EnvironmentRepoID = fmt.Sprintf("env-repo-%d", i+1)
		checkout.TaskRepositoryID = fmt.Sprintf("task-repo-%d", i+1)
		checkout.RepositoryID = fmt.Sprintf("repo-%d", i+1)
		checkout.WorktreeID = fmt.Sprintf("worktree-%d", i+1)
		checkout.DisplayName = fmt.Sprintf("repo-%d", i+1)
		checkout.RepositorySubpath = fmt.Sprintf("repo-%d", i+1)
		checkout.RepositorySubpathKnown = true
		admission.Checkouts[i] = checkout
	}
	return admission
}

func newSQLiteStartBudgetCoordinator(t *testing.T, admission Admission) (*Coordinator, *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "start-budget.db"))
	require.NoError(t, err)
	store := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	repo, err := sqlite.NewWithDB(store, store, nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: admission.TaskID, Title: "Start capture budget"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: admission.TaskEnvironmentID, TaskID: admission.TaskID}))
	for index, checkout := range admission.Checkouts {
		require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
			ID: checkout.EnvironmentRepoID, TaskEnvironmentID: admission.TaskEnvironmentID,
			RepositoryID: checkout.RepositoryID, Position: index,
		}))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: admission.SessionID, TaskID: admission.TaskID, TaskEnvironmentID: admission.TaskEnvironmentID,
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: admission.TurnID, TaskID: admission.TaskID, TaskSessionID: admission.SessionID, StartedAt: time.Now().UTC(),
	}))
	policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{
		SettingsUserID: "user-initiator", Enabled: true, ResolutionKind: turnchanges.PolicyAuthenticatedUser,
	}}
	return NewCoordinator(repo, coordinatorTurnReader{}, policy, nil, nil), repo
}
