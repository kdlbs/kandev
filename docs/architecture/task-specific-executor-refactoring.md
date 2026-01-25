# Task-Specific Executor Selection Architecture Refactoring

> **Status**: Proposal  
> **Author**: Generated for architectural review  
> **Date**: 2026-01-25

## Executive Summary

This document proposes refactoring the executor selection architecture from a global runtime configuration to a per-task executor selection model. The goal is to enable multiple runtimes (Docker and Standalone) to coexist, with each task independently selecting its execution environment.

---

## 1. Current Architecture

### 1.1 Global Runtime Configuration

The backend currently uses a **global runtime configuration** that determines how ALL agents are launched:

```go
// apps/backend/internal/common/config/config.go
type AgentConfig struct {
    Runtime        string // "docker" or "standalone" - GLOBAL for all tasks
    StandaloneHost string
    StandalonePort int
}
```

The `provideLifecycleManager()` function in `apps/backend/cmd/kandev/agents.go` creates a **single runtime** based on this global config:

```go
func provideLifecycleManager(...) (*lifecycle.Manager, *registry.Registry, error) {
    var agentRuntime lifecycle.Runtime
    switch cfg.Agent.Runtime {
    case "standalone":
        agentRuntime = lifecycle.NewStandaloneRuntime(...)  // ALL tasks use this
    default:
        agentRuntime = lifecycle.NewDockerRuntime(...)      // OR ALL tasks use this
    }
    // Only ONE runtime is created and passed to the Manager
    lifecycleMgr := lifecycle.NewManager(..., agentRuntime, ...)
}
```

### 1.2 Why This Is Limiting

1. **All-or-nothing approach**: Every task uses either Docker OR Standalone, never both
2. **No runtime flexibility**: Can't run lightweight tasks on host while running isolated tasks in Docker
3. **Configuration at startup**: Runtime is fixed when the backend starts; changing requires restart
4. **Underutilized Executor model**: The database has `Executor` with `Type` field (`local_pc`, `local_docker`) but it's not used for runtime selection

### 1.3 Current Data Flow

```
Task Creation
    │
    ├── TaskSession.ExecutorID is set (from workspace default)
    │   But it's only used for metadata (MCP policy), not runtime selection
    │
    └── executor.ExecuteWithProfile()
            │
            └── LaunchAgent() → lifecycle.Manager.Launch()
                    │
                    └── m.runtime.CreateInstance()  ← Uses GLOBAL runtime
```

### 1.4 Existing Executor Infrastructure

The database already has:
- `executors` table with `type` field (local_pc, local_docker, remote_docker, etc.)
- `task_sessions.executor_id` foreign key
- `executors_running.runtime` field (currently underutilized)
- Default executors seeded: "Local PC" (exec-local-pc) and "Local Docker" (exec-local-docker)

---

## 2. Proposed Architecture

### 2.1 Core Change: Multi-Runtime Manager

Replace the single-runtime lifecycle manager with a **runtime registry** that can dispatch to multiple runtimes based on executor type:

```go
// NEW: RuntimeRegistry manages multiple runtimes
type RuntimeRegistry struct {
    runtimes map[runtime.Name]Runtime  // docker, standalone, etc.
    logger   *logger.Logger
}

func (r *RuntimeRegistry) GetRuntime(name runtime.Name) (Runtime, error) {
    rt, ok := r.runtimes[name]
    if !ok {
        return nil, fmt.Errorf("runtime %q not available", name)
    }
    return rt, nil
}

// Lifecycle Manager now uses RuntimeRegistry instead of single Runtime
type Manager struct {
    runtimeRegistry *RuntimeRegistry  // Replaces: runtime Runtime
    // ... rest unchanged
}
```

### 2.2 Executor Type → Runtime Mapping

```go
// ExecutorType to Runtime mapping
func ExecutorTypeToRuntime(execType models.ExecutorType) runtime.Name {
    switch execType {
    case models.ExecutorTypeLocalPC:
        return runtime.NameStandalone
    case models.ExecutorTypeLocalDocker:
        return runtime.NameDocker
    case models.ExecutorTypeRemoteDocker:
        return runtime.NameDocker  // Future: separate RemoteDockerRuntime
    default:
        return runtime.NameStandalone  // Fallback
    }
}
```

### 2.3 New Data Flow

```
Task Creation (with executor_id)
    │
    ├── TaskSession.ExecutorID → Lookup Executor → Get ExecutorType
    │
    └── executor.ExecuteWithProfile()
            │
            └── LaunchAgentRequest now includes ExecutorType
                    │
                    └── lifecycle.Manager.Launch()
                            │
                            ├── Resolve runtime: ExecutorTypeToRuntime(req.ExecutorType)
                            │
                            └── runtimeRegistry.GetRuntime(runtimeName).CreateInstance()
```

### 2.4 Configuration Changes

**Remove** the global `cfg.Agent.Runtime` setting. Instead:

```yaml
# BEFORE: Global runtime selection (to be removed)
agent:
  runtime: "standalone"  # DELETE THIS

# AFTER: Runtime availability configuration
agent:
  docker:
    enabled: true
    # Docker-specific config (image, network, etc.)
  standalone:
    host: "localhost"
    port: 9999
    # NOTE: No "enabled" flag - agentctl always runs as a core service
```

**Important**: The `standalone` section configures connection settings for the agentctl process, but there's no `enabled` flag. The agentctl launcher always starts as a core service - it's the control plane for standalone execution and is always available.

---

## 3. Files Requiring Modification

### 3.1 Configuration Layer

| File | Change |
|------|--------|
| `internal/common/config/config.go` | Remove `Runtime` field; add `Docker.Enabled` (standalone is always on) |
| `cmd/kandev/agents.go` | Refactor `provideLifecycleManager()` to create RuntimeRegistry with both runtimes |
| `cmd/kandev/agentctl.go` | **Always start agentctl launcher** (remove conditional check) |

### 3.2 Lifecycle Manager

| File | Change |
|------|--------|
| `internal/agent/lifecycle/manager.go` | Replace `runtime Runtime` with `runtimeRegistry *RuntimeRegistry`; update `Launch()` to select runtime via registry |
| `internal/agent/lifecycle/runtime_registry.go` | **NEW FILE**: RuntimeRegistry implementation |
| `internal/agent/lifecycle/runtime.go` | Add `ExecutorTypeToRuntime()` mapping function |

> **Design Principle**: Keep `manager.go` free of runtime-specific code (no Docker imports, no standalone-specific logic). All runtime-specific behavior should be encapsulated in the `Runtime` interface implementations (`runtime_docker.go`, `runtime_standalone.go`) and dispatched through the `RuntimeRegistry`. The manager should only interact with the abstract `Runtime` interface.

### 3.3 Orchestrator/Executor

| File | Change |
|------|--------|
| `internal/orchestrator/executor/executor.go` | Pass `ExecutorType` in `LaunchAgentRequest`; resolve executor before launch |
| `internal/agent/lifecycle/types.go` | Add `ExecutorType` field to `LaunchRequest` |
| `cmd/kandev/adapters.go` | Pass `ExecutorType` from executor request to lifecycle request |

### 3.4 Data Layer

| File | Change |
|------|--------|
| `internal/task/models/models.go` | Add helper: `Executor.GetRuntimeName()` |
| `internal/agent/runtime/runtime.go` | Add `ExecutorTypeToRuntime()` function |

### 3.5 Recovery/Resumption

| File | Change |
|------|--------|
| `internal/orchestrator/service.go` | `resumeExecutorsOnStartup()` must iterate all available runtimes |
| `internal/agent/lifecycle/manager.go` | `RecoverExecutions()` must query each runtime in registry |

---

## 4. Implementation Plan

### Phase 1: Infrastructure Setup (Non-Breaking)

1. **Create RuntimeRegistry** (`runtime_registry.go`)
   - Implement registry with `Register()`, `GetRuntime()`, `List()`, `HealthCheckAll()`
   - Add `ExecutorTypeToRuntime()` mapping function

2. **Update Configuration**
   - Add `Docker.Enabled` and `Standalone.Enabled` fields
   - Keep `Runtime` field temporarily for backward compatibility
   - Add deprecation warning if `Runtime` is used

3. **Refactor `provideLifecycleManager()`**
   - Create both runtimes if enabled
   - Build RuntimeRegistry with available runtimes
   - Pass registry to Manager (but Manager still uses single runtime internally)

### Phase 2: Lifecycle Manager Refactoring

4. **Update Manager to use RuntimeRegistry**
   - Change `runtime Runtime` to `runtimeRegistry *RuntimeRegistry`
   - Add `ExecutorType` to `LaunchRequest`
   - Update `Launch()` to:
     ```go
     runtimeName := ExecutorTypeToRuntime(req.ExecutorType)
     rt, err := m.runtimeRegistry.GetRuntime(runtimeName)
     if err != nil {
         return nil, fmt.Errorf("runtime not available: %w", err)
     }
     runtimeInstance, err := rt.CreateInstance(ctx, runtimeReq)
     ```

5. **Update RecoverExecutions()**
   - Iterate all registered runtimes
   - Merge recovered instances from each runtime

### Phase 3: Executor Integration

6. **Update `LaunchAgentRequest`**
   - Add `ExecutorType models.ExecutorType` field

7. **Update `executor.ExecuteWithProfile()`**
   - Resolve executor before launch
   - Pass executor type in request

8. **Update `lifecycleAdapter.LaunchAgent()`**
   - Map executor type to lifecycle request

### Phase 4: Cleanup

9. **Remove global `cfg.Agent.Runtime`**
   - Delete from config struct
   - Update flag parsing
   - Update documentation

10. **Update agentctl launcher logic**
    - **Always start** the agentctl launcher (remove all conditional checks)
    - agentctl is a core service, not dependent on runtime configuration

---

## 5. Edge Cases and Challenges

### 5.1 Session Resumption Across Runtimes

**Challenge**: A session started on Docker cannot be resumed on Standalone (different environment).

**Solution**: Store runtime name in `executors_running.runtime` and validate on resume:
```go
func (e *Executor) ResumeSession(...) {
    running, _ := e.repo.GetExecutorRunningBySessionID(session.ID)
    if running != nil && running.Runtime != "" {
        // Use the same runtime that was used originally
        req.ExecutorType = RuntimeToExecutorType(running.Runtime)
    }
}
```

### 5.2 Runtime Availability Changes

**Challenge**: Task was created with Docker executor, but Docker is disabled on restart.

**Solution**:
- Check runtime availability before launch
- Return clear error: "Runtime 'docker' is not available. Enable it in configuration or change the task's executor."
- Frontend can show unavailable executors as disabled

### 5.3 Fallback Behavior

**Challenge**: Executor not set on task/session.

**Solution**: Define precedence:
1. Session's executor (if resuming)
2. Workspace default executor
3. System default ("Local PC" → Standalone)

### 5.4 agentctl Architecture

**Clarification**: agentctl runs in **both** modes:
- **Docker**: agentctl runs inside each container (started by entrypoint)
- **Standalone**: agentctl runs as a subprocess on the host, managing multiple agent instances

**Current**: agentctl launcher only started if `cfg.Agent.Runtime == "standalone"`

**New**: agentctl launcher should **always start** on the host. It serves as the control plane for standalone execution. The launcher startup should not depend on any runtime configuration - it's a core service that's always available.

### 5.5 Container Recovery

**Challenge**: Docker runtime recovers containers on startup; Standalone doesn't persist instances.

**Solution**:
- Docker: `RecoverInstances()` finds containers by label
- Standalone: Returns empty (sessions recreated on resume via `LaunchAgent()`)
- RuntimeRegistry: Iterates all runtimes, merges results

---

## 6. Database Schema Changes

No schema changes required. Existing fields are sufficient:
- `task_sessions.executor_id` - Links to selected executor
- `executors.type` - Determines runtime (local_pc → standalone, local_docker → docker)
- `executors_running.runtime` - Records which runtime was used

---

## 7. API Changes

### 7.1 Task Creation (No Change Needed)

The `executor_id` can already be passed via:
- Workspace default (`workspace.default_executor_id`)
- Potentially extended to task creation payload in future

### 7.2 New: Runtime Status Endpoint

```json
GET /api/v1/runtimes/status
{
  "runtimes": [
    { "name": "standalone", "available": true, "healthy": true },
    { "name": "docker", "available": true, "healthy": true }
  ]
}
```

This allows the frontend to:
- Show which executors are available
- Disable executor selection for unavailable runtimes

---

## 8. Testing Strategy

1. **Unit Tests**
   - `RuntimeRegistry.GetRuntime()` for registered/unregistered runtimes
   - `ExecutorTypeToRuntime()` mapping
   - `Launch()` with different executor types

2. **Integration Tests**
   - Start task with `local_pc` executor → uses Standalone runtime
   - Start task with `local_docker` executor → uses Docker runtime
   - Resume session maintains original runtime
   - Backend restart with Docker disabled → sessions on Docker can't resume

3. **E2E Tests**
   - Create workspace with Docker as default executor
   - Start task → runs in Docker container
   - Change workspace default to Local PC
   - Start new task → runs via Standalone
   - Both tasks run concurrently with different runtimes

---

## 9. Code Examples

### 9.1 RuntimeRegistry Implementation

```go
// internal/agent/lifecycle/runtime_registry.go
package lifecycle

import (
    "context"
    "fmt"
    "sync"

    "github.com/kandev/kandev/internal/agent/runtime"
    "github.com/kandev/kandev/internal/common/logger"
)

type RuntimeRegistry struct {
    runtimes map[runtime.Name]Runtime
    mu       sync.RWMutex
    logger   *logger.Logger
}

func NewRuntimeRegistry(log *logger.Logger) *RuntimeRegistry {
    return &RuntimeRegistry{
        runtimes: make(map[runtime.Name]Runtime),
        logger:   log,
    }
}

func (r *RuntimeRegistry) Register(rt Runtime) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.runtimes[rt.Name()] = rt
    r.logger.Info("registered runtime", zap.String("name", string(rt.Name())))
}

func (r *RuntimeRegistry) GetRuntime(name runtime.Name) (Runtime, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    rt, ok := r.runtimes[name]
    if !ok {
        return nil, fmt.Errorf("runtime %q not registered", name)
    }
    return rt, nil
}

func (r *RuntimeRegistry) List() []runtime.Name {
    r.mu.RLock()
    defer r.mu.RUnlock()
    names := make([]runtime.Name, 0, len(r.runtimes))
    for name := range r.runtimes {
        names = append(names, name)
    }
    return names
}

func (r *RuntimeRegistry) HealthCheckAll(ctx context.Context) map[runtime.Name]error {
    r.mu.RLock()
    defer r.mu.RUnlock()
    results := make(map[runtime.Name]error)
    for name, rt := range r.runtimes {
        results[name] = rt.HealthCheck(ctx)
    }
    return results
}

func (r *RuntimeRegistry) RecoverAll(ctx context.Context) ([]*RuntimeInstance, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    var allRecovered []*RuntimeInstance
    for _, rt := range r.runtimes {
        recovered, err := rt.RecoverInstances(ctx)
        if err != nil {
            r.logger.Warn("failed to recover instances from runtime",
                zap.String("runtime", string(rt.Name())),
                zap.Error(err))
            continue
        }
        allRecovered = append(allRecovered, recovered...)
    }
    return allRecovered, nil
}
```

### 9.2 Updated provideLifecycleManager

```go
// apps/backend/cmd/kandev/agents.go
func provideLifecycleManager(
    ctx context.Context,
    cfg *config.Config,
    log *logger.Logger,
    eventBus bus.EventBus,
    dockerClient *docker.Client,
    agentSettingsRepo settingsstore.Repository,
) (*lifecycle.Manager, *registry.Registry, error) {

    // Create runtime registry
    runtimeRegistry := lifecycle.NewRuntimeRegistry(log)

    // Standalone runtime is always available (agentctl is a core service)
    controlClient := agentctl.NewControlClient(
        cfg.Agent.Standalone.Host,
        cfg.Agent.Standalone.Port,
        log,
    )
    standaloneRuntime := lifecycle.NewStandaloneRuntime(
        controlClient,
        cfg.Agent.Standalone.Host,
        cfg.Agent.Standalone.Port,
        log,
    )
    runtimeRegistry.Register(standaloneRuntime)
    log.Info("Standalone runtime registered")

    // Register Docker runtime if enabled and Docker client is available
    if cfg.Agent.Docker.Enabled && dockerClient != nil {
        dockerRuntime := lifecycle.NewDockerRuntime(dockerClient, log)
        runtimeRegistry.Register(dockerRuntime)
        log.Info("Docker runtime registered")
    }

    // ... rest of initialization using runtimeRegistry
    // NOTE: Manager receives the registry, not individual runtimes
    // This keeps manager.go free of runtime-specific code
    lifecycleMgr := lifecycle.NewManager(
        agentRegistry, eventBus, runtimeRegistry, /* ... */
    )
    return lifecycleMgr, agentRegistry, nil
}
```

### 9.3 Updated Launch Method

```go
// internal/agent/lifecycle/manager.go
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
    // ... existing validation and setup ...

    // Determine which runtime to use based on executor type
    runtimeName := runtime.ExecutorTypeToRuntime(req.ExecutorType)
    if runtimeName == runtime.NameUnknown {
        runtimeName = runtime.NameStandalone // Default fallback
    }

    rt, err := m.runtimeRegistry.GetRuntime(runtimeName)
    if err != nil {
        return nil, fmt.Errorf("runtime %q not available for executor type %q: %w",
            runtimeName, req.ExecutorType, err)
    }

    // Create runtime instance using the selected runtime
    runtimeInstance, err := rt.CreateInstance(ctx, runtimeReq)
    if err != nil {
        return nil, fmt.Errorf("failed to create instance: %w", err)
    }

    // Store runtime name for session resumption
    execution := runtimeInstance.ToAgentExecution(runtimeReq)
    execution.RuntimeName = runtimeName  // NEW FIELD

    // ... rest unchanged ...
}
```

---

## 10. Migration Path

Since breaking changes are acceptable, the migration is straightforward:

1. **Deploy new backend** with both runtimes enabled
2. **Running sessions continue** (Docker containers persist; Standalone sessions will need resume)
3. **New tasks** use executor-specific runtime selection
4. **Remove old config** after verifying stability

No database migration needed - schema already supports this model.