package backendapp

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agent/sandboxruntime"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type sandboxRuntimeFactory struct {
	runtime  *sandboxruntime.Manager
	policies *policy.Service
	tasks    *taskrepo.Repository
	config   *config.Config
}

func newSandboxRuntime(cfg *config.Config, policies *policy.Service, providers *plugins.Service) (*sandboxruntime.Manager, error) {
	if !cfg.Features.SandboxPolicies || providers == nil {
		return nil, nil
	}
	if policies == nil {
		return nil, errSandboxRuntimeUnavailable
	}
	base := filepath.Join(cfg.ResolvedDataDir(), "sandbox-runtime")
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, errSandboxRuntimeUnavailable
	}
	canonical, err := filepath.EvalSymlinks(base)
	if err != nil || canonical != base {
		return nil, errSandboxRuntimeUnavailable
	}
	info, err := os.Stat(base)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, errSandboxRuntimeUnavailable
	}
	manager := sandboxruntime.NewManager(policies, providers, base)
	policies.SetReportRefresher(manager.RefreshReport)
	policies.SetLearningCompiler(manager.CompileLearning)
	return manager, nil
}

func (f sandboxRuntimeFactory) prepareExecution(ctx context.Context, execution *runtime.AgentExecution, generation uint64, restartOf string, _ map[string]string, transport *runtime.AgentCtlClient) (runtime.SandboxAttempt, error) {
	if f.runtime == nil || execution == nil || execution.SandboxSelection == nil || execution.RuntimeName != executor.NameStandalone || transport == nil {
		return nil, errSandboxRuntimeUnavailable
	}
	if err := f.validateExecutor(ctx, execution.SandboxSelection.ExecutorID); err != nil {
		return nil, err
	}
	protected, err := f.protectedPorts(transport)
	if err != nil {
		return nil, err
	}
	restartOf, generation, err = f.previousAttempt(ctx, execution, restartOf, generation)
	if err != nil {
		return nil, err
	}
	return f.runtime.Prepare(ctx, sandboxruntime.PrepareRequest{RestartOf: restartOf, Selection: *execution.SandboxSelection, SessionID: execution.SessionID, ExecutionID: execution.ID, Generation: generation, WorkDir: execution.WorkspacePath, Arguments: execution.AgentArgs, ProtectedPorts: protected, MCPForward: transport.SandboxMCPForward})
}

func (f sandboxRuntimeFactory) validateExecutor(ctx context.Context, id string) error {
	selected, err := f.tasks.GetExecutor(ctx, id)
	if err != nil {
		return err
	}
	if selected == nil || selected.DeletedAt != nil || (selected.Type != models.ExecutorTypeLocal && selected.Type != models.ExecutorTypeWorktree) {
		return policy.ErrIneligible
	}
	return nil
}

func (f sandboxRuntimeFactory) protectedPorts(transport *runtime.AgentCtlClient) ([]uint16, error) {
	endpoint, err := url.Parse(transport.BaseURL())
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil {
		return nil, policy.ErrIneligible
	}
	host := endpoint.Hostname()
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return nil, policy.ErrIneligible
	}
	instancePort, err := strconv.Atoi(endpoint.Port())
	if err != nil {
		return nil, policy.ErrIneligible
	}
	ports := []uint16{}
	for _, port := range []int{f.config.Server.Port, f.config.Agent.StandalonePort, instancePort} {
		if port <= 0 || port > 65535 {
			return nil, policy.ErrIneligible
		}
		ports = append(ports, uint16(port))
	}
	slices.Sort(ports)
	return slices.Compact(ports), nil
}

func (f sandboxRuntimeFactory) previousAttempt(ctx context.Context, execution *runtime.AgentExecution, requested string, generation uint64) (string, uint64, error) {
	if execution.SessionID == "" {
		return requested, generation, nil
	}
	previous, err := f.policies.LatestSessionAttempt(ctx, execution.SandboxSelection.OrgID, execution.SessionID)
	if errors.Is(err, policy.ErrNotFound) && requested == "" {
		return "", generation, nil
	}
	if err != nil {
		return "", 0, err
	}
	if !sandboxRestartScopeMatches(execution, previous) {
		return "", 0, policy.ErrConflict
	}
	if requested != "" && requested != previous.ID {
		return "", 0, policy.ErrConflict
	}
	if previous.State != policy.AttemptStopped {
		previous, err = f.runtime.ReconcileAttempt(ctx, execution.SandboxSelection.OrgID, previous.ID)
		if err != nil {
			return "", 0, err
		}
	}
	if previous.State != policy.AttemptStopped {
		return "", 0, policy.ErrConflict
	}
	if generation <= previous.Snapshot.Launch.Generation {
		generation = previous.Snapshot.Launch.Generation + 1
	}
	return previous.ID, generation, nil
}

func sandboxRestartScopeMatches(execution *runtime.AgentExecution, previous *policy.Attempt) bool {
	selection, snapshot := execution.SandboxSelection, previous.Snapshot
	return snapshot.TaskID == selection.TaskID && snapshot.WorkspaceID == selection.WorkspaceID && snapshot.AgentProfileID == selection.AgentProfileID && snapshot.Resolved.ExecutorID == selection.ExecutorID && snapshot.WorkDir == execution.WorkspacePath
}
