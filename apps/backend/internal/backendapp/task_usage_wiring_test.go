package backendapp

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

type taskUsageWiringRepo struct {
	recorded chan *models.TaskUsageEvent
}

func (r *taskUsageWiringRepo) CreateTaskUsageEvent(_ context.Context, event *models.TaskUsageEvent) error {
	r.recorded <- event
	return nil
}

func TestInitOfficeServicesUsesTaskUsageWiringHelper(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var initOffice *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "initOfficeServices" {
			initOffice = fn
			break
		}
	}
	if initOffice == nil {
		t.Fatal("initOfficeServices not found in main.go")
	}

	var helperCalls int
	ast.Inspect(initOffice, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "startTaskUsageWriter" {
			helperCalls++
		}
		return true
	})
	if helperCalls != 1 {
		t.Fatalf("startTaskUsageWriter calls = %d, want exactly one composition call", helperCalls)
	}
}

func TestStartTaskUsageWriter_PublishesIntoRepository(t *testing.T) {
	repo := &taskUsageWiringRepo{recorded: make(chan *models.TaskUsageEvent, 1)}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	var cleanups []func() error
	addCleanup := func(cleanup func() error) {
		cleanups = append(cleanups, cleanup)
	}
	if err := startTaskUsageWriter(repo, nil, eventBus, nil, addCleanup); err != nil {
		t.Fatalf("startTaskUsageWriter: %v", err)
	}
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})

	if err := eventBus.Publish(context.Background(), events.BuildSessionPromptUsageSubject("session-1"), &bus.Event{
		Data: map[string]any{
			"task_id":        "task-1",
			"session_id":     "session-1",
			"usage_event_id": "evt-wiring",
			"usage":          map[string]any{"input_tokens": int64(7)},
		},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case event := <-repo.recorded:
		if event.UsageEventID != "evt-wiring" || event.TokensIn != 7 {
			t.Fatalf("recorded event = %+v, want usage event evt-wiring with 7 input tokens", event)
		}
	case <-time.After(time.Second):
		t.Fatal("task usage writer did not receive the published event")
	}
}
