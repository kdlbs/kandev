package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

type failingAutoMergeReadRepository struct {
	Repository
}

func (r *failingAutoMergeReadRepository) GetAutoMergeOverride(
	context.Context,
	QueueSessionIdentity,
) (*AutoMergeOverride, error) {
	return nil, errors.New("policy unavailable")
}

type pausingAutoMergePolicyRepository struct {
	Repository
	admission autoMergeAdmissionRepository
	captured  chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (r *pausingAutoMergePolicyRepository) GetAutoMergeOverride(
	ctx context.Context,
	identity QueueSessionIdentity,
) (*AutoMergeOverride, error) {
	override, err := r.Repository.GetAutoMergeOverride(ctx, identity)
	r.once.Do(func() {
		close(r.captured)
		<-r.release
	})
	return override, err
}

func (r *pausingAutoMergePolicyRepository) InsertForSessionWithPolicy(
	ctx context.Context,
	identity QueueSessionIdentity,
	message *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy AutoMergePolicy,
) error {
	return r.admission.InsertForSessionWithPolicy(ctx, identity, message, claim, maxPerSession, policy)
}

func (r *pausingAutoMergePolicyRepository) AutoMergeCandidateIntoAboveForSessionWithPolicy(
	ctx context.Context,
	identity QueueSessionIdentity,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	policy AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	return r.admission.AutoMergeCandidateIntoAboveForSessionWithPolicy(ctx, identity, candidate, claim, policy)
}

func (r *pausingAutoMergePolicyRepository) AutoMergeIntoAboveForSessionWithPolicy(
	ctx context.Context,
	identity QueueSessionIdentity,
	sourceID string,
	policy AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	return r.admission.AutoMergeIntoAboveForSessionWithPolicy(ctx, identity, sourceID, policy)
}

func (r *failingAutoMergeReadRepository) Snapshot(
	ctx context.Context,
	identity QueueSessionIdentity,
) (RepositorySnapshot, error) {
	snapshot, err := r.Repository.Snapshot(ctx, identity)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	snapshot.AutoMergeOverride = nil
	snapshot.AutoMergeError = errors.New("policy unavailable")
	return snapshot, nil
}

type autoMergePolicyService interface {
	SetAutoMergePolicy(bool, int64)
	ResolveAutoMergePolicy(context.Context, QueueSessionIdentity) (AutoMergePolicy, error)
	SetSessionAutoMerge(context.Context, QueueSessionIdentity, bool) (AutoMergePolicy, error)
}
type identityQueueAdmission interface {
	QueueMessageWithMetadataForSession(
		context.Context,
		QueueSessionIdentity,
		string,
		string,
		string,
		bool,
		[]MessageAttachment,
		map[string]interface{},
	) (*QueuedMessage, error)
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
func TestServiceResolvesGlobalPolicyUntilSessionOverride(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	repository := NewMemoryRepository()
	service := NewService(repository, DefaultMaxPerSession, log)
	policies, ok := interface{}(service).(autoMergePolicyService)
	if !ok {
		t.Fatal("message queue service does not expose effective Auto-merge policy")
	}
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation"}
	untouched := QueueSessionIdentity{TaskID: "task-2", SessionID: "session-2", SessionIncarnationID: "incarnation-2"}
	seedQueueSessionIdentity(t, repository, identity)
	seedQueueSessionIdentity(t, repository, untouched)

	policies.SetAutoMergePolicy(true, 4)
	resolved, err := policies.ResolveAutoMergePolicy(ctx, identity)
	assertAutoMergePolicy(t, resolved, err, AutoMergePolicy{
		Enabled: true, Source: AutoMergeSourceGlobal, Revision: 4,
	})
	written, err := policies.SetSessionAutoMerge(ctx, identity, false)
	if err != nil {
		t.Fatalf("set session Auto-merge: %v", err)
	}
	if written != (AutoMergePolicy{Enabled: false, Source: AutoMergeSourceSession, Revision: 1}) {
		t.Fatalf("written session policy = %+v", written)
	}

	policies.SetAutoMergePolicy(false, 5)
	resolved, err = policies.ResolveAutoMergePolicy(ctx, identity)
	assertAutoMergePolicy(t, resolved, err, written)
	resolved, err = policies.ResolveAutoMergePolicy(ctx, untouched)
	assertAutoMergePolicy(t, resolved, err, AutoMergePolicy{
		Enabled: false, Source: AutoMergeSourceGlobal, Revision: 5,
	})
}

func TestServiceReloadsSharedGlobalPolicyForIdentity(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository, DefaultMaxPerSession, logger.Default())
	identity := QueueSessionIdentity{
		TaskID: "task-shared", SessionID: "session-shared", SessionIncarnationID: "inc-shared",
	}
	seedQueueSessionIdentity(t, repository, identity)
	service.SetAutoMergePolicy(true, 2)
	service.SetAutoMergePolicyLoader(func(context.Context) (bool, int64, error) {
		return false, 3, nil
	})

	resolved, err := service.ResolveAutoMergePolicy(context.Background(), identity)
	assertAutoMergePolicy(t, resolved, err, AutoMergePolicy{
		Enabled: false, Source: AutoMergeSourceGlobal, Revision: 3,
	})
	if service.AutoMergeEnabled() {
		t.Fatal("persisted policy reload did not refresh the local cache")
	}
	status, err := service.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("snapshot shared policy: %v", err)
	}
	if !status.AutoMergeAvailable || status.AutoMergeEnabled == nil ||
		*status.AutoMergeEnabled || status.AutoMergeRevision == nil ||
		*status.AutoMergeRevision != 3 {
		t.Fatalf("snapshot shared policy = %+v, want false revision 3", status)
	}
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
func TestIdentityAdmissionUsesOneEffectiveSessionPolicy(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository, DefaultMaxPerSession, logger.Default())
	admissions, ok := interface{}(service).(identityQueueAdmission)
	if !ok {
		t.Fatal("message queue service does not expose identity-bound admission")
	}
	ctx := context.Background()
	explicitOff := QueueSessionIdentity{TaskID: "task-off", SessionID: "session-off", SessionIncarnationID: "inc-off"}
	inheritedOn := QueueSessionIdentity{TaskID: "task-on", SessionID: "session-on", SessionIncarnationID: "inc-on"}
	seedQueueSessionIdentity(t, repository, explicitOff)
	seedQueueSessionIdentity(t, repository, inheritedOn)
	service.SetAutoMergePolicy(true, 2)
	if _, err := service.SetSessionAutoMerge(ctx, explicitOff, false); err != nil {
		t.Fatalf("set explicit policy: %v", err)
	}
	for _, identity := range []QueueSessionIdentity{explicitOff, inheritedOn} {
		for _, content := range []string{"first", "second"} {
			if _, err := admissions.QueueMessageWithMetadataForSession(
				ctx, identity, content, "", QueuedByUser, false, nil, nil,
			); err != nil {
				t.Fatalf("queue %s: %v", identity.SessionID, err)
			}
		}
	}
	offEntries, err := service.repo.ListBySession(ctx, explicitOff.SessionID)
	if err != nil {
		t.Fatalf("list explicit OFF queue: %v", err)
	}
	onEntries, err := service.repo.ListBySession(ctx, inheritedOn.SessionID)
	if err != nil {
		t.Fatalf("list inherited ON queue: %v", err)
	}
	if len(offEntries) != 2 || len(onEntries) != 1 {
		t.Fatalf("entry counts: explicit OFF=%d inherited ON=%d", len(offEntries), len(onEntries))
	}
}

func TestIdentityAdmissionSkipsAutomaticMergeWhenPolicyReadFails(t *testing.T) {
	baseRepository := NewMemoryRepository()
	seedQueueSessionIdentity(t, baseRepository, QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation",
	})
	repository := &failingAutoMergeReadRepository{Repository: baseRepository}
	service := NewService(repository, DefaultMaxPerSession, logger.Default())
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation",
	}
	for _, content := range []string{"first", "second"} {
		if _, err := service.QueueMessageWithMetadataForSession(
			context.Background(), identity, content, "", QueuedByUser, false, nil, nil,
		); err != nil {
			t.Fatalf("queue with unavailable policy: %v", err)
		}
	}
	entries, err := repository.ListBySession(context.Background(), identity.SessionID)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("queue length = %d, want 2 separate rows", len(entries))
	}
}

func TestIdentityAdmissionRechecksPolicyChangedByAnotherConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue-policy-race.db")
	open := func() *sql.DB {
		raw, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		raw.SetMaxOpenConns(2)
		t.Cleanup(func() { _ = raw.Close() })
		return raw
	}
	firstRaw := open()
	secondRaw := open()
	if _, err := firstRaw.Exec(`
		CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at TIMESTAMP, updated_at TIMESTAMP);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			queue_incarnation_id TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("create shared authority tables: %v", err)
	}
	firstDB := sqlx.NewDb(firstRaw, "sqlite3")
	secondDB := sqlx.NewDb(secondRaw, "sqlite3")
	firstRepository, err := NewSQLiteRepository(firstDB, firstDB)
	if err != nil {
		t.Fatalf("create first repository: %v", err)
	}
	secondRepository, err := NewSQLiteRepository(secondDB, secondDB)
	if err != nil {
		t.Fatalf("create second repository: %v", err)
	}
	identity := QueueSessionIdentity{
		TaskID: "task-policy-race", SessionID: "session-policy-race", SessionIncarnationID: "inc-policy-race",
	}
	if _, err := firstRaw.Exec(`INSERT INTO tasks (id, updated_at) VALUES (?, CURRENT_TIMESTAMP)`, identity.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := firstRaw.Exec(`
		INSERT INTO task_sessions (id, task_id, queue_incarnation_id) VALUES (?, ?, ?)
	`, identity.SessionID, identity.TaskID, identity.SessionIncarnationID); err != nil {
		t.Fatal(err)
	}
	firstSQLite := firstRepository.(*sqliteRepository)
	secondSQLite := secondRepository.(*sqliteRepository)
	firstSQLite.tasksTablePresent = true
	secondSQLite.tasksTablePresent = true
	if err := firstRepository.InsertForSession(context.Background(), identity, &QueuedMessage{
		TaskID: identity.TaskID, SessionID: identity.SessionID, Content: "first", QueuedBy: QueuedByUser,
	}, DefaultMaxPerSession); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	pausing := &pausingAutoMergePolicyRepository{
		Repository: firstRepository,
		admission:  firstSQLite,
		captured:   make(chan struct{}),
		release:    make(chan struct{}),
	}
	firstService := NewService(pausing, DefaultMaxPerSession, logger.Default())
	secondService := NewService(secondRepository, DefaultMaxPerSession, logger.Default())

	result := make(chan error, 1)
	go func() {
		_, queueErr := firstService.QueueMessageWithMetadataForSession(
			context.Background(), identity, "second", "", QueuedByUser, false, nil, nil,
		)
		result <- queueErr
	}()
	<-pausing.captured
	if _, err := secondService.SetSessionAutoMerge(context.Background(), identity, false); err != nil {
		t.Fatalf("change policy from second connection: %v", err)
	}
	close(pausing.release)
	if err := <-result; err != nil {
		t.Fatalf("admit after policy race: %v", err)
	}
	entries, err := firstRepository.ListBySession(context.Background(), identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Content != "first" || entries[1].Content != "second" {
		t.Fatalf("queue after OFF override race = %+v", entries)
	}
}

func assertAutoMergePolicy(t *testing.T, got AutoMergePolicy, err error, want AutoMergePolicy) {
	t.Helper()
	if err != nil {
		t.Fatalf("resolve Auto-merge policy: %v", err)
	}
	if got != want {
		t.Fatalf("Auto-merge policy = %+v, want %+v", got, want)
	}
}
