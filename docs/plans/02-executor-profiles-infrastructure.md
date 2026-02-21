# Plan 02: Executor Profiles Infrastructure

> Unified executor abstraction with profiles. Full rename of `runtime` to `executor`.
> Merge `environments` into executor profiles. Foundation for SSH, K8s, Sprites, and more.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Rename Strategy: Runtime → Executor](#rename-strategy-runtime--executor)
4. [Executor & Profile Models](#executor--profile-models)
5. [Backend Implementation](#backend-implementation)
6. [Environment Preparation Abstraction](#environment-preparation-abstraction)
7. [Health Check System](#health-check-system)
8. [API Endpoints](#api-endpoints)
9. [Frontend Implementation](#frontend-implementation)
10. [Tracing](#tracing)
11. [Database Schema (New Service)](#database-schema-new-service)
12. [Backward Compatibility](#backward-compatibility)

---

## Overview

Kandev currently has two separate concepts that overlap:

1. **Executors** — where to run (local, worktree, local_docker, remote_docker)
2. **Environments** — how to configure the runtime (local_pc, docker_image with image tag/dockerfile)

Additionally, the backend uses "Runtime" as the internal interface name while "Executor" is the user-facing concept. This creates confusion.

### Goals

- **Full rename**: `runtime.Name` → `executor.Name`, `Runtime` interface → `Executor` interface, `RuntimeRegistry` → `ExecutorRegistry`, all through the Go codebase
- **Merge environments into executor profiles**: Docker profiles hold image config, local profiles hold worktree root, SSH profiles hold host/key, etc.
- **Add executor types**: `ssh`, `k8s`, `sprites` (in addition to existing `local`, `worktree`, `local_docker`, `remote_docker`)
- **Profile system**: Each executor can have multiple named profiles with type-specific configuration
- **Health check system**: Periodic health checks for remote executors with status on session cards
- **Environment preparation**: Abstraction for setting up executor environments before agent launch

### Non-Goals (This Phase)

- Implementing SSH, K8s, or Sprites executors (separate plans)
- Remote preparer implementations (SSH/Sprites/K8s — only the interface + local/worktree/docker preparers)
- Health check UI beyond session card indicator

---

## Architecture

### Current vs. New Model

```
CURRENT MODEL:
┌────────────────────┐     ┌─────────────────────┐
│     Executor       │     │    Environment       │
│ (where to run)     │     │ (how to configure)   │
│                    │     │                      │
│ - local            │     │ - local_pc           │
│ - worktree         │     │   worktree_root      │
│ - local_docker     │     │ - docker_image       │
│ - remote_docker    │     │   image_tag          │
│                    │     │   dockerfile         │
│ config: map[str]str│     │   build_config       │
└────────────────────┘     └─────────────────────┘
        ↓                          ↓
   chosen per session         chosen per session
   (executor_id)              (environment_id)


NEW MODEL:
┌──────────────────────────────────────────────────────────┐
│                      Executor                             │
│  (unified: where + how to run)                           │
│                                                          │
│  Types: local, worktree, docker, ssh, k8s, sprites       │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │              Executor Profiles                     │  │
│  │                                                    │  │
│  │  Docker Executor:                                  │  │
│  │   ├── "local" profile:                             │  │
│  │   │    host: /var/run/docker.sock                  │  │
│  │   │    image: kandev-agent:latest                  │  │
│  │   │    dockerfile: ./Dockerfile.agent              │  │
│  │   └── "remote-server" profile:                     │  │
│  │        host: tcp://192.168.1.100:2376              │  │
│  │        image: kandev-agent:latest                  │  │
│  │                                                    │  │
│  │  SSH Executor:                                     │  │
│  │   ├── "dev-server" profile:                        │  │
│  │   │    host: dev.example.com                       │  │
│  │   │    user: deploy                                │  │
│  │   │    key_secret_id: <ref to secrets store>       │  │
│  │   └── "staging" profile:                           │  │
│  │        host: staging.example.com                   │  │
│  │        user: deploy                                │  │
│  │                                                    │  │
│  │  Sprites Executor:                                 │  │
│  │   └── "default" profile:                           │  │
│  │        api_token_secret_id: <ref to secrets store> │  │
│  │        network_policies: [...]                     │  │
│  │                                                    │  │
│  │  Local Executor:                                   │  │
│  │   └── "default" profile:                           │  │
│  │        (no additional config)                      │  │
│  │                                                    │  │
│  │  Worktree Executor:                                │  │
│  │   └── "default" profile:                           │  │
│  │        worktree_root: ~/kandev                     │  │
│  │                                                    │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
└──────────────────────────────────────────────────────────┘
        ↓
   chosen per session:
   (executor_id + profile_id)
```

### Execution Flow (Updated)

```
Client (WS)     Orchestrator       Lifecycle Manager     ExecutorImpl       agentctl
    │                │                     │                  │                │
    │ task.start     │                     │                  │                │
    │ executor_id    │                     │                  │                │
    │ profile_id     │                     │                  │                │
    │───────────────>│ LaunchAgent()       │                  │                │
    │                │────────────────────>│                  │                │
    │                │                     │ ResolveExecutor()│                │
    │                │                     │ (load profile    │                │
    │                │                     │  merge config)   │                │
    │                │                     │                  │                │
    │                │                     │ PrepareEnvironment()              │
    │                │                     │────────────────>│                │
    │                │                     │                  │ (install deps, │
    │                │                     │                  │  copy configs) │
    │                │                     │                  │<───────────────│
    │                │                     │                  │                │
    │                │                     │ CreateInstance() │                │
    │                │                     │────────────────>│                │
    │                │                     │                  │ (start env +   │
    │                │                     │                  │  agentctl)     │
    │                │                     │                  │───────────────>│
    │                │                     │                  │                │
    │                │ ConfigureAgent()    │                  │                │
    │                │────────────────────>│─────────────────────────────────>│
    │                │ Start()            │                  │                │
    │                │────────────────────>│─────────────────────────────────>│
    │                │                     │                  │                │
    │<── WS events ──│<── stream updates ──│<─────────────────────────────────│
    │                │                     │                  │                │
    │                │              ┌──────┴──────┐          │                │
    │                │              │ Health Loop │          │                │
    │                │              │ (remote     │          │                │
    │                │              │  executors) │          │                │
    │                │              └─────────────┘          │                │
```

### Session Resume Flow (Remote Executors)

```
Backend Restart
    │
    ▼
┌─────────────────────────────────┐
│ RecoverInstances()              │
│ for each registered executor    │
│                                 │
│  Local/Worktree:                │
│    Check PID still alive        │
│    Reconnect agentctl           │
│                                 │
│  Docker:                        │
│    List kandev.managed          │
│    containers, reconnect        │
│                                 │
│  Remote (SSH/Sprites/K8s):      │
│    Load executors_running       │
│    rows from DB                 │
│    HealthCheck each             │
│    ├── healthy → reconnect      │
│    └── unhealthy → mark dead,   │
│        emit session.error       │
└─────────────────────────────────┘
```

---

## Rename Strategy: Runtime → Executor

### Scope of Rename

The rename affects Go packages, interfaces, types, and variables. Frontend already uses "executor" terminology.

```
GO PACKAGE RENAME:
  internal/agent/runtime/runtime.go
    → internal/agent/executor/executor.go

  runtime.Name → executor.Name
  runtime.NameDocker → executor.NameDocker
  runtime.NameStandalone → executor.NameStandalone
  ...

INTERFACE RENAME (in lifecycle package):
  Runtime → ExecutorBackend
  RuntimeRegistry → ExecutorRegistry
  RuntimeCreateRequest → ExecutorCreateRequest
  RuntimeInstance → ExecutorInstance
  RuntimeFallbackPolicy → ExecutorFallbackPolicy

  NewRuntimeRegistry → NewExecutorRegistry
  NewDockerRuntime → NewDockerExecutor
  NewStandaloneRuntime → NewStandaloneExecutor
  NewRemoteDockerRuntime → NewRemoteDockerExecutor

FILE RENAMES:
  lifecycle/runtime.go → lifecycle/executor_backend.go
  lifecycle/runtime_registry.go → lifecycle/executor_registry.go
  lifecycle/runtime_registry_test.go → lifecycle/executor_registry_test.go
  lifecycle/runtime_docker.go → lifecycle/executor_docker.go
  lifecycle/runtime_standalone.go → lifecycle/executor_standalone.go
  lifecycle/runtime_remote_docker.go → lifecycle/executor_remote_docker.go

DB COLUMN (executors_running table):
  "runtime" column → keep as "runtime" for DB compat, but add alias
  OR rename to "executor_backend" with migration

LOGGING:
  zap.String("runtime", ...) → zap.String("executor", ...)
```

### Rename Procedure

1. **Create new package**: `internal/agent/executor/` with updated names
2. **Update lifecycle package**: Rename interfaces and types
3. **Rename implementation files**: Docker, Standalone, RemoteDocker
4. **Update all imports**: `cmd/kandev/`, orchestrator, handlers
5. **DB migration**: Add `executor_backend` column, copy from `runtime`, drop `runtime` (or keep both during transition)
6. **Update tests**
7. **Delete old `internal/agent/runtime/` package**

### Detailed File Changes

```
DELETED:
  internal/agent/runtime/runtime.go

CREATED:
  internal/agent/executor/executor.go

RENAMED:
  lifecycle/runtime.go              → lifecycle/executor_backend.go
  lifecycle/runtime_registry.go     → lifecycle/executor_registry.go
  lifecycle/runtime_registry_test.go → lifecycle/executor_registry_test.go
  lifecycle/runtime_docker.go       → lifecycle/executor_docker.go
  lifecycle/runtime_standalone.go   → lifecycle/executor_standalone.go
  lifecycle/runtime_remote_docker.go → lifecycle/executor_remote_docker.go
  lifecycle/manager_runtime.go      → lifecycle/manager_executor.go

UPDATED (imports + references):
  cmd/kandev/agents.go
  cmd/kandev/main.go
  internal/agent/lifecycle/manager.go
  internal/agent/lifecycle/execution_store.go
  internal/agent/lifecycle/session.go
  internal/agent/lifecycle/streams.go
  internal/agent/lifecycle/events.go
  internal/agent/lifecycle/container.go
  internal/agent/lifecycle/process_runner.go
  internal/orchestrator/executor/executor.go
  internal/orchestrator/handlers/*.go
  internal/task/models/models.go
  internal/task/repository/sqlite/executor.go
```

---

## Executor & Profile Models

### Updated Go Models: `task/models/models.go`

```go
// ExecutorType represents the executor type.
type ExecutorType string

const (
    ExecutorTypeLocal        ExecutorType = "local"
    ExecutorTypeWorktree     ExecutorType = "worktree"
    ExecutorTypeDocker       ExecutorType = "docker"        // was "local_docker"
    ExecutorTypeSSH          ExecutorType = "ssh"            // new
    ExecutorTypeK8s          ExecutorType = "k8s"            // new
    ExecutorTypeSprites      ExecutorType = "sprites"        // new
    // Deprecated aliases for backward compat during migration
    ExecutorTypeLocalDocker  ExecutorType = "local_docker"   // → docker
    ExecutorTypeRemoteDocker ExecutorType = "remote_docker"  // → docker + remote profile
)

// Executor represents an execution target with profiles.
type Executor struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Type        ExecutorType      `json:"type"`
    Status      ExecutorStatus    `json:"status"`
    IsSystem    bool              `json:"is_system"`
    Resumable   bool              `json:"resumable"`
    Config      map[string]string `json:"config,omitempty"`    // executor-level defaults
    Profiles    []*ExecutorProfile `json:"profiles,omitempty"` // loaded via join
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    DeletedAt   *time.Time        `json:"deleted_at,omitempty"`
}

// ExecutorProfile represents a named configuration variant for an executor.
type ExecutorProfile struct {
    ID          string                 `json:"id"`
    ExecutorID  string                 `json:"executor_id"`
    Name        string                 `json:"name"`
    IsDefault   bool                   `json:"is_default"`
    Config      map[string]interface{} `json:"config"`  // type-specific config (see below)
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
    DeletedAt   *time.Time             `json:"deleted_at,omitempty"`
}

// ExecutorRunning tracks an active executor instance for a session.
type ExecutorRunning struct {
    ID               string     `json:"id"`
    SessionID        string     `json:"session_id"`
    TaskID           string     `json:"task_id"`
    ExecutorID       string     `json:"executor_id"`
    ProfileID        string     `json:"profile_id,omitempty"`
    ExecutorBackend  string     `json:"executor_backend,omitempty"` // was "runtime"
    Status           string     `json:"status"`
    Resumable        bool       `json:"resumable"`
    ResumeToken      string     `json:"resume_token,omitempty"`
    AgentExecutionID string     `json:"agent_execution_id,omitempty"`
    ContainerID      string     `json:"container_id,omitempty"`
    RemoteID         string     `json:"remote_id,omitempty"`         // NEW: sprite name, SSH session, pod name
    AgentctlURL      string     `json:"agentctl_url,omitempty"`
    AgentctlPort     int        `json:"agentctl_port,omitempty"`
    PID              int        `json:"pid,omitempty"`
    WorktreeID       string     `json:"worktree_id,omitempty"`
    WorktreePath     string     `json:"worktree_path,omitempty"`
    WorktreeBranch   string     `json:"worktree_branch,omitempty"`
    HealthStatus     string     `json:"health_status,omitempty"`     // NEW: "healthy", "unhealthy", "unknown"
    HealthCheckedAt  *time.Time `json:"health_checked_at,omitempty"` // NEW
    ErrorMessage     string     `json:"error_message,omitempty"`
    LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
    CreatedAt        time.Time  `json:"created_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
}
```

### Profile Config Schemas (by executor type)

Each executor type has a specific set of config keys stored in the profile's `config` JSON:

```go
// DockerProfileConfig — stored in executor_profiles.config for type="docker"
// {
//   "host": "/var/run/docker.sock" | "tcp://remote:2376",
//   "image_tag": "kandev-agent:latest",
//   "dockerfile": "FROM ubuntu:22.04\nRUN ...",
//   "build_config": {"build_arg": "value"},
//   "network": "bridge",
//   "resource_limits": {"memory": "4g", "cpus": "2"}
// }

// SSHProfileConfig — stored in executor_profiles.config for type="ssh"
// {
//   "host": "dev.example.com",
//   "port": "22",
//   "user": "deploy",
//   "key_secret_id": "secret-uuid-ref",   // references secrets store
//   "key_path": "~/.ssh/id_ed25519",       // OR direct path
//   "install_deps": true,
//   "workspace_path": "/home/deploy/kandev"
// }

// K8sProfileConfig — stored in executor_profiles.config for type="k8s"
// {
//   "context": "my-cluster",
//   "namespace": "kandev",
//   "image": "kandev-agent:latest",
//   "service_account": "kandev-agent",
//   "resource_limits": {"memory": "4Gi", "cpu": "2"},
//   "storage_class": "standard",
//   "kubeconfig_secret_id": "secret-uuid-ref"
// }

// SpritesProfileConfig — stored in executor_profiles.config for type="sprites"
// {
//   "api_token_secret_id": "secret-uuid-ref",
//   "base_image": "ubuntu:22.04",
//   "network_policies": ["*.npm.org", "*.github.com"],
//   "copy_gh_auth": true,
//   "copy_git_config": true,
//   "copy_agent_config": true
// }

// LocalProfileConfig — stored in executor_profiles.config for type="local"
// {
//   (empty — no additional config needed)
// }

// WorktreeProfileConfig — stored in executor_profiles.config for type="worktree"
// {
//   "worktree_root": "~/kandev"
// }
```

---

## Backend Implementation

### New/Renamed Package: `internal/agent/executor/`

```go
// Package executor defines the executor backend types shared across lifecycle and policy logic.
package executor

import "github.com/kandev/kandev/internal/task/models"

// Name identifies the execution backend.
type Name string

const (
    NameUnknown      Name = ""
    NameDocker       Name = "docker"
    NameStandalone   Name = "standalone"
    NameLocal        Name = "local"
    NameRemoteDocker Name = "remote_docker"  // deprecated, use NameDocker + remote profile
    NameSSH          Name = "ssh"
    NameK8s          Name = "k8s"
    NameSprites      Name = "sprites"
)

// ExecutorTypeToBackend maps an ExecutorType to its corresponding backend Name.
func ExecutorTypeToBackend(execType models.ExecutorType) Name {
    switch execType {
    case models.ExecutorTypeLocal:
        return NameStandalone
    case models.ExecutorTypeWorktree:
        return NameStandalone
    case models.ExecutorTypeDocker, models.ExecutorTypeLocalDocker:
        return NameDocker
    case models.ExecutorTypeRemoteDocker:
        return NameRemoteDocker
    case models.ExecutorTypeSSH:
        return NameSSH
    case models.ExecutorTypeK8s:
        return NameK8s
    case models.ExecutorTypeSprites:
        return NameSprites
    default:
        return NameStandalone
    }
}
```

### Renamed Interface: `lifecycle/executor_backend.go`

```go
// ExecutorBackend abstracts the agent execution environment (Docker, Standalone, K8s, SSH, etc.)
// Each backend is responsible for creating and managing agentctl instances.
type ExecutorBackend interface {
    // Name returns the backend identifier (e.g., "docker", "standalone", "k8s")
    Name() executor.Name

    // HealthCheck verifies the backend is available and operational
    HealthCheck(ctx context.Context) error

    // CreateInstance creates a new agentctl instance for a task.
    CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error)

    // StopInstance stops an agentctl instance.
    StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error

    // RecoverInstances discovers and recovers instances that were running before a restart.
    RecoverInstances(ctx context.Context) ([]*ExecutorInstance, error)

    // GetInteractiveRunner returns the interactive runner for passthrough mode.
    GetInteractiveRunner() *process.InteractiveRunner

    // IsRemote returns true if this backend manages remote environments.
    // Remote backends get periodic health checks and special recovery logic.
    IsRemote() bool
}

// ExecutorCreateRequest contains parameters for creating an agentctl instance.
type ExecutorCreateRequest struct {
    InstanceID     string
    TaskID         string
    SessionID      string
    AgentProfileID string
    ExecutorID     string                 // NEW: executor DB ID
    ProfileID      string                 // NEW: profile DB ID
    ProfileConfig  map[string]interface{} // NEW: merged profile config
    WorkspacePath  string
    Protocol       string
    Env            map[string]string
    Metadata       map[string]interface{}
    McpServers     []McpServerConfig
    AgentConfig    agents.Agent
}

// ExecutorInstance represents an agentctl instance created by a backend.
type ExecutorInstance struct {
    // Core identifiers
    InstanceID string
    TaskID     string
    SessionID  string

    // Backend name (e.g., "docker", "standalone") - set by the backend that created this instance
    BackendName string

    // Agentctl client for communicating with this instance
    Client *agentctl.Client

    // Backend-specific identifiers (only one set is populated)
    ContainerID          string // Docker
    ContainerIP          string // Docker
    StandaloneInstanceID string // Standalone
    StandalonePort       int    // Standalone
    RemoteID             string // NEW: Sprites sprite name, SSH session, K8s pod name

    // Common fields
    WorkspacePath string
    Metadata      map[string]interface{}
}
```

### Executor Registry: `lifecycle/executor_registry.go`

```go
// ExecutorRegistry manages multiple ExecutorBackend implementations.
type ExecutorRegistry struct {
    backends map[executor.Name]ExecutorBackend
    mu       sync.RWMutex
    logger   *logger.Logger
}

func NewExecutorRegistry(log *logger.Logger) *ExecutorRegistry { ... }
func (r *ExecutorRegistry) Register(backend ExecutorBackend) { ... }
func (r *ExecutorRegistry) GetBackend(name executor.Name) (ExecutorBackend, error) { ... }
func (r *ExecutorRegistry) List() []executor.Name { ... }
func (r *ExecutorRegistry) HealthCheckAll(ctx context.Context) map[executor.Name]error { ... }
func (r *ExecutorRegistry) RecoverAll(ctx context.Context) ([]*ExecutorInstance, error) { ... }
```

### Profile Resolution: `lifecycle/profile_resolver.go` additions

```go
// ResolveExecutorProfile loads the executor and profile, merging configs.
// Returns the merged config that includes executor-level defaults + profile overrides.
func (r *StoreProfileResolver) ResolveExecutorProfile(
    ctx context.Context,
    executorID string,
    profileID string,
) (*models.Executor, *models.ExecutorProfile, map[string]interface{}, error) {
    // 1. Load executor from DB
    // 2. Load profile from DB (or default profile if profileID empty)
    // 3. Merge: executor.Config (base) + profile.Config (override)
    // 4. Return merged config
}
```

### Database Changes

#### New Table: `executor_profiles`

```sql
CREATE TABLE IF NOT EXISTS executor_profiles (
    id           TEXT PRIMARY KEY,
    executor_id  TEXT NOT NULL REFERENCES executors(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    is_default   INTEGER NOT NULL DEFAULT 0,
    config       TEXT NOT NULL DEFAULT '{}',   -- JSON: type-specific config
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at   TIMESTAMP DEFAULT NULL,
    UNIQUE(executor_id, name)
);

CREATE INDEX IF NOT EXISTS idx_executor_profiles_executor_id
    ON executor_profiles(executor_id);
```

#### Column Changes: `executors_running`

```sql
-- Add new columns
ALTER TABLE executors_running ADD COLUMN profile_id TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN executor_backend TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN remote_id TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN health_status TEXT DEFAULT 'unknown';
ALTER TABLE executors_running ADD COLUMN health_checked_at TIMESTAMP DEFAULT NULL;

-- Migrate runtime → executor_backend
UPDATE executors_running SET executor_backend = runtime WHERE executor_backend = '';
```

#### Executor Type Migration

```sql
-- Migrate old executor types
UPDATE executors SET type = 'docker' WHERE type = 'local_docker';
-- remote_docker executors get a "remote" profile instead
```

### Profile CRUD Repository: `repository/sqlite/executor_profile.go`

```go
func (r *Repository) CreateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
func (r *Repository) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
func (r *Repository) UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
func (r *Repository) DeleteExecutorProfile(ctx context.Context, id string) error
func (r *Repository) ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error)
func (r *Repository) GetDefaultExecutorProfile(ctx context.Context, executorID string) (*models.ExecutorProfile, error)
```

### Repository Interface Addition: `repository/interface.go`

```go
// Add to Repository interface:
CreateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error
DeleteExecutorProfile(ctx context.Context, id string) error
ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error)
GetDefaultExecutorProfile(ctx context.Context, executorID string) (*models.ExecutorProfile, error)
```

### Default Executors & Profiles: `repository/sqlite/defaults.go`

```go
func ensureDefaultExecutorsAndProfiles(ctx context.Context, db *sqlx.DB) error {
    // exec-local (Local) + "default" profile (empty config)
    // exec-worktree (Worktree) + "default" profile (worktree_root: ~/kandev)
    // exec-docker (Docker) + "local" profile (host: /var/run/docker.sock)
    //
    // If old exec-local-docker exists, migrate to exec-docker
    // If old environments exist, migrate config into profiles
}
```

---

## Environment Preparation Abstraction

The environment preparation layer runs **before** agent process launch and is responsible for making the workspace ready. For local executors this means git pull + worktree creation; for Docker it means image builds; for remote executors it means installing dependencies and copying configs.

Each preparation step emits real-time progress events to the frontend so the user can see exactly what's happening (commands being run, stdout/stderr, exit codes).

### Interface: `lifecycle/env_preparer.go`

```go
// EnvironmentPreparer handles setting up an executor environment
// before agent process launch. This includes git operations, workspace
// setup, dependency installation, and configuration.
type EnvironmentPreparer interface {
    // Prepare sets up the environment for agent execution.
    // Called before CreateInstance for local/worktree (workspace must exist first),
    // or after CreateInstance for Docker/remote (container must exist to run commands in).
    // Emits PrepareProgress events via the provided callback.
    Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error)

    // Cleanup tears down any environment-specific resources.
    Cleanup(ctx context.Context, instanceID string) error
}

// EnvPrepareRequest contains the info needed to prepare an environment.
type EnvPrepareRequest struct {
    TaskID         string
    SessionID      string
    ExecutionID    string                 // For event correlation

    // Workspace
    RepositoryPath string                 // Main repo path on host (local/worktree)
    RepositoryURL  string                 // HTTPS clone URL (remote executors — no local filesystem)
    RepositoryID   string
    BaseBranch     string
    WorkspacePath  string                 // Resolved workspace path (may be overridden by result)

    // Worktree config (local/worktree executors)
    UseWorktree          bool
    TaskTitle            string
    WorktreeBranchPrefix string
    PullBeforeWorktree   bool
    WorktreeID           string           // For session resumption

    // Instance (set for Docker/remote — nil for local/worktree pre-instance prep)
    Instance       *ExecutorInstance

    // Agent & profile
    AgentConfig    agents.Agent
    ProfileConfig  map[string]interface{}

    // Remote preparation flags (from profile config)
    CopyGHAuth      bool
    CopyGitConfig   bool
    CopyAgentConfig bool
    InstallDeps     []string

    // Progress callback — preparer calls this for each step
    OnProgress func(event *PrepareProgressEvent)
}

// EnvPrepareResult contains the outcome of environment preparation.
type EnvPrepareResult struct {
    WorkspacePath  string    // Final workspace path (may differ if worktree created)
    WorktreeID     string    // Worktree ID if created
    WorktreePath   string    // Worktree filesystem path
    WorktreeBranch string    // Worktree branch name
}

// PrepareProgressEvent represents a single preparation step update.
// Streamed to the frontend in real-time.
type PrepareProgressEvent struct {
    TaskID      string `json:"task_id"`
    SessionID   string `json:"session_id"`
    ExecutionID string `json:"execution_id"`

    // Phase & step identification
    Phase       string `json:"phase"`         // "deps" or "repo"
    Step        string `json:"step"`          // e.g., "git_fetch", "git_pull", "git_clone", "worktree_create", "setup_script", "install_agentctl", "copy_gh_auth"
    StepIndex   int    `json:"step_index"`    // 0-based index within the phase
    TotalSteps  int    `json:"total_steps"`   // Total expected steps in this phase

    // Status
    Status      string `json:"status"`        // "running", "completed", "failed", "skipped"

    // Command execution details (shown to user)
    Command     string `json:"command,omitempty"`      // e.g., "git fetch origin main"
    Stdout      string `json:"stdout,omitempty"`       // Captured stdout
    Stderr      string `json:"stderr,omitempty"`       // Captured stderr
    ExitCode    *int   `json:"exit_code,omitempty"`    // Process exit code (nil while running)

    // Human-readable
    Message     string `json:"message,omitempty"`      // e.g., "Fetching latest changes from origin..."
    Error       string `json:"error,omitempty"`        // Error message if failed

    Timestamp   string `json:"timestamp"`
}
```

### Preparer Registry

```go
// PreparerRegistry maps executor types to their preparers.
type PreparerRegistry struct {
    preparers map[executor.Name]EnvironmentPreparer
}

func (r *PreparerRegistry) Register(name executor.Name, preparer EnvironmentPreparer)
func (r *PreparerRegistry) Get(name executor.Name) (EnvironmentPreparer, bool)
```

### Preparation Phases

Every preparer runs two phases in order. Either phase may be a no-op depending on executor type.

```
Phase 1: DEPS  — install tools, copy configs & credentials
Phase 2: REPO  — git clone/pull, worktree creation, repository setup script
```

Why this order: credentials and tools (e.g., git config, gh auth, SSH keys) must be available before git clone/pull can succeed on private repos.

### Built-in Preparers

```
┌────────────────────────────────────────────────────────────────────────────┐
│                          EnvironmentPreparer                               │
│                            (interface)                                     │
├─────────────────────┬────────────────────┬────────────────────────────────┤
│ LocalPreparer       │ DockerPreparer     │ RemotePreparer                 │
│ (local executor)    │ (local docker)     │ (SSH, Sprites, K8s)            │
│                     │                    │                                │
│ DEPS: no-op         │ DEPS: build image  │ DEPS: install agentctl,       │
│ REPO: git pull      │       if needed    │       install agent CLI,      │
│       + setup script│ REPO: git pull     │       copy gh auth/git config,│
│                     │       (inside      │       copy agent config       │
│ WorktreePreparer    │        container,  │ REPO: git clone <url>,        │
│ (worktree executor) │        repo is     │       checkout branch,        │
│                     │        mounted)    │       run setup script        │
│ DEPS: no-op         │                    │                                │
│ REPO: git pull +    │                    │                                │
│       worktree add +│                    │                                │
│       setup script  │                    │                                │
└─────────────────────┴────────────────────┴────────────────────────────────┘
```

```go
// LocalPreparer — for local executor: pull repo + run setup script
type LocalPreparer struct {
    repoProvider worktree.RepositoryProvider
    logger       *logger.Logger
}
func (p *LocalPreparer) Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error) {
    result := &EnvPrepareResult{WorkspacePath: req.RepositoryPath}

    // === Phase 1: DEPS — no-op for local (tools already on host) ===

    // === Phase 2: REPO — git pull + setup script ===

    if !req.PullBeforeWorktree {
        return result, nil
    }

    // Step 1: git fetch origin <baseBranch>
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_fetch", StepIndex: 0, TotalSteps: 3,
        Status: "running", Command: fmt.Sprintf("git fetch origin %s", req.BaseBranch),
        Message: "Fetching latest changes...",
    })
    stdout, stderr, exitCode, err := runGitCommand(ctx, req.RepositoryPath,
        "git", "fetch", "origin", req.BaseBranch)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_fetch", StepIndex: 0, TotalSteps: 3,
        Status: statusFromExit(exitCode, err),
        Command: fmt.Sprintf("git fetch origin %s", req.BaseBranch),
        Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
    })

    // Step 2: git pull --ff-only (if on the base branch)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_pull", StepIndex: 1, TotalSteps: 3,
        Status: "running", Command: fmt.Sprintf("git pull --ff-only origin %s", req.BaseBranch),
        Message: "Pulling latest changes...",
    })
    stdout, stderr, exitCode, err = runGitCommand(ctx, req.RepositoryPath,
        "git", "pull", "--ff-only", "origin", req.BaseBranch)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_pull", StepIndex: 1, TotalSteps: 3,
        Status: statusFromExit(exitCode, err),
        Command: fmt.Sprintf("git pull --ff-only origin %s", req.BaseBranch),
        Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
    })
    // Pull failures are non-fatal — continue with current HEAD

    // Step 3: Run repository setup script (if exists)
    p.runSetupScript(ctx, req, result, 2, 3)

    return result, nil
}

// WorktreePreparer — for worktree executor: pull + worktree create + setup script
type WorktreePreparer struct {
    worktreeMgr  *worktree.Manager
    repoProvider worktree.RepositoryProvider
    logger       *logger.Logger
}
func (p *WorktreePreparer) Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error) {
    totalSteps := 3 // fetch + worktree create + setup script
    if req.PullBeforeWorktree {
        totalSteps = 4 // fetch + pull + worktree create + setup script
    }
    stepIdx := 0

    // === Phase 1: DEPS — no-op for local worktree (tools already on host) ===

    // === Phase 2: REPO — git pull + worktree + setup script ===

    // Step 1: git fetch
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_fetch", StepIndex: stepIdx, TotalSteps: totalSteps,
        Status: "running", Command: fmt.Sprintf("git fetch origin %s", req.BaseBranch),
        Message: "Fetching latest changes...",
    })
    // ... run git fetch, emit completion event ...
    stepIdx++

    // Step 2: git pull --ff-only (if configured)
    if req.PullBeforeWorktree {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "git_pull", StepIndex: stepIdx, TotalSteps: totalSteps,
            Status: "running", Message: "Pulling latest changes...",
        })
        // ... run git pull, emit completion event (non-fatal) ...
        stepIdx++
    }

    // Step 3: Create git worktree
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "worktree_create", StepIndex: stepIdx, TotalSteps: totalSteps,
        Status: "running",
        Command: fmt.Sprintf("git worktree add -b <branch> <path> %s", req.BaseBranch),
        Message: "Creating isolated worktree...",
    })
    wt, err := p.worktreeMgr.Create(ctx, &worktree.CreateRequest{
        TaskID:               req.TaskID,
        SessionID:            req.SessionID,
        TaskTitle:            req.TaskTitle,
        RepositoryID:         req.RepositoryID,
        RepositoryPath:       req.RepositoryPath,
        BaseBranch:           req.BaseBranch,
        WorktreeBranchPrefix: req.WorktreeBranchPrefix,
        PullBeforeWorktree:   false, // Already pulled above
        WorktreeID:           req.WorktreeID,
    })
    if err != nil {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "worktree_create", StepIndex: stepIdx, TotalSteps: totalSteps,
            Status: "failed", Error: err.Error(),
        })
        return nil, fmt.Errorf("create worktree: %w", err)
    }
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "worktree_create", StepIndex: stepIdx, TotalSteps: totalSteps,
        Status: "completed",
        Message: fmt.Sprintf("Created worktree at %s (branch: %s)", wt.Path, wt.Branch),
    })
    stepIdx++

    // Step 4: Run repository setup script (if exists)
    result := &EnvPrepareResult{
        WorkspacePath:  wt.Path,
        WorktreeID:     wt.ID,
        WorktreePath:   wt.Path,
        WorktreeBranch: wt.Branch,
    }
    repo, err := p.repoProvider.GetRepository(ctx, req.RepositoryID)
    if err == nil && repo != nil && strings.TrimSpace(repo.SetupScript) != "" {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "setup_script", StepIndex: stepIdx, TotalSteps: totalSteps,
            Status: "running",
            Command: repo.SetupScript,
            Message: "Running repository setup script...",
        })
        stdout, stderr, exitCode, err := runScript(ctx, wt.Path, repo.SetupScript)
        if err != nil {
            req.OnProgress(&PrepareProgressEvent{
                Phase: "repo", Step: "setup_script", StepIndex: stepIdx, TotalSteps: totalSteps,
                Status: "failed",
                Command: repo.SetupScript,
                Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
                Error: err.Error(),
            })
            // Cleanup worktree on setup script failure (mirrors existing behavior)
            p.worktreeMgr.Delete(ctx, wt.ID)
            return nil, fmt.Errorf("setup script failed: %w", err)
        }
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "setup_script", StepIndex: stepIdx, TotalSteps: totalSteps,
            Status: "completed",
            Command: repo.SetupScript,
            Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
            Message: "Setup script completed",
        })
    } else {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "setup_script", StepIndex: stepIdx, TotalSteps: totalSteps,
            Status: "skipped", Message: "No setup script configured",
        })
    }

    return result, nil
}

// DockerPreparer — builds Docker image if Dockerfile provided in profile
type DockerPreparer struct {
    docker       *docker.Client
    repoProvider worktree.RepositoryProvider
    logger       *logger.Logger
}
func (p *DockerPreparer) Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error) {
    result := &EnvPrepareResult{WorkspacePath: req.WorkspacePath}

    // === Phase 1: DEPS — Docker image build ===
    // 1. Check if image exists → emit progress
    // 2. If Dockerfile in profile config, build image → emit progress with build output
    // 3. Tag with profile-specific tag → emit progress

    // === Phase 2: REPO — repo is bind-mounted from host, pull inside container ===
    // Repo is already on the host filesystem and mounted into the container.
    // If pull_before requested, run git pull inside the container via agentctl shell.
    // Run setup script inside container via agentctl shell if configured.

    return result, nil
}

// RemotePreparer — base for SSH/Sprites/K8s (run commands on remote instance)
type RemotePreparer struct {
    repoProvider worktree.RepositoryProvider
    logger       *logger.Logger
}
func (p *RemotePreparer) Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error) {
    result := &EnvPrepareResult{WorkspacePath: req.WorkspacePath}

    // === Phase 1: DEPS — install tools, copy configs & credentials ===
    // All steps run commands on the remote instance via agentctl shell.
    // Credentials must be set up BEFORE git clone (private repos need auth).

    // Step 1: Install agentctl (if not present)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "deps", Step: "install_agentctl", StepIndex: 0, TotalSteps: depsTotal,
        Status: "running", Message: "Checking agentctl on remote host...",
    })
    // ... run via instance shell, emit completion ...

    // Step 2: Install agent CLI
    req.OnProgress(&PrepareProgressEvent{
        Phase: "deps", Step: "install_agent_cli", StepIndex: 1, TotalSteps: depsTotal,
        Status: "running",
        Command: "npm install -g @anthropics/claude-code",
        Message: "Installing agent CLI...",
    })
    // ... run via instance shell, emit completion ...

    // Step 3: Copy git config (if configured)
    if req.CopyGitConfig {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "deps", Step: "copy_git_config", Status: "running",
            Message: "Copying git configuration...",
        })
        // ... scp or write ~/.gitconfig on remote, emit completion ...
    }

    // Step 4: Copy GH auth token (if configured)
    if req.CopyGHAuth {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "deps", Step: "copy_gh_auth", Status: "running",
            Message: "Copying GitHub CLI authentication...",
        })
        // ... scp or write gh auth config on remote, emit completion ...
    }

    // Step 5: Copy agent config (if configured)
    if req.CopyAgentConfig {
        // ... copy agent-specific config files ...
    }

    // Step 6: Install additional dependencies (if configured)
    for _, dep := range req.InstallDeps {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "deps", Step: "install_dep", Status: "running",
            Command: fmt.Sprintf("apt-get install -y %s", dep),
            Message: fmt.Sprintf("Installing %s...", dep),
        })
        // ... run on remote, emit completion ...
    }

    // === Phase 2: REPO — git clone + checkout + setup script ===
    // Remote host has no local filesystem access — must clone the repo.

    // Step 1: git clone
    workspacePath := getRemoteWorkspacePath(req.ProfileConfig)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_clone", Status: "running",
        Command: fmt.Sprintf("git clone %s %s", req.RepositoryURL, workspacePath),
        Message: "Cloning repository on remote host...",
    })
    stdout, stderr, exitCode, err := runRemoteCommand(ctx, req.Instance,
        "git", "clone", req.RepositoryURL, workspacePath)
    req.OnProgress(&PrepareProgressEvent{
        Phase: "repo", Step: "git_clone",
        Status: statusFromExit(exitCode, err),
        Command: fmt.Sprintf("git clone %s %s", req.RepositoryURL, workspacePath),
        Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
    })
    if err != nil {
        return nil, fmt.Errorf("git clone failed: %w", err)
    }
    result.WorkspacePath = workspacePath

    // Step 2: git checkout <branch> (if specific branch requested)
    if req.BaseBranch != "" {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "git_checkout", Status: "running",
            Command: fmt.Sprintf("git checkout %s", req.BaseBranch),
            Message: fmt.Sprintf("Checking out branch %s...", req.BaseBranch),
        })
        stdout, stderr, exitCode, err = runRemoteCommand(ctx, req.Instance,
            "git", "-C", workspacePath, "checkout", req.BaseBranch)
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "git_checkout",
            Status: statusFromExit(exitCode, err),
            Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
        })
        // Checkout failure is non-fatal (may already be on the branch)
    }

    // Step 3: Run repository setup script (if exists)
    repo, err := p.repoProvider.GetRepository(ctx, req.RepositoryID)
    if err == nil && repo != nil && strings.TrimSpace(repo.SetupScript) != "" {
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "setup_script", Status: "running",
            Command: repo.SetupScript,
            Message: "Running repository setup script...",
        })
        stdout, stderr, exitCode, err = runRemoteCommand(ctx, req.Instance,
            "sh", "-c", fmt.Sprintf("cd %s && %s", workspacePath, repo.SetupScript))
        status := statusFromExit(exitCode, err)
        req.OnProgress(&PrepareProgressEvent{
            Phase: "repo", Step: "setup_script", Status: status,
            Command: repo.SetupScript,
            Stdout: stdout, Stderr: stderr, ExitCode: &exitCode,
        })
        if err != nil {
            return nil, fmt.Errorf("setup script failed on remote: %w", err)
        }
    }

    return result, nil
}
```

### Integration Point

```go
// In lifecycle manager LaunchAgent():
func (m *Manager) LaunchAgent(ctx context.Context, req LaunchRequest) error {
    // 1. Resolve executor + profile
    exec, profile, mergedConfig := m.profileResolver.ResolveExecutorProfile(...)

    // 2. Get executor backend
    backendName := executor.ExecutorTypeToBackend(exec.Type)
    backend, err := m.executorRegistry.GetBackend(backendName)

    // 3. Build progress callback that publishes events to frontend
    onProgress := func(event *PrepareProgressEvent) {
        event.TaskID = req.TaskID
        event.SessionID = req.SessionID
        event.ExecutionID = req.ExecutionID
        event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
        m.eventPublisher.PublishPrepareProgress(event)
    }

    // 4. Prepare environment (NEW — runs BEFORE CreateInstance for local/worktree)
    preparer, _ := m.preparerRegistry.Get(backendName)
    if preparer != nil {
        m.eventPublisher.PublishPrepareStarted(req.TaskID, req.SessionID)

        result, err := preparer.Prepare(ctx, &EnvPrepareRequest{
            TaskID:               req.TaskID,
            SessionID:            req.SessionID,
            RepositoryPath:       req.RepositoryPath,
            RepositoryID:         req.RepositoryID,
            BaseBranch:           req.BaseBranch,
            UseWorktree:          req.UseWorktree,
            TaskTitle:            req.TaskTitle,
            WorktreeBranchPrefix: req.WorktreeBranchPrefix,
            PullBeforeWorktree:   req.PullBeforeWorktree,
            WorktreeID:           req.WorktreeID,
            ProfileConfig:        mergedConfig,
            OnProgress:           onProgress,
        })
        if err != nil {
            m.eventPublisher.PublishPrepareFailed(req.TaskID, req.SessionID, err)
            return fmt.Errorf("prepare environment: %w", err)
        }

        // Apply result (workspace path may have changed due to worktree)
        if result.WorkspacePath != "" {
            req.WorkspacePath = result.WorkspacePath
        }
        // Store worktree metadata for later use
        if result.WorktreeID != "" {
            req.Metadata[MetadataKeyWorktreeID] = result.WorktreeID
            req.Metadata[MetadataKeyWorktreeBranch] = result.WorktreeBranch
        }

        m.eventPublisher.PublishPrepareCompleted(req.TaskID, req.SessionID)
    }

    // 5. Create instance (container or standalone process)
    instance, err := backend.CreateInstance(ctx, &ExecutorCreateRequest{
        ProfileConfig: mergedConfig,
        ...
    })

    // 6. For Docker/remote: run post-instance preparation (image build, deps install)
    // (uses instance.Client to run commands inside container)

    // 7. Configure and start agent (existing flow)
    // ...
}
```

### Execution Flow by Executor Type

```
LOCAL EXECUTOR:
  Prepare (pre-instance):
    DEPS: (no-op — tools already on host)
    REPO:
      1. git fetch origin <branch>              → progress event
      2. git pull --ff-only origin <branch>     → progress event (non-fatal)
      3. Run repository setup script            → progress event (if configured)
  CreateInstance:
    4. Start standalone agentctl process
  Start:
    5. Configure + start agent subprocess

WORKTREE EXECUTOR:
  Prepare (pre-instance):
    DEPS: (no-op — tools already on host)
    REPO:
      1. git fetch origin <branch>              → progress event
      2. git pull --ff-only origin <branch>     → progress event (non-fatal)
      3. git worktree add -b <branch> <path>    → progress event
      4. Run repository setup script            → progress event (if configured, fatal on failure)
  CreateInstance:
    5. Start standalone agentctl process (in worktree dir)
  Start:
    6. Configure + start agent subprocess

DOCKER EXECUTOR:
  Prepare (pre-instance — image build on host):
    DEPS:
      1. Check/build Docker image               → progress event
  CreateInstance:
    2. Create + start Docker container (repo mounted from host)
  Prepare (post-instance — inside container):
    REPO:
      3. git pull (if configured)               → progress event (via agentctl shell)
      4. Run repository setup script            → progress event (if configured)
  Start:
    5. Configure + start agent subprocess

REMOTE EXECUTOR (SSH/Sprites/K8s):
  CreateInstance:
    1. Create remote instance (VM, pod, sprite)
  Prepare (post-instance — on remote):
    DEPS:
      2. Install agentctl binary                → progress event
      3. Install agent CLI                      → progress event
      4. Copy git config                        → progress event (if configured)
      5. Copy GH auth / SSH keys                → progress event (if configured)
      6. Copy agent config                      → progress event (if configured)
      7. Install additional packages            → progress event (if configured)
    REPO:
      8. git clone <url> <workspace>            → progress event (FATAL on failure)
      9. git checkout <branch>                  → progress event (non-fatal)
     10. Run repository setup script            → progress event (if configured, fatal)
  Start:
    11. Configure + start agent subprocess
```

---

## Health Check System

### Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                     Health Check Loop                              │
│                                                                    │
│  lifecycle.Manager.healthCheckLoop() (goroutine)                  │
│                                                                    │
│  Every 30s:                                                       │
│    for each running execution:                                    │
│      if execution.backend.IsRemote():                             │
│        err := execution.agentctl.HealthCheck(ctx)                 │
│        if err:                                                    │
│          execution.healthStatus = "unhealthy"                     │
│          execution.healthFailCount++                              │
│          if healthFailCount >= 3:                                 │
│            emit SessionHealthDegraded event                       │
│        else:                                                      │
│          execution.healthStatus = "healthy"                       │
│          execution.healthFailCount = 0                            │
│        execution.healthCheckedAt = now()                          │
│        upsert executors_running (health_status, health_checked_at)│
│        emit ExecutorHealthUpdate event                            │
│                                                                    │
│  Events:                                                          │
│    executor.health_update → {session_id, status, checked_at}     │
│    session.health_degraded → {session_id, error}                 │
└──────────────────────────────────────────────────────────────────┘
```

### Implementation

```go
// In lifecycle manager:
const (
    healthCheckInterval = 30 * time.Second
    healthCheckTimeout  = 10 * time.Second
    healthFailThreshold = 3
)

func (m *Manager) healthCheckLoop(ctx context.Context) {
    ticker := time.NewTicker(healthCheckInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.runHealthChecks(ctx)
        }
    }
}

func (m *Manager) runHealthChecks(ctx context.Context) {
    executions := m.executionStore.ListActive()
    for _, exec := range executions {
        // Only health-check remote executors
        backend, err := m.executorRegistry.GetBackend(executor.Name(exec.BackendName))
        if err != nil || !backend.IsRemote() {
            continue
        }

        checkCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
        err = exec.agentctl.HealthCheck(checkCtx)
        cancel()

        if err != nil {
            exec.healthFailCount++
            m.updateHealthStatus(exec, "unhealthy")
            if exec.healthFailCount >= healthFailThreshold {
                m.eventPublisher.PublishSessionHealthDegraded(exec.SessionID, err)
            }
        } else {
            exec.healthFailCount = 0
            m.updateHealthStatus(exec, "healthy")
        }
    }
}
```

### Health Status Events (WS)

```json
{
    "type": "executor.health_update",
    "payload": {
        "session_id": "sess-123",
        "health_status": "healthy",      // "healthy" | "unhealthy" | "unknown"
        "health_checked_at": "2026-02-20T10:30:00Z"
    }
}
```

---

## API Endpoints

### Executor Profile Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/executors/:id/profiles` | List profiles for executor |
| `POST` | `/api/v1/executors/:id/profiles` | Create profile |
| `GET` | `/api/v1/executors/:id/profiles/:profileId` | Get profile |
| `PATCH` | `/api/v1/executors/:id/profiles/:profileId` | Update profile |
| `DELETE` | `/api/v1/executors/:id/profiles/:profileId` | Delete profile |

### Updated Executor Endpoints

The existing executor endpoints remain but responses now include profiles:

```json
// GET /api/v1/executors/:id — now includes profiles
{
    "id": "exec-docker",
    "name": "Docker",
    "type": "docker",
    "status": "active",
    "is_system": false,
    "resumable": true,
    "config": {},
    "profiles": [
        {
            "id": "prof-local",
            "name": "local",
            "is_default": true,
            "config": {
                "host": "/var/run/docker.sock",
                "image_tag": "kandev-agent:latest"
            }
        },
        {
            "id": "prof-remote",
            "name": "remote-server",
            "is_default": false,
            "config": {
                "host": "tcp://192.168.1.100:2376",
                "image_tag": "kandev-agent:latest"
            }
        }
    ]
}
```

### Session Start Request Update

```json
// task.start payload — now includes profile_id
{
    "task_id": "task-123",
    "executor_id": "exec-docker",
    "profile_id": "prof-remote",     // NEW: optional, defaults to is_default profile
    "agent_profile_id": "agent-prof-123",
    "workspace_id": "ws-123"
}
```

### Health Status in Session Response

```json
// GET /api/v1/sessions/:id — now includes executor health
{
    "id": "sess-123",
    "executor_id": "exec-docker",
    "profile_id": "prof-remote",
    "executor_health": {
        "status": "healthy",
        "checked_at": "2026-02-20T10:30:00Z"
    }
}
```

### WebSocket Actions

| Action | Payload | Description |
|--------|---------|-------------|
| `executor.profile.list` | `{executor_id}` | List profiles |
| `executor.profile.create` | `{executor_id, name, config, is_default?}` | Create profile |
| `executor.profile.update` | `{id, name?, config?, is_default?}` | Update profile |
| `executor.profile.delete` | `{id}` | Delete profile |

### Preparation Progress Events (WS Notifications)

These are server-push events sent during environment preparation. The frontend subscribes to these to show real-time progress.

```json
// executor.prepare.started — preparation phase begins
{
    "type": "executor.prepare.started",
    "payload": {
        "task_id": "task-123",
        "session_id": "sess-456",
        "timestamp": "2026-02-20T10:30:00.000Z"
    }
}

// executor.prepare.progress — individual step update (sent multiple times)
{
    "type": "executor.prepare.progress",
    "payload": {
        "task_id": "task-123",
        "session_id": "sess-456",
        "execution_id": "exec-789",
        "phase": "repo",                          // "deps" | "repo"
        "step": "git_fetch",
        "step_index": 0,
        "total_steps": 4,
        "status": "completed",                    // "running" | "completed" | "failed" | "skipped"
        "command": "git fetch origin main",
        "stdout": "From github.com:org/repo\n * branch main -> FETCH_HEAD\n",
        "stderr": "",
        "exit_code": 0,
        "message": "Fetched latest changes from origin",
        "timestamp": "2026-02-20T10:30:01.234Z"
    }
}

// executor.prepare.completed — preparation phase done
{
    "type": "executor.prepare.completed",
    "payload": {
        "task_id": "task-123",
        "session_id": "sess-456",
        "worktree_id": "wt-abc",
        "worktree_path": "/home/user/kandev/worktrees/fix-login-bug_a1b2c3d4",
        "worktree_branch": "feature/fix-login-bug-xyz",
        "timestamp": "2026-02-20T10:30:03.000Z"
    }
}

// executor.prepare.failed — preparation failed
{
    "type": "executor.prepare.failed",
    "payload": {
        "task_id": "task-123",
        "session_id": "sess-456",
        "error": "create worktree: git worktree add failed: exit code 128",
        "step": "worktree_create",
        "timestamp": "2026-02-20T10:30:02.000Z"
    }
}
```

---

## Frontend Implementation

### Type Updates: `lib/types/http.ts`

```typescript
export type ExecutorType =
    | 'local'
    | 'worktree'
    | 'docker'       // replaces 'local_docker'
    | 'ssh'
    | 'k8s'
    | 'sprites'
    | 'local_docker'  // deprecated alias
    | 'remote_docker'; // deprecated alias

export interface ExecutorProfile {
    id: string;
    executor_id: string;
    name: string;
    is_default: boolean;
    config: Record<string, unknown>;
    created_at: string;
    updated_at: string;
}

export interface Executor {
    id: string;
    name: string;
    type: ExecutorType;
    status: 'active' | 'disabled';
    is_system: boolean;
    resumable: boolean;
    config?: Record<string, string>;
    profiles?: ExecutorProfile[];
    created_at: string;
    updated_at: string;
}

export interface ExecutorHealth {
    status: 'healthy' | 'unhealthy' | 'unknown';
    checked_at?: string;
}
```

### Executor Icons Update: `lib/executor-icons.ts`

```typescript
// Add new executor type icons:
const executorIcons: Record<string, IconType> = {
    local: IconFolder,
    worktree: IconFolders,
    docker: IconBrandDocker,     // was local_docker
    local_docker: IconBrandDocker, // deprecated alias
    remote_docker: IconCloud,      // deprecated alias
    ssh: IconTerminal2,            // NEW
    k8s: IconBrandKubernetes,      // NEW (or IconHexagons)
    sprites: IconSparkles,         // NEW
};

const executorLabels: Record<string, string> = {
    local: 'Local',
    worktree: 'Worktree',
    docker: 'Docker',
    local_docker: 'Docker',        // deprecated alias
    remote_docker: 'Remote Docker', // deprecated alias
    ssh: 'SSH',
    k8s: 'Kubernetes',
    sprites: 'Sprites',
};
```

### Settings Store Update: `lib/state/slices/settings/settings-slice.ts`

```typescript
// Remove environments state, add profiles to executors:
executors: {
    items: Executor[];          // now includes profiles[]
    loading: boolean;
    loaded: boolean;
}

// Remove:
// environments: { items: Environment[] }
```

### Executor Settings Page: `components/settings/executor-page.tsx`

```
┌──────────────────────────────────────────────────────────────────┐
│  Docker Executor                                    [Active ●]   │
│──────────────────────────────────────────────────────────────────│
│                                                                  │
│  Type: Docker                                                    │
│  System: No                    Resumable: Yes                    │
│                                                                  │
│  ┌─── Profiles ───────────────────────────────────────────────┐  │
│  │                                                     [+ Add]│  │
│  │  ┌─────────────────────────────────────────────────────┐   │  │
│  │  │ ★ local (default)                                   │   │  │
│  │  │   Host: /var/run/docker.sock                        │   │  │
│  │  │   Image: kandev-agent:latest                        │   │  │
│  │  │                              [Edit] [Set Default]   │   │  │
│  │  └─────────────────────────────────────────────────────┘   │  │
│  │                                                            │  │
│  │  ┌─────────────────────────────────────────────────────┐   │  │
│  │  │   remote-server                                     │   │  │
│  │  │   Host: tcp://192.168.1.100:2376                    │   │  │
│  │  │   Image: kandev-agent:latest                        │   │  │
│  │  │                        [Edit] [Set Default] [Delete]│   │  │
│  │  └─────────────────────────────────────────────────────┘   │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  [Delete Executor]                                               │
└──────────────────────────────────────────────────────────────────┘
```

### Profile Edit Dialog

```
┌──────────────────────────────────────────────┐
│  Edit Profile: local                         │
│──────────────────────────────────────────────│
│                                              │
│  Name:  [local_______________]               │
│                                              │
│  Docker Host:                                │
│  [/var/run/docker.sock_______]               │
│                                              │
│  Image Tag:                                  │
│  [kandev-agent:latest________]               │
│                                              │
│  Dockerfile: (optional)                      │
│  ┌──────────────────────────────────┐        │
│  │ FROM ubuntu:22.04               │        │
│  │ RUN apt-get update && ...       │        │
│  └──────────────────────────────────┘        │
│                                              │
│  ☐ Set as default profile                    │
│                                              │
│              [Cancel]  [Save]                │
└──────────────────────────────────────────────┘
```

The dialog renders different fields based on executor type using a config schema per type.

### Health Indicator on Session Cards

```
┌──────────────────────────────────────┐
│  Task: Fix login bug                 │
│  Session #1                          │
│                                      │
│  Agent: Claude Code • auto-approve   │
│  Executor: Docker • remote-server    │
│  Health: ● healthy                   │  ← green dot for healthy
│                                      │  ← red dot for unhealthy
│  Status: Running                     │  ← gray dot for unknown
│  ...                                 │
└──────────────────────────────────────┘
```

Implementation in session card component:

```typescript
function ExecutorHealthDot({ health }: { health?: ExecutorHealth }) {
    if (!health) return null;

    const colors = {
        healthy: 'bg-green-500',
        unhealthy: 'bg-red-500',
        unknown: 'bg-gray-400',
    };

    return (
        <span className={cn('inline-block h-2 w-2 rounded-full', colors[health.status])}
              title={`Health: ${health.status}${health.checked_at ? ` (checked ${formatRelative(health.checked_at)})` : ''}`}
        />
    );
}
```

### WS Handler for Health Updates

```typescript
// lib/ws/handlers/executor-health.ts
export function registerExecutorHealthHandlers(dispatcher: WSDispatcher) {
    dispatcher.on('executor.health_update', (payload) => {
        const { session_id, health_status, health_checked_at } = payload;
        useAppStore.getState().updateSessionExecutorHealth(session_id, {
            status: health_status,
            checked_at: health_checked_at,
        });
    });
}
```

### Preparation Progress State & Components

#### Types: `lib/types/backend.ts`

```typescript
export interface PrepareProgressEvent {
    task_id: string;
    session_id: string;
    execution_id: string;
    phase: 'deps' | 'repo';
    step: string;           // "git_fetch" | "git_pull" | "git_clone" | "git_checkout" | "worktree_create" | "setup_script" | "install_agentctl" | "install_agent_cli" | "copy_gh_auth" | "copy_git_config" | ...
    step_index: number;
    total_steps: number;
    status: 'running' | 'completed' | 'failed' | 'skipped';
    command?: string;
    stdout?: string;
    stderr?: string;
    exit_code?: number;
    message?: string;
    error?: string;
    timestamp: string;
}

export type PrepareStatus = 'idle' | 'preparing' | 'completed' | 'failed';

export interface SessionPrepareState {
    status: PrepareStatus;
    steps: PrepareProgressEvent[];
    error?: string;
}
```

#### State: `lib/state/slices/session-runtime/types.ts`

```typescript
// Add to SessionRuntimeSlice:
prepare: {
    bySessionId: Record<string, SessionPrepareState>;
};

// Actions:
setPrepareStarted: (sessionId: string) => void;
addPrepareStep: (sessionId: string, step: PrepareProgressEvent) => void;
setPrepareCompleted: (sessionId: string) => void;
setPrepareFailed: (sessionId: string, error: string) => void;
clearPrepareState: (sessionId: string) => void;
```

#### WS Handler: `lib/ws/handlers/executor-prepare.ts`

```typescript
export function registerExecutorPrepareHandlers(store: StoreApi<AppState>): WsHandlers {
    return {
        'executor.prepare.started': (message) => {
            const { session_id } = message.payload;
            store.getState().setPrepareStarted(session_id);
        },
        'executor.prepare.progress': (message) => {
            const payload = message.payload as PrepareProgressEvent;
            store.getState().addPrepareStep(payload.session_id, payload);
        },
        'executor.prepare.completed': (message) => {
            const { session_id } = message.payload;
            store.getState().setPrepareCompleted(session_id);
        },
        'executor.prepare.failed': (message) => {
            const { session_id, error } = message.payload;
            store.getState().setPrepareFailed(session_id, error);
        },
    };
}
```

#### Component: `components/session/prepare-progress.tsx`

Shows real-time preparation progress in the session panel (above the chat).

```
┌──────────────────────────────────────────────────────────────────┐
│  ⚙ Preparing environment...                          [2/4]      │
│──────────────────────────────────────────────────────────────────│
│                                                                  │
│  ✓ git fetch origin main                              0.8s      │
│    From github.com:org/repo                                      │
│    * branch main -> FETCH_HEAD                                   │
│                                                                  │
│  ✓ git pull --ff-only origin main                     0.3s      │
│    Already up to date.                                           │
│                                                                  │
│  ◐ Creating isolated worktree...                                │
│    git worktree add -b feature/fix-login-bug-xyz                │
│    /home/user/kandev/worktrees/fix-login-bug_a1b2c3d4           │
│    origin/main                                                   │
│                                                                  │
│  ○ Run setup script                                  (pending)  │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

After completion, collapses to a summary line:

```
┌──────────────────────────────────────────────────────────────────┐
│  ✓ Environment ready • worktree: feature/fix-login-bug-xyz   ▸  │
└──────────────────────────────────────────────────────────────────┘
```

On failure:

```
┌──────────────────────────────────────────────────────────────────┐
│  ✗ Environment preparation failed                               │
│──────────────────────────────────────────────────────────────────│
│                                                                  │
│  ✓ git fetch origin main                              0.8s      │
│  ✓ git pull --ff-only origin main                     0.3s      │
│  ✗ git worktree add ...                               exit: 128 │
│    stderr: fatal: 'feature/fix-login-bug' already exists        │
│                                                                  │
│                                                    [Retry]       │
└──────────────────────────────────────────────────────────────────┘
```

```typescript
function PrepareProgress({ sessionId }: { sessionId: string }) {
    const prepareState = useAppStore(
        (s) => s.prepare.bySessionId[sessionId]
    );

    if (!prepareState || prepareState.status === 'idle') return null;

    const isActive = prepareState.status === 'preparing';
    const isFailed = prepareState.status === 'failed';
    const isCompleted = prepareState.status === 'completed';

    // Collapsed summary after completion
    if (isCompleted && !expanded) {
        return <PrepareCompletedSummary steps={prepareState.steps} />;
    }

    return (
        <div className="border rounded-lg p-3 mb-3">
            <PrepareHeader status={prepareState.status} steps={prepareState.steps} />
            <div className="space-y-2 mt-2">
                {prepareState.steps.map((step, i) => (
                    <PrepareStepItem key={`${step.step}-${i}`} step={step} />
                ))}
            </div>
            {isFailed && <PrepareFailedActions sessionId={sessionId} />}
        </div>
    );
}

function PrepareStepItem({ step }: { step: PrepareProgressEvent }) {
    const statusIcon = {
        running: <Spinner className="h-3 w-3" />,
        completed: <CheckIcon className="h-3 w-3 text-green-500" />,
        failed: <XIcon className="h-3 w-3 text-red-500" />,
        skipped: <MinusIcon className="h-3 w-3 text-muted-foreground" />,
    }[step.status];

    return (
        <div className="text-sm font-mono">
            <div className="flex items-center gap-2">
                {statusIcon}
                <span className="text-muted-foreground">{step.command || step.message}</span>
                {step.exit_code != null && (
                    <Badge variant={step.exit_code === 0 ? 'secondary' : 'destructive'}>
                        exit: {step.exit_code}
                    </Badge>
                )}
            </div>
            {step.stdout && (
                <pre className="ml-5 text-xs text-muted-foreground whitespace-pre-wrap">
                    {step.stdout}
                </pre>
            )}
            {step.stderr && (
                <pre className="ml-5 text-xs text-red-400 whitespace-pre-wrap">
                    {step.stderr}
                </pre>
            )}
        </div>
    );
}
```

---

## Tracing

All executor lifecycle and preparation events are instrumented with OpenTelemetry spans, following the existing `kandev-transport` tracer pattern in `internal/agentctl/tracing/`.

### New Trace Functions: `tracing/executor.go`

```go
const executorTracerName = "kandev-executor"

func executorTracer() trace.Tracer {
    return Tracer(executorTracerName)
}

// TraceExecutorPrepare creates a span covering the entire preparation phase.
// The span contains child spans for each preparation step.
func TraceExecutorPrepare(ctx context.Context, taskID, sessionID, executorType string) (context.Context, trace.Span) {
    ctx, span := executorTracer().Start(ctx, "executor.prepare",
        trace.WithSpanKind(trace.SpanKindInternal),
    )
    span.SetAttributes(
        attribute.String("task_id", taskID),
        attribute.String("session_id", sessionID),
        attribute.String("executor_type", executorType),
    )
    return ctx, span
}

// TraceExecutorPrepareStep creates a child span for a single preparation step.
func TraceExecutorPrepareStep(ctx context.Context, step string, command string) (context.Context, trace.Span) {
    ctx, span := executorTracer().Start(ctx, "executor.prepare."+step,
        trace.WithSpanKind(trace.SpanKindInternal),
    )
    span.SetAttributes(
        attribute.String("step", step),
        attribute.String("command", command),
    )
    return ctx, span
}

// TraceExecutorPrepareStepResult records the outcome of a preparation step.
func TraceExecutorPrepareStepResult(span trace.Span, exitCode int, stdout, stderr string, err error) {
    span.SetAttributes(
        attribute.Int("exit_code", exitCode),
        attribute.String("stdout", truncate(stdout, 4096)),
        attribute.String("stderr", truncate(stderr, 4096)),
    )
    if err != nil {
        span.SetStatus(codes.Error, err.Error())
        span.RecordError(err)
    }
    span.End()
}

// TraceExecutorCreateInstance creates a span for executor instance creation.
func TraceExecutorCreateInstance(ctx context.Context, taskID, sessionID, backendName string) (context.Context, trace.Span) {
    ctx, span := executorTracer().Start(ctx, "executor.create_instance",
        trace.WithSpanKind(trace.SpanKindInternal),
    )
    span.SetAttributes(
        attribute.String("task_id", taskID),
        attribute.String("session_id", sessionID),
        attribute.String("backend", backendName),
    )
    return ctx, span
}

// TraceExecutorHealthCheck creates a span for a health check.
func TraceExecutorHealthCheck(ctx context.Context, sessionID, backendName string) (context.Context, trace.Span) {
    ctx, span := executorTracer().Start(ctx, "executor.health_check",
        trace.WithSpanKind(trace.SpanKindInternal),
    )
    span.SetAttributes(
        attribute.String("session_id", sessionID),
        attribute.String("backend", backendName),
    )
    return ctx, span
}

// TraceExecutorStop creates a span for executor instance teardown.
func TraceExecutorStop(ctx context.Context, sessionID, executionID string, force bool) (context.Context, trace.Span) {
    ctx, span := executorTracer().Start(ctx, "executor.stop",
        trace.WithSpanKind(trace.SpanKindInternal),
    )
    span.SetAttributes(
        attribute.String("session_id", sessionID),
        attribute.String("execution_id", executionID),
        attribute.Bool("force", force),
    )
    return ctx, span
}
```

### Trace Integration in Preparers

```go
// In WorktreePreparer.Prepare():
func (p *WorktreePreparer) Prepare(ctx context.Context, req *EnvPrepareRequest) (*EnvPrepareResult, error) {
    // Parent span for entire preparation
    ctx, prepSpan := tracing.TraceExecutorPrepare(ctx, req.TaskID, req.SessionID, "worktree")
    defer prepSpan.End()

    // Step 1: git fetch
    stepCtx, fetchSpan := tracing.TraceExecutorPrepareStep(ctx, "git_fetch", cmd)
    stdout, stderr, exitCode, err := runGitCommand(stepCtx, ...)
    tracing.TraceExecutorPrepareStepResult(fetchSpan, exitCode, stdout, stderr, err)

    // Step 2: git pull
    stepCtx, pullSpan := tracing.TraceExecutorPrepareStep(ctx, "git_pull", cmd)
    stdout, stderr, exitCode, err = runGitCommand(stepCtx, ...)
    tracing.TraceExecutorPrepareStepResult(pullSpan, exitCode, stdout, stderr, err)

    // Step 3: worktree create
    stepCtx, wtSpan := tracing.TraceExecutorPrepareStep(ctx, "worktree_create", cmd)
    wt, err := p.worktreeMgr.Create(stepCtx, ...)
    if err != nil {
        wtSpan.SetStatus(codes.Error, err.Error())
        wtSpan.End()
        prepSpan.SetStatus(codes.Error, "worktree creation failed")
        return nil, err
    }
    wtSpan.SetAttributes(
        attribute.String("worktree_id", wt.ID),
        attribute.String("worktree_branch", wt.Branch),
        attribute.String("worktree_path", wt.Path),
    )
    wtSpan.End()

    // ...
}
```

### Trace Hierarchy

```
session (long-lived root span)
  │
  ├─ executor.prepare (duration of entire preparation)
  │   │
  │   ├─ executor.prepare.deps (phase 1 — tools & credentials)
  │   │    ├─ executor.prepare.install_agentctl     (remote only)
  │   │    ├─ executor.prepare.install_agent_cli     (remote only)
  │   │    ├─ executor.prepare.copy_git_config       (remote only, if configured)
  │   │    ├─ executor.prepare.copy_gh_auth          (remote only, if configured)
  │   │    └─ executor.prepare.docker_build          (docker only, if Dockerfile)
  │   │
  │   └─ executor.prepare.repo (phase 2 — repository setup)
  │        ├─ executor.prepare.git_clone             (remote only)
  │        │    attrs: url, workspace_path, exit_code, stdout, stderr
  │        ├─ executor.prepare.git_fetch             (local/worktree)
  │        │    attrs: command, exit_code, stdout, stderr
  │        ├─ executor.prepare.git_pull              (local/worktree)
  │        │    attrs: command, exit_code, stdout, stderr
  │        ├─ executor.prepare.git_checkout          (remote only)
  │        │    attrs: branch, exit_code
  │        ├─ executor.prepare.worktree_create       (worktree only)
  │        │    attrs: worktree_id, worktree_branch, worktree_path
  │        └─ executor.prepare.setup_script          (all types, if configured)
  │             attrs: command, exit_code, stdout, stderr
  │
  ├─ executor.create_instance
  │    attrs: backend, container_id (if docker)
  │
  ├─ session.init (existing span)
  │   ├─ HTTP: POST /api/v1/configure
  │   ├─ HTTP: POST /api/v1/start
  │   └─ WS: session/new
  │
  └─ executor.health_check (periodic, remote only)
       attrs: session_id, backend, status
```

---

## Database Schema (New Service)

This is a new service — no migrations from existing tables are needed. All tables are created fresh in `initSchema()`.

### Tables

```sql
-- Executor profiles table (new)
CREATE TABLE IF NOT EXISTS executor_profiles (
    id           TEXT PRIMARY KEY,
    executor_id  TEXT NOT NULL REFERENCES executors(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    is_default   INTEGER NOT NULL DEFAULT 0,
    config       TEXT NOT NULL DEFAULT '{}',   -- JSON: type-specific config
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at   TIMESTAMP DEFAULT NULL,
    UNIQUE(executor_id, name)
);

CREATE INDEX IF NOT EXISTS idx_executor_profiles_executor_id
    ON executor_profiles(executor_id);
```

### Column Additions: `executors_running`

These columns are added to the existing `executors_running` table:

```sql
ALTER TABLE executors_running ADD COLUMN profile_id TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN executor_backend TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN remote_id TEXT DEFAULT '';
ALTER TABLE executors_running ADD COLUMN health_status TEXT DEFAULT 'unknown';
ALTER TABLE executors_running ADD COLUMN health_checked_at TIMESTAMP DEFAULT NULL;
```

### Default Profiles

Created in `ensureDefaultExecutorsAndProfiles()`:

```go
func ensureDefaultExecutorsAndProfiles(ctx context.Context, db *sqlx.DB) error {
    // exec-local (Local) + "default" profile (empty config)
    // exec-worktree (Worktree) + "default" profile (worktree_root config)
    // exec-docker (Docker) + "local" profile (host: /var/run/docker.sock)
}
```

---

## Backward Compatibility

### API Compatibility

- `local_docker` and `remote_docker` executor types are accepted as aliases
- API responses include both old and new field names during transition

### Frontend Compatibility

- Old executor type strings mapped to new types in `executor-icons.ts`
- Settings sidebar removes environments section (replaced by executor profiles)

### Timeline

1. **Phase 1**: Add executor_profiles table, profile CRUD, runtime→executor rename, preparation abstraction
2. **Phase 2**: Frontend preparation progress UI, tracing instrumentation
3. **Phase 3**: Remove deprecated type aliases, remove old environment code

---

## Files Changed Summary

### New Files
| File | Description |
|------|-------------|
| `apps/backend/internal/agent/executor/executor.go` | New package replacing `runtime` |
| `apps/backend/internal/agent/lifecycle/executor_backend.go` | Renamed from `runtime.go` |
| `apps/backend/internal/agent/lifecycle/executor_registry.go` | Renamed from `runtime_registry.go` |
| `apps/backend/internal/agent/lifecycle/executor_docker.go` | Renamed from `runtime_docker.go` |
| `apps/backend/internal/agent/lifecycle/executor_standalone.go` | Renamed from `runtime_standalone.go` |
| `apps/backend/internal/agent/lifecycle/executor_remote_docker.go` | Renamed from `runtime_remote_docker.go` |
| `apps/backend/internal/agent/lifecycle/env_preparer.go` | EnvironmentPreparer interface + registry + types |
| `apps/backend/internal/agent/lifecycle/env_preparer_local.go` | Local preparer (git pull) |
| `apps/backend/internal/agent/lifecycle/env_preparer_worktree.go` | Worktree preparer (git pull + worktree create + setup script) |
| `apps/backend/internal/agent/lifecycle/env_preparer_docker.go` | Docker image build preparer |
| `apps/backend/internal/agent/lifecycle/env_preparer_remote.go` | Remote preparer (SSH/Sprites/K8s) |
| `apps/backend/internal/agentctl/tracing/executor.go` | OTel trace functions for executor lifecycle |
| `apps/backend/internal/task/repository/sqlite/executor_profile.go` | Profile CRUD |
| `apps/backend/internal/task/handlers/executor_profile_handlers.go` | Profile API handlers |
| `apps/web/components/session/prepare-progress.tsx` | Preparation progress UI component |
| `apps/web/components/settings/executor-profile-dialog.tsx` | Profile create/edit dialog |
| `apps/web/lib/ws/handlers/executor-health.ts` | Health update WS handler |
| `apps/web/lib/ws/handlers/executor-prepare.ts` | Preparation progress WS handler |

### Deleted Files
| File | Description |
|------|-------------|
| `apps/backend/internal/agent/runtime/runtime.go` | Replaced by `executor/executor.go` |
| `apps/backend/internal/agent/lifecycle/runtime.go` | Renamed |
| `apps/backend/internal/agent/lifecycle/runtime_registry.go` | Renamed |
| `apps/backend/internal/agent/lifecycle/runtime_docker.go` | Renamed |
| `apps/backend/internal/agent/lifecycle/runtime_standalone.go` | Renamed |
| `apps/backend/internal/agent/lifecycle/runtime_remote_docker.go` | Renamed |

### Modified Files
| File | Change |
|------|--------|
| `apps/backend/internal/task/models/models.go` | Add ExecutorProfile, new types, ExecutorRunning health fields |
| `apps/backend/internal/task/repository/interface.go` | Add profile methods |
| `apps/backend/internal/task/repository/sqlite/base.go` | Create executor_profiles table |
| `apps/backend/internal/task/repository/sqlite/defaults.go` | Default profiles for system executors |
| `apps/backend/internal/task/repository/sqlite/executor.go` | Load profiles with executor |
| `apps/backend/internal/task/handlers/executor_handlers.go` | Include profiles in responses |
| `apps/backend/internal/agent/lifecycle/manager.go` | Use ExecutorRegistry, add health loop, integrate preparers |
| `apps/backend/internal/agent/lifecycle/manager_launch.go` | Call preparer before CreateInstance (local/worktree) |
| `apps/backend/internal/agent/lifecycle/events.go` | Add PublishPrepareProgress/Started/Completed/Failed |
| `apps/backend/internal/agent/lifecycle/event_types.go` | Add PrepareProgressEvent, PrepareStarted/Completed/Failed payloads |
| `apps/backend/internal/agent/lifecycle/execution_store.go` | Track health status |
| `apps/backend/internal/events/types.go` | Add executor.prepare.* event types + subject builders |
| `apps/backend/internal/gateway/websocket/session_notifications.go` | Subscribe to prepare events, broadcast to frontend |
| `apps/backend/cmd/kandev/agents.go` | Register with new names |
| `apps/backend/cmd/kandev/routes.go` | Profile routes |
| All files importing `agent/runtime` | Update imports to `agent/executor` |
| `apps/web/lib/types/http.ts` | ExecutorProfile type, health types |
| `apps/web/lib/types/backend.ts` | PrepareProgressEvent, SessionPrepareState types |
| `apps/web/lib/executor-icons.ts` | New executor type icons |
| `apps/web/lib/state/slices/session-runtime/types.ts` | Add prepare state + actions |
| `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts` | Implement prepare state reducers |
| `apps/web/lib/state/slices/settings/settings-slice.ts` | Remove environments, add profile state |
| `apps/web/lib/api/domains/settings-api.ts` | Profile API client methods |
| `apps/web/lib/ws/router.ts` | Register executor-prepare + executor-health handlers |
| `apps/web/components/settings/settings-app-sidebar.tsx` | Remove environments section |
| `apps/web/components/settings/executor-page.tsx` | Show profiles |
| Session card components | Add health indicator |
| Session panel components | Add PrepareProgress component above chat |

---

*Last updated: 2026-02-20*
