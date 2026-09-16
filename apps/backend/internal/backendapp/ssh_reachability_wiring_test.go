package backendapp

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type sshReachabilityWiringRepo struct {
	listed chan struct{}
}

func (r *sshReachabilityWiringRepo) ListSSHExecutorsForReachability(context.Context) ([]*models.Executor, error) {
	select {
	case r.listed <- struct{}{}:
	default:
	}
	return nil, nil
}

func (r *sshReachabilityWiringRepo) GetExecutorReachability(context.Context, string) (*models.ExecutorReachability, error) {
	return nil, models.ErrExecutorReachabilityNotFound
}

func (r *sshReachabilityWiringRepo) UpsertExecutorReachability(context.Context, models.ExecutorReachabilityObservation) error {
	return nil
}

func (r *sshReachabilityWiringRepo) ResetExecutorReachability(context.Context, string, string) error {
	return nil
}

// TestStartAgentInfrastructureUsesSSHReachabilityPollerWiringHelper mirrors
// TestInitOfficeServicesUsesTaskUsageWiringHelper: the composition root
// (startAgentInfrastructure, too large to exercise directly in a unit test)
// must delegate to the narrow, independently-testable wiring helper exactly
// once rather than inlining the poller construction.
func TestStartAgentInfrastructureUsesSSHReachabilityPollerWiringHelper(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var target *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "startAgentInfrastructure" {
			target = fn
			break
		}
	}
	if target == nil {
		t.Fatal("startAgentInfrastructure not found in main.go")
	}

	var helperCalls int
	ast.Inspect(target, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "startSSHReachabilityPoller" {
			helperCalls++
		}
		return true
	})
	if helperCalls != 1 {
		t.Fatalf("startSSHReachabilityPoller calls = %d, want exactly one composition call", helperCalls)
	}
}

func TestStartSSHReachabilityPoller_StartsPollerAndRegistersCleanup(t *testing.T) {
	repo := &sshReachabilityWiringRepo{listed: make(chan struct{}, 1)}
	var cleanups []func() error
	addCleanup := func(fn func() error) func() error {
		cleanups = append(cleanups, fn)
		return fn
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller := startSSHReachabilityPoller(ctx, repo, 3600, logger.Default(), addCleanup)
	if poller == nil {
		t.Fatal("startSSHReachabilityPoller returned a nil poller")
	}
	if len(cleanups) != 1 {
		t.Fatalf("cleanups registered = %d, want exactly 1", len(cleanups))
	}

	select {
	case <-repo.listed:
	case <-time.After(time.Second):
		t.Fatal("poller never ran its immediate pass")
	}

	if err := cleanups[0](); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
