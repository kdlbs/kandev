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
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type sshOrphanSweepWiringRepo struct {
	listed chan struct{}
}

func (r *sshOrphanSweepWiringRepo) GetExecutor(context.Context, string) (*models.Executor, error) {
	return nil, nil
}

func (r *sshOrphanSweepWiringRepo) ListSSHExecutorsForReachability(context.Context) ([]*models.Executor, error) {
	select {
	case r.listed <- struct{}{}:
	default:
	}
	return nil, nil
}

func (r *sshOrphanSweepWiringRepo) GetExecutorReachability(context.Context, string) (*models.ExecutorReachability, error) {
	return nil, models.ErrExecutorReachabilityNotFound
}

func (r *sshOrphanSweepWiringRepo) GetTask(context.Context, string) (*models.Task, error) {
	return nil, repoerrors.ErrTaskNotFound
}

func (r *sshOrphanSweepWiringRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, nil
}

func (r *sshOrphanSweepWiringRepo) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return nil, nil
}

func (r *sshOrphanSweepWiringRepo) ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error) {
	return nil, nil
}

// TestStartAgentInfrastructureUsesSSHOrphanSweepWiringHelper mirrors
// TestStartAgentInfrastructureUsesSSHReachabilityPollerWiringHelper: the
// composition root must delegate to the narrow, independently-testable
// wiring helper exactly once rather than inlining the scheduler
// construction.
func TestStartAgentInfrastructureUsesSSHOrphanSweepWiringHelper(t *testing.T) {
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
		if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "startSSHOrphanSweepScheduler" {
			helperCalls++
		}
		return true
	})
	if helperCalls != 1 {
		t.Fatalf("startSSHOrphanSweepScheduler calls = %d, want exactly one composition call", helperCalls)
	}
}

func TestStartSSHOrphanSweepScheduler_StartsSchedulerAndRegistersCleanup(t *testing.T) {
	repo := &sshOrphanSweepWiringRepo{listed: make(chan struct{}, 1)}
	var cleanups []func() error
	addCleanup := func(fn func() error) func() error {
		cleanups = append(cleanups, fn)
		return fn
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler := startSSHOrphanSweepScheduler(ctx, repo, nil, logger.Default(), addCleanup, nil)
	if scheduler == nil {
		t.Fatal("startSSHOrphanSweepScheduler returned a nil scheduler")
	}
	if len(cleanups) != 1 {
		t.Fatalf("cleanups registered = %d, want exactly 1", len(cleanups))
	}

	select {
	case <-repo.listed:
	case <-time.After(time.Second):
		t.Fatal("scheduler never ran its immediate interval pass")
	}

	if err := cleanups[0](); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
