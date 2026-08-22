package plugins

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	wfrepo "github.com/kandev/kandev/internal/workflow/repository"
	wfservice "github.com/kandev/kandev/internal/workflow/service"
)

// The ListSteps merge is otherwise proven only against a fake
// workflowStepLister, which returns whatever monitoring the test handed it.
// That cannot show that a policy an operator actually saved comes back out
// on the step a plugin reads — the delivery path criteria 22 and 23 depend
// on. This wires the real workflow repository and service into the plugin
// host so the whole server-side chain (save -> SQLite -> service -> host
// merge -> DTO) runs with nothing stubbed.
func newRealWorkflowStepHost(t *testing.T) (*pluginHost, *wfservice.Service) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatalf("create workflows table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-real', 'ws-real', 'Delivery', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	repo, err := wfrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("workflow repository: %v", err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := wfservice.NewService(repo, log)
	t.Cleanup(func() { _ = svc.Close() })

	host := &pluginHost{
		pluginID:      "plugin-coordinator",
		capabilities:  manifest.Capabilities{APIRead: []string{"workflows"}, AgentConversation: true},
		workflows:     &fakeWorkflowLister{},
		workflowSteps: svc,
	}
	return host, svc
}

func TestListStepsDeliversRealSavedCoordinatorPolicy(t *testing.T) {
	host, svc := newRealWorkflowStepHost(t)
	ctx := context.Background()

	monitored := &wfmodels.WorkflowStep{WorkflowID: "wf-real", Name: "Work", Position: 0}
	if err := svc.CreateStep(ctx, monitored); err != nil {
		t.Fatalf("create monitored step: %v", err)
	}
	ignored := &wfmodels.WorkflowStep{WorkflowID: "wf-real", Name: "Done", Position: 1}
	if err := svc.CreateStep(ctx, ignored); err != nil {
		t.Fatalf("create unmonitored step: %v", err)
	}

	const prompt = "Check the branch is pushed before advancing."
	if _, err := svc.SetCoordinatorMonitoring(ctx, "ws-real", "wf-real", []wfmodels.CoordinatorStepMonitor{
		{WorkflowStepID: monitored.ID, Selected: true, Prompt: prompt},
	}); err != nil {
		t.Fatalf("SetCoordinatorMonitoring: %v", err)
	}

	steps, err := host.Workflows().ListSteps(ctx, "wf-real")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	// The workflow service seeds template steps of its own, so assert on the
	// steps this test created and require every other step to stay clean
	// rather than pinning a total count.
	sawMonitored, sawIgnored := false, false
	for _, step := range steps {
		switch step.ID {
		case monitored.ID:
			sawMonitored = true
			if !step.CoordinatorMonitored {
				t.Error("the saved policy did not reach the plugin: CoordinatorMonitored = false")
			}
			if step.CoordinatorPrompt != prompt {
				t.Errorf("CoordinatorPrompt = %q, want %q", step.CoordinatorPrompt, prompt)
			}
		default:
			if step.ID == ignored.ID {
				sawIgnored = true
			}
			if step.CoordinatorMonitored || step.CoordinatorPrompt != "" {
				t.Errorf("step %q carried policy it was never given: monitored=%v prompt=%q",
					step.ID, step.CoordinatorMonitored, step.CoordinatorPrompt)
			}
		}
	}
	if !sawMonitored {
		t.Fatalf("the monitored step %q was not returned by ListSteps", monitored.ID)
	}
	if !sawIgnored {
		t.Fatalf("the unmonitored step %q was not returned by ListSteps", ignored.ID)
	}
}

// Unchecking a step in Settings has to actually stop reaching the plugin,
// otherwise a coordinator keeps applying a prompt an operator removed.
func TestListStepsStopsDeliveringUncheckedPolicy(t *testing.T) {
	host, svc := newRealWorkflowStepHost(t)
	ctx := context.Background()

	step := &wfmodels.WorkflowStep{WorkflowID: "wf-real", Name: "Work", Position: 0}
	if err := svc.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if _, err := svc.SetCoordinatorMonitoring(ctx, "ws-real", "wf-real", []wfmodels.CoordinatorStepMonitor{
		{WorkflowStepID: step.ID, Selected: true, Prompt: "watch this"},
	}); err != nil {
		t.Fatalf("SetCoordinatorMonitoring (on): %v", err)
	}
	if _, err := svc.SetCoordinatorMonitoring(ctx, "ws-real", "wf-real", nil); err != nil {
		t.Fatalf("SetCoordinatorMonitoring (cleared): %v", err)
	}

	steps, err := host.Workflows().ListSteps(ctx, "wf-real")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	for _, s := range steps {
		if s.CoordinatorMonitored || s.CoordinatorPrompt != "" {
			t.Errorf("cleared policy still reaching the plugin on step %q: monitored=%v prompt=%q",
				s.ID, s.CoordinatorMonitored, s.CoordinatorPrompt)
		}
	}
}
