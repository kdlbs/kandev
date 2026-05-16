package costs_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// noopActivity implements shared.ActivityLogger.
type noopActivity struct{}

func (n *noopActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// noopAgentReader implements shared.AgentReader.
type noopAgentReader struct{}

func (n *noopAgentReader) GetAgentInstance(_ context.Context, _ string) (*models.AgentInstance, error) {
	return nil, nil
}

func (n *noopAgentReader) ListAgentInstances(_ context.Context, _ string) ([]*models.AgentInstance, error) {
	return nil, nil
}

func (n *noopAgentReader) ListAgentInstancesByIDs(_ context.Context, _ []string) ([]*models.AgentInstance, error) {
	return nil, nil
}

// noopAgentWriter implements shared.AgentWriter.
type noopAgentWriter struct{}

func (n *noopAgentWriter) UpdateAgentStatusFields(_ context.Context, _, _, _ string) error {
	return nil
}

// newTestCostService creates a CostService with an in-memory SQLite repo.
func newTestCostService(t *testing.T) (*costs.CostService, func(string, ...interface{})) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: is per-connection; pin to one to keep the schema in scope
	// across concurrent queries (e.g. GetCostsBreakdown's errgroup).
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Create the tasks table (needed for JOIN in some repo queries).
	// Must include description and identifier so FTS5 triggers don't fail on INSERT.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		title TEXT DEFAULT '',
		description TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

	// Stub agent_profiles for the by-agent breakdown LEFT JOIN.
	// Production schema lives in the agent settings package; tests only need
	// the columns the cost queries touch (id, name).
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create agent_profiles table: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	svc := costs.NewCostService(repo, log, &noopActivity{}, &noopAgentReader{}, &noopAgentWriter{})

	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}

	return svc, execSQL
}

// TestRecordCostEvent_StoresCallerValues confirms that RecordCostEvent
// is a verbatim writer post-refactor: cost computation moved to the
// office subscriber (Layer A / B lookup). The helper now records
// whatever the caller supplies.
func TestRecordCostEvent_StoresCallerValues(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	ctx := context.Background()

	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)

	event, err := svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		int64(1_000_000), int64(500_000), int64(100_000), int64(465),
		false,
	)
	if err != nil {
		t.Fatalf("RecordCostEvent: %v", err)
	}
	if event.CostSubcents != 465 {
		t.Errorf("cost_subcents = %d, want 465", event.CostSubcents)
	}
	if event.Estimated {
		t.Error("estimated should be false")
	}
}

func TestRecordCostEvent_FlagsEstimated(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	ctx := context.Background()

	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)

	event, err := svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"codex-acp-model", "openai",
		int64(500), int64(0), int64(0), int64(0),
		true,
	)
	if err != nil {
		t.Fatalf("RecordCostEvent: %v", err)
	}
	if !event.Estimated {
		t.Error("estimated should be true for synthesised codex-acp delta")
	}
}

func TestGetCostSummary(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	ctx := context.Background()

	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)
	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-2', 'ws-1')`)

	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		int64(1_000_000), int64(0), int64(0), int64(300),
		false,
	)
	_, _ = svc.RecordCostEvent(ctx,
		"sess-2", "task-2", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		int64(2_000_000), int64(0), int64(0), int64(600),
		false,
	)

	total, err := svc.GetCostSummary(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}
	if total != 900 {
		t.Errorf("total = %d, want 900", total)
	}
}

// GetCostsBreakdown runs four sub-queries concurrently via errgroup. Verify
// the closure wiring is correct (each Go func writes to the right field) and
// that nil byAgent/byProject/byModel slices get normalised to empty slices.
func TestGetCostsBreakdown_ReturnsAllViews(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	ctx := context.Background()

	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)
	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-2', 'ws-1')`)

	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-A", "proj-X",
		"claude-sonnet-4", "anthropic",
		int64(1_000_000), int64(0), int64(0), int64(300),
		false,
	)
	_, _ = svc.RecordCostEvent(ctx,
		"sess-2", "task-2", "agent-B", "proj-Y",
		"claude-sonnet-4", "anthropic",
		int64(2_000_000), int64(0), int64(0), int64(600),
		false,
	)

	total, byAgent, byProject, byModel, byProvider, err := svc.GetCostsBreakdown(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetCostsBreakdown: %v", err)
	}
	if total != 900 {
		t.Errorf("total = %d, want 900", total)
	}
	if len(byAgent) != 2 {
		t.Errorf("byAgent rows = %d, want 2", len(byAgent))
	}
	if len(byProject) != 2 {
		t.Errorf("byProject rows = %d, want 2", len(byProject))
	}
	if len(byModel) != 1 {
		t.Errorf("byModel rows = %d, want 1", len(byModel))
	}
	if len(byProvider) != 1 || byProvider[0].GroupLabel != "Claude" {
		t.Errorf("byProvider = %+v, want one row labelled Claude", byProvider)
	}
}

// RecordCostEvent must persist whatever (model, provider) the caller
// supplies — including the routed values that the office scheduler
// resolves at launch time (resolved_model / resolved_provider_id on the
// Run row).
func TestRecordCostEvent_PersistsRoutedProviderAndModel(t *testing.T) {
	svc, execSQL := newTestCostService(t)
	ctx := context.Background()
	execSQL(`INSERT OR IGNORE INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`)

	event, err := svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		int64(1_000_000), int64(0), int64(100_000), int64(300),
		false,
	)
	if err != nil {
		t.Fatalf("RecordCostEvent: %v", err)
	}
	if event.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic (routed value)", event.Provider)
	}
	if event.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want claude-sonnet-4 (routed value)", event.Model)
	}

	events, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListCostEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Provider != "anthropic" || events[0].Model != "claude-sonnet-4" {
		t.Errorf("persisted event = (%q,%q), want (anthropic,claude-sonnet-4)",
			events[0].Provider, events[0].Model)
	}
}

// Empty workspace must return zero total and empty (non-nil) slices.
func TestGetCostsBreakdown_EmptyWorkspace(t *testing.T) {
	svc, _ := newTestCostService(t)
	ctx := context.Background()

	total, byAgent, byProject, byModel, byProvider, err := svc.GetCostsBreakdown(ctx, "ws-empty")
	if err != nil {
		t.Fatalf("GetCostsBreakdown: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if byAgent == nil || byProject == nil || byModel == nil || byProvider == nil {
		t.Error("expected empty (non-nil) slices for empty workspace")
	}
	if len(byAgent) != 0 || len(byProject) != 0 || len(byModel) != 0 || len(byProvider) != 0 {
		t.Errorf("expected empty slices, got %d/%d/%d/%d",
			len(byAgent), len(byProject), len(byModel), len(byProvider))
	}
}
