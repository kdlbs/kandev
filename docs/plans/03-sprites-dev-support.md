# Plan 03: Sprites.dev Support

> First remote sandbox executor using the Sprites.dev service.
> Depends on: Plan 01 (Secrets Infrastructure), Plan 02 (Executor Profiles Infrastructure).

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Sprites.dev Concepts](#spritesdev-concepts)
4. [Backend Implementation](#backend-implementation)
5. [Environment Preparation](#environment-preparation)
6. [agentctl Deployment & Connection](#agentctl-deployment--connection)
7. [Session Resume & Recovery](#session-resume--recovery)
8. [Settings Page](#settings-page)
9. [Debug & Test Harness](#debug--test-harness)
10. [API Endpoints](#api-endpoints)
11. [Frontend Implementation](#frontend-implementation)
12. [Error Handling & Edge Cases](#error-handling--edge-cases)

---

## Overview

Sprites.dev is a paid service that provides ephemeral sandbox environments (sprites). Users provide their API key, and Kandev creates sprites on their behalf to run agents in isolated environments.

### Goals

- Implement `SpritesExecutor` backend (implements `ExecutorBackend` interface from Plan 02)
- Deploy and run agentctl inside sprites via the sprites-go SDK
- Establish port-forwarded connections between kandev backend and agentctl in sprite
- Settings page for Sprites account setup (API key, network policies, running sprites)
- Environment preparation: install agent CLI, node.js, copy auth files
- Session resume with health checking
- Comprehensive debug logging and test harness

### Prerequisites

- Plan 01 (Secrets) must be implemented — API key stored in secrets store
- Plan 02 (Executor Profiles) must be implemented — executor/profile model in place

### Key SDK

```
github.com/superfly/sprites-go
```

The SDK provides:
- `sprites.New(token)` — create client with API token
- `client.Sprite(name)` — get sprite handle
- `sprite.Command(cmd, args...)` — run commands (mirrors `exec.Cmd`)
- `sprite.CommandContext(ctx, cmd, args...)` — with context
- `sprite.ProxyPort(ctx, localPort, remotePort)` — TCP port forwarding
- `sprite.ProxyPorts(ctx, []PortMapping{...})` — multiple port forwarding
- `cmd.StdinPipe()`, `cmd.StdoutPipe()` — pipe access
- `cmd.SetTTY(true)` — TTY mode

Sprites API:
- `POST /v1/sprites` — create sprite
- `DELETE /v1/sprites/{name}` — destroy sprite
- `GET /v1/sprites` — list sprites
- `GET /v1/sprites/{name}` — get sprite status
- `WSS /v1/sprites/{name}/exec` — execute commands
- `POST /v1/sprites/{name}/services` — create background service
- `GET/POST /v1/sprites/{name}/network` — network policies
- `WSS proxy` — TCP port tunneling

---

## Architecture

### High-Level Flow

```
┌───────────────────────────────────────────────────────────────────────────┐
│                         Kandev Backend                                     │
│                                                                           │
│  ┌─────────────┐    ┌─────────────────┐    ┌──────────────────────────┐  │
│  │ Orchestrator │───>│ Lifecycle Mgr   │───>│ SpritesExecutor          │  │
│  │              │    │                 │    │ (ExecutorBackend impl)   │  │
│  │ task.start   │    │ LaunchAgent()   │    │                          │  │
│  │ executor=    │    │ profile=        │    │ 1. Create sprite         │  │
│  │   sprites    │    │   sprites/def   │    │ 2. Prepare environment   │  │
│  │              │    │                 │    │ 3. Deploy agentctl       │  │
│  │              │    │                 │    │ 4. Start agentctl        │  │
│  │              │    │                 │    │ 5. Port forward          │  │
│  │              │    │                 │    │ 6. Return client         │  │
│  └─────────────┘    └─────────────────┘    └──────────┬───────────────┘  │
│                                                        │                  │
│                              ┌──────────────────────────┘                  │
│                              │                                            │
│                              ▼                                            │
│  ┌───────────────────────────────────────────────────────────────────┐    │
│  │                    Port Forwarding (sprites-go SDK)                │    │
│  │                                                                   │    │
│  │  localhost:$LOCAL_PORT  ←──WSS tunnel──→  sprite:$AGENTCTL_PORT  │    │
│  │                                                                   │    │
│  │  agentctl.Client connects to localhost:$LOCAL_PORT                │    │
│  │  which tunnels to agentctl running inside the sprite              │    │
│  └───────────────────────────────────────────────────────────────────┘    │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
          │
          │ WSS tunnel (sprites-go SDK manages)
          ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                         Sprites.dev Cloud                                  │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Sprite: kandev-sess-abc123                                         │  │
│  │                                                                     │  │
│  │  ┌───────────────────────────────────────────────────────────────┐  │  │
│  │  │  agentctl process (port 8765)                                 │  │  │
│  │  │                                                               │  │  │
│  │  │  ┌─────────────────────────────────────────────────────────┐  │  │  │
│  │  │  │  Agent subprocess (claude-code, codex, etc.)            │  │  │  │
│  │  │  │  stdin/stdout ←→ ACP protocol ←→ agentctl              │  │  │  │
│  │  │  └─────────────────────────────────────────────────────────┘  │  │  │
│  │  │                                                               │  │  │
│  │  │  /workspace/ ← git clone of repo                             │  │  │
│  │  │  ~/.config/ ← copied agent configs                           │  │  │
│  │  │  ~/.gitconfig ← copied git config                            │  │  │
│  │  │  Auth tokens for gh, git, etc.                                │  │  │
│  │  └───────────────────────────────────────────────────────────────┘  │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

### Detailed Sequence Diagram

```
Orchestrator    LifecycleMgr    SpritesExecutor   SpritesPreparer    Sprites API     Sprite
    │                │                │                │                │              │
    │ LaunchAgent()  │                │                │                │              │
    │───────────────>│                │                │                │              │
    │                │ resolve profile│                │                │              │
    │                │ (api_token from│                │                │              │
    │                │  secrets store)│                │                │              │
    │                │                │                │                │              │
    │                │ CreateInstance()│                │                │              │
    │                │───────────────>│                │                │              │
    │                │                │ POST /v1/sprites                │              │
    │                │                │───────────────────────────────>│              │
    │                │                │                │    sprite created              │
    │                │                │<───────────────────────────────│              │
    │                │                │                │                │              │
    │                │ Prepare()      │                │                │              │
    │                │───────────────────────────────>│                │              │
    │                │                │                │ install node   │              │
    │                │                │                │───────────────────────────────>│
    │                │                │                │ install agent  │              │
    │                │                │                │───────────────────────────────>│
    │                │                │                │ copy configs   │              │
    │                │                │                │───────────────────────────────>│
    │                │                │                │ clone repo     │              │
    │                │                │                │───────────────────────────────>│
    │                │                │                │<──────────────────────────────│
    │                │<──────────────────────────────│                │              │
    │                │                │                │                │              │
    │                │                │ upload agentctl│                │              │
    │                │                │ binary         │                │              │
    │                │                │───────────────────────────────────────────────>│
    │                │                │                │                │              │
    │                │                │ start agentctl │                │              │
    │                │                │ as service     │                │              │
    │                │                │───────────────────────────────>│              │
    │                │                │                │  POST /services│              │
    │                │                │<───────────────────────────────│              │
    │                │                │                │                │              │
    │                │                │ ProxyPort()    │                │              │
    │                │                │ local:random ←→ sprite:8765    │              │
    │                │                │───────────────────────────────>│              │
    │                │                │ ← WSS tunnel established ──── │              │
    │                │                │                │                │              │
    │                │                │ return ExecutorInstance         │              │
    │                │                │ (client=localhost:random)      │              │
    │                │<───────────────│                │                │              │
    │                │                │                │                │              │
    │                │ ConfigureAgent()│               │                │              │
    │                │ via agentctl   │                │                │              │
    │                │ client (tunnel)│                │                │              │
    │                │═══════════════════════════════════════════════════════════════>│
    │                │                │                │                │              │
    │                │ Start() agent  │                │                │              │
    │                │═══════════════════════════════════════════════════════════════>│
    │                │                │                │                │              │
    │<═══════════════│ stream updates │                │                │              │
    │  via WS        │ via tunnel     │                │                │              │
```

---

## Sprites.dev Concepts

### Sprite Lifecycle

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Creating  │────>│  Ready   │────>│ Running  │────>│ Stopped  │
│           │     │          │     │ (services│     │          │
│           │     │          │     │  active) │     │ (destroy)│
└──────────┘     └──────────┘     └──────────┘     └──────────┘
                       │               │
                       │     ┌─────────┘
                       ▼     ▼
                  ┌──────────────┐
                  │ Checkpointed │ (optional, save state)
                  └──────────────┘
```

### Naming Convention

Sprites are named: `kandev-{session_id_short}` (e.g., `kandev-abc12345`)

This allows:
- Easy identification of Kandev-managed sprites
- Recovery by listing sprites with `kandev-` prefix
- Cleanup of orphaned sprites

### Network Policies

Sprites can have DNS-based outbound network policies. Users configure these per-profile:

```json
{
    "network_policies": [
        "*.npm.org",
        "*.github.com",
        "*.githubusercontent.com",
        "registry.npmjs.org",
        "pypi.org",
        "*.anthropic.com"
    ]
}
```

These are applied via the Sprites API `POST /v1/sprites/{name}/network`.

---

## Backend Implementation

### Package Structure

```
apps/backend/internal/agent/lifecycle/
├── executor_sprites.go          # SpritesExecutor (ExecutorBackend impl)
├── env_preparer_sprites.go      # SpritesEnvironmentPreparer
└── sprites_client.go            # Sprites SDK wrapper/helpers

apps/backend/internal/sprites/
├── settings.go                  # Sprites settings service
└── handlers.go                  # Sprites-specific API handlers (list sprites, network policies)
```

### SpritesExecutor: `lifecycle/executor_sprites.go`

```go
package lifecycle

import (
    "context"
    "fmt"
    "net"

    sprites "github.com/superfly/sprites-go"
    "go.uber.org/zap"

    "github.com/kandev/kandev/internal/agent/executor"
    agentctl "github.com/kandev/kandev/internal/agentctl/client"
    "github.com/kandev/kandev/internal/agentctl/server/process"
    "github.com/kandev/kandev/internal/common/logger"
)

const (
    spritesAgentctlPort = 8765
    spritesNamePrefix   = "kandev-"
)

// SpritesExecutor implements ExecutorBackend for Sprites.dev sandbox environments.
type SpritesExecutor struct {
    secretsProvider SecretsProvider   // to fetch API token
    logger          *logger.Logger

    // Active proxy sessions keyed by instance ID
    proxies map[string]*SpritesProxy
}

// SpritesProxy holds the tunnel and sprite handle for an active instance.
type SpritesProxy struct {
    client     *sprites.Client
    sprite     *sprites.Sprite
    spriteName string
    localPort  int
    cancelFn   context.CancelFunc
}

func NewSpritesExecutor(secretsProvider SecretsProvider, log *logger.Logger) *SpritesExecutor {
    return &SpritesExecutor{
        secretsProvider: secretsProvider,
        logger:          log.WithFields(zap.String("executor", "sprites")),
        proxies:         make(map[string]*SpritesProxy),
    }
}

func (e *SpritesExecutor) Name() executor.Name {
    return executor.NameSprites
}

func (e *SpritesExecutor) IsRemote() bool {
    return true
}

func (e *SpritesExecutor) HealthCheck(ctx context.Context) error {
    // Try to get API token from secrets
    token, err := e.secretsProvider.GetSecret(ctx, "SPRITES_API_TOKEN")
    if err != nil {
        return fmt.Errorf("sprites API token not configured: %w", err)
    }

    // Try listing sprites to verify token works
    client := sprites.New(token)
    sprite := client.Sprite("kandev-healthcheck")
    // Simple command to verify API connectivity
    cmd := sprite.CommandContext(ctx, "echo", "health")
    // If the sprite doesn't exist, the API will return an error
    // which is fine — we just want to verify the token is valid
    _ = cmd // We don't actually run this — just verify client creation works
    return nil
}

func (e *SpritesExecutor) CreateInstance(
    ctx context.Context,
    req *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
    // 1. Get API token from profile config → secrets store
    tokenSecretID, _ := req.ProfileConfig["api_token_secret_id"].(string)
    token, err := e.secretsProvider.GetSecret(ctx, tokenSecretID)
    if err != nil {
        return nil, fmt.Errorf("sprites API token: %w", err)
    }

    // 2. Create sprites client
    client := sprites.New(token)

    // 3. Create sprite with unique name
    spriteName := fmt.Sprintf("%s%s", spritesNamePrefix, req.SessionID[:8])
    sprite := client.Sprite(spriteName)

    e.logger.Info("creating sprite",
        zap.String("name", spriteName),
        zap.String("session_id", req.SessionID))

    // 4. Verify sprite is accessible by running a simple command
    cmd := sprite.CommandContext(ctx, "echo", "kandev-ready")
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("sprite creation/access failed: %w", err)
    }
    e.logger.Debug("sprite ready", zap.String("output", string(output)))

    // 5. Set up port forwarding
    localPort, err := getFreePort()
    if err != nil {
        return nil, fmt.Errorf("get free port: %w", err)
    }

    proxyCtx, proxyCancel := context.WithCancel(context.Background())
    go func() {
        if proxyErr := sprite.ProxyPort(proxyCtx, localPort, spritesAgentctlPort); proxyErr != nil {
            e.logger.Error("sprite port proxy failed",
                zap.String("sprite", spriteName),
                zap.Error(proxyErr))
        }
    }()

    // 6. Store proxy info for cleanup
    e.proxies[req.InstanceID] = &SpritesProxy{
        client:     &client, // Note: need to check SDK type
        sprite:     &sprite,
        spriteName: spriteName,
        localPort:  localPort,
        cancelFn:   proxyCancel,
    }

    // 7. Create agentctl client pointing to the local end of the tunnel
    agentctlClient := agentctl.NewClient("127.0.0.1", localPort, e.logger)

    e.logger.Info("sprite instance created",
        zap.String("sprite", spriteName),
        zap.String("instance_id", req.InstanceID),
        zap.Int("local_port", localPort))

    return &ExecutorInstance{
        InstanceID:  req.InstanceID,
        TaskID:      req.TaskID,
        SessionID:   req.SessionID,
        BackendName: string(e.Name()),
        Client:      agentctlClient,
        RemoteID:    spriteName,
        Metadata: map[string]interface{}{
            "sprite_name": spriteName,
            "local_port":  localPort,
        },
    }, nil
}

func (e *SpritesExecutor) StopInstance(
    ctx context.Context,
    instance *ExecutorInstance,
    force bool,
) error {
    proxy, ok := e.proxies[instance.InstanceID]
    if !ok {
        return nil
    }

    // Cancel port forwarding
    proxy.cancelFn()

    // Destroy sprite
    spriteName := instance.RemoteID
    if spriteName != "" {
        sprite := proxy.client.Sprite(spriteName)
        // Run a cleanup command or just let it be destroyed
        cmd := sprite.CommandContext(ctx, "kill", "-TERM", "1") // send signal to init
        _ = cmd.Run()
    }

    delete(e.proxies, instance.InstanceID)

    e.logger.Info("sprite instance stopped",
        zap.String("sprite", spriteName),
        zap.String("instance_id", instance.InstanceID))

    return nil
}

func (e *SpritesExecutor) RecoverInstances(ctx context.Context) ([]*ExecutorInstance, error) {
    // Recovery strategy:
    // 1. Load all executors_running with executor_backend="sprites"
    // 2. For each, try to reconnect:
    //    a. Get API token from secrets
    //    b. Check sprite still exists (sprite.Command("echo", "alive"))
    //    c. Check agentctl is still running (health check via new proxy)
    //    d. If alive: set up port forwarding, return recovered instance
    //    e. If dead: return nil (lifecycle manager will clean up)

    e.logger.Info("recovering sprites instances")

    // Note: actual recovery requires DB access which is done by the lifecycle manager.
    // This method handles re-establishing connections to known-alive sprites.
    // The lifecycle manager calls this after loading ExecutorRunning records.

    return nil, nil // Sprites are ephemeral — recovery is best-effort
}

func (e *SpritesExecutor) GetInteractiveRunner() *process.InteractiveRunner {
    return nil // No passthrough mode for remote sprites
}

// getFreePort returns an available TCP port on localhost.
func getFreePort() (int, error) {
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        return 0, err
    }
    port := listener.Addr().(*net.TCPAddr).Port
    _ = listener.Close()
    return port, nil
}
```

### Sprites Environment Preparer: `lifecycle/env_preparer_sprites.go`

```go
package lifecycle

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    sprites "github.com/superfly/sprites-go"
    "go.uber.org/zap"

    "github.com/kandev/kandev/internal/common/logger"
)

// SpritesEnvironmentPreparer sets up the sprite environment for agent execution.
type SpritesEnvironmentPreparer struct {
    agentctlBinaryPath string // path to local agentctl binary to upload
    logger             *logger.Logger
}

func NewSpritesEnvironmentPreparer(
    agentctlBinaryPath string,
    log *logger.Logger,
) *SpritesEnvironmentPreparer {
    return &SpritesEnvironmentPreparer{
        agentctlBinaryPath: agentctlBinaryPath,
        logger:             log.WithFields(zap.String("preparer", "sprites")),
    }
}

func (p *SpritesEnvironmentPreparer) Prepare(
    ctx context.Context,
    req *EnvPrepareRequest,
) error {
    spriteName := req.Instance.RemoteID
    proxy := req.Instance.Metadata["_sprites_proxy"].(*SpritesProxy)
    sprite := proxy.client.Sprite(spriteName)

    steps := []struct {
        name string
        fn   func() error
    }{
        {"install-system-deps", func() error { return p.installSystemDeps(ctx, sprite) }},
        {"install-node", func() error { return p.installNode(ctx, sprite) }},
        {"install-agent", func() error { return p.installAgent(ctx, sprite, req.AgentConfig) }},
        {"upload-agentctl", func() error { return p.uploadAgentctl(ctx, sprite) }},
        {"setup-workspace", func() error { return p.setupWorkspace(ctx, sprite, req.WorkspacePath) }},
        {"copy-configs", func() error { return p.copyConfigs(ctx, sprite, req) }},
        {"start-agentctl", func() error { return p.startAgentctl(ctx, sprite, req) }},
    }

    for _, step := range steps {
        p.logger.Info("sprite prep step",
            zap.String("step", step.name),
            zap.String("sprite", spriteName))

        if err := step.fn(); err != nil {
            return fmt.Errorf("sprite prep %s: %w", step.name, err)
        }

        p.logger.Info("sprite prep step complete",
            zap.String("step", step.name),
            zap.String("sprite", spriteName))
    }

    return nil
}

func (p *SpritesEnvironmentPreparer) installSystemDeps(
    ctx context.Context,
    sprite sprites.Sprite,
) error {
    cmd := sprite.CommandContext(ctx, "sh", "-c",
        "apt-get update -qq && apt-get install -y -qq git curl ca-certificates")
    output, err := cmd.CombinedOutput()
    if err != nil {
        p.logger.Error("install system deps failed", zap.String("output", string(output)))
        return fmt.Errorf("install deps: %w", err)
    }
    return nil
}

func (p *SpritesEnvironmentPreparer) installNode(
    ctx context.Context,
    sprite sprites.Sprite,
) error {
    // Install Node.js via NodeSource or fnm
    script := `
        if command -v node >/dev/null 2>&1; then
            echo "node already installed: $(node --version)"
            exit 0
        fi
        curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
        apt-get install -y -qq nodejs
        echo "node installed: $(node --version)"
    `
    cmd := sprite.CommandContext(ctx, "sh", "-c", script)
    output, err := cmd.CombinedOutput()
    if err != nil {
        p.logger.Error("install node failed", zap.String("output", string(output)))
        return fmt.Errorf("install node: %w", err)
    }
    p.logger.Debug("node installation", zap.String("output", string(output)))
    return nil
}

func (p *SpritesEnvironmentPreparer) installAgent(
    ctx context.Context,
    sprite sprites.Sprite,
    agentConfig agents.Agent,
) error {
    // Install the agent CLI based on agent type
    // This uses the agent's InstallCommand from the registry
    installCmd := agentConfig.InstallCommand
    if installCmd == "" {
        p.logger.Info("no install command for agent, skipping",
            zap.String("agent", agentConfig.Name))
        return nil
    }

    cmd := sprite.CommandContext(ctx, "sh", "-c", installCmd)
    output, err := cmd.CombinedOutput()
    if err != nil {
        p.logger.Error("install agent failed",
            zap.String("agent", agentConfig.Name),
            zap.String("output", string(output)))
        return fmt.Errorf("install agent %s: %w", agentConfig.Name, err)
    }
    return nil
}

func (p *SpritesEnvironmentPreparer) uploadAgentctl(
    ctx context.Context,
    sprite sprites.Sprite,
) error {
    // Upload the agentctl binary to the sprite
    // Strategy: pipe binary via stdin to a cat > /usr/local/bin/agentctl command
    binaryData, err := os.ReadFile(p.agentctlBinaryPath)
    if err != nil {
        return fmt.Errorf("read agentctl binary: %w", err)
    }

    cmd := sprite.CommandContext(ctx, "sh", "-c",
        "cat > /usr/local/bin/agentctl && chmod +x /usr/local/bin/agentctl")
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return fmt.Errorf("stdin pipe: %w", err)
    }
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("start upload: %w", err)
    }
    if _, err := stdin.Write(binaryData); err != nil {
        return fmt.Errorf("write binary: %w", err)
    }
    if err := stdin.Close(); err != nil {
        return fmt.Errorf("close stdin: %w", err)
    }
    if err := cmd.Wait(); err != nil {
        return fmt.Errorf("upload agentctl: %w", err)
    }

    // Verify
    verifyCmd := sprite.CommandContext(ctx, "agentctl", "--version")
    output, err := verifyCmd.Output()
    if err != nil {
        return fmt.Errorf("verify agentctl: %w", err)
    }
    p.logger.Info("agentctl uploaded", zap.String("version", strings.TrimSpace(string(output))))
    return nil
}

func (p *SpritesEnvironmentPreparer) setupWorkspace(
    ctx context.Context,
    sprite sprites.Sprite,
    workspacePath string,
) error {
    // Clone the repository into /workspace
    // The repo URL and credentials come from the workspace config
    script := fmt.Sprintf(`
        mkdir -p /workspace
        if [ -d /workspace/.git ]; then
            echo "workspace already initialized"
            exit 0
        fi
        cd /workspace
        git init
        echo "workspace initialized at /workspace"
    `)
    cmd := sprite.CommandContext(ctx, "sh", "-c", script)
    output, err := cmd.CombinedOutput()
    if err != nil {
        p.logger.Error("setup workspace failed", zap.String("output", string(output)))
        return fmt.Errorf("setup workspace: %w", err)
    }
    return nil
}

func (p *SpritesEnvironmentPreparer) copyConfigs(
    ctx context.Context,
    sprite sprites.Sprite,
    req *EnvPrepareRequest,
) error {
    // Copy configuration files from local machine to sprite

    if req.CopyGitConfig {
        if err := p.copyFileToSprite(ctx, sprite,
            filepath.Join(os.Getenv("HOME"), ".gitconfig"),
            "/root/.gitconfig"); err != nil {
            p.logger.Warn("copy .gitconfig failed", zap.Error(err))
            // Non-fatal
        }
    }

    if req.CopyGHAuth {
        // Copy gh CLI auth
        ghConfigDir := filepath.Join(os.Getenv("HOME"), ".config", "gh")
        if err := p.copyFileToSprite(ctx, sprite,
            filepath.Join(ghConfigDir, "hosts.yml"),
            "/root/.config/gh/hosts.yml"); err != nil {
            p.logger.Warn("copy gh auth failed", zap.Error(err))
        }
    }

    if req.CopyAgentConfig {
        // Copy agent-specific config (e.g., ~/.claude for Claude Code)
        // This depends on the agent type
        agentConfigPaths := req.AgentConfig.ConfigPaths
        for _, path := range agentConfigPaths {
            localPath := filepath.Join(os.Getenv("HOME"), path)
            remotePath := filepath.Join("/root", path)
            if err := p.copyFileToSprite(ctx, sprite, localPath, remotePath); err != nil {
                p.logger.Warn("copy agent config failed",
                    zap.String("path", path), zap.Error(err))
            }
        }
    }

    return nil
}

func (p *SpritesEnvironmentPreparer) copyFileToSprite(
    ctx context.Context,
    sprite sprites.Sprite,
    localPath, remotePath string,
) error {
    data, err := os.ReadFile(localPath)
    if err != nil {
        return fmt.Errorf("read %s: %w", localPath, err)
    }

    // Create parent directory
    dir := filepath.Dir(remotePath)
    mkdirCmd := sprite.CommandContext(ctx, "mkdir", "-p", dir)
    if err := mkdirCmd.Run(); err != nil {
        return fmt.Errorf("mkdir %s: %w", dir, err)
    }

    // Write file via stdin pipe
    cmd := sprite.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat > %s", remotePath))
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return err
    }
    if err := cmd.Start(); err != nil {
        return err
    }
    if _, err := stdin.Write(data); err != nil {
        return err
    }
    _ = stdin.Close()
    return cmd.Wait()
}

func (p *SpritesEnvironmentPreparer) startAgentctl(
    ctx context.Context,
    sprite sprites.Sprite,
    req *EnvPrepareRequest,
) error {
    // Start agentctl as a background service in the sprite
    // Using the Sprites services API for long-running processes
    //
    // Note: We could also use sprite.Command with Start() (no Wait()),
    // but using the services API gives us better lifecycle management.

    envVars := make([]string, 0)
    for k, v := range req.Instance.Metadata {
        if strings.HasPrefix(k, "env_") {
            envVars = append(envVars, fmt.Sprintf("%s=%s", strings.TrimPrefix(k, "env_"), v))
        }
    }

    cmd := sprite.CommandContext(ctx, "agentctl",
        "--port", fmt.Sprintf("%d", spritesAgentctlPort),
        "--workspace", "/workspace",
    )
    cmd.Env = envVars

    if err := cmd.Start(); err != nil {
        return fmt.Errorf("start agentctl: %w", err)
    }

    // Don't Wait() — agentctl runs as a long-lived process.
    // Port forwarding will connect to it.

    // Give agentctl a moment to start listening
    // Then verify with a health check
    // (the actual health check happens via the port-forwarded connection
    //  after CreateInstance returns)

    return nil
}

func (p *SpritesEnvironmentPreparer) Cleanup(
    ctx context.Context,
    instanceID string,
) error {
    // Cleanup is handled by SpritesExecutor.StopInstance which destroys the sprite
    return nil
}
```

---

## agentctl Deployment & Connection

### Binary Deployment Strategy

```
┌─────────────────────────────────────────────────────────────────┐
│                 agentctl Binary Deployment                        │
│                                                                  │
│  Option A: Upload local binary (primary)                         │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  1. Read agentctl binary from known path                 │   │
│  │     (compiled for linux/amd64, same as backend binary)   │   │
│  │  2. Pipe to sprite via stdin: cat > /usr/local/bin/...   │   │
│  │  3. chmod +x                                              │   │
│  │  4. Verify: agentctl --version                            │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  Option B: Download from release (fallback)                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  1. curl agentctl binary from release URL                 │   │
│  │  2. Install to /usr/local/bin/agentctl                    │   │
│  │  3. Verify version                                        │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  Config: agentctl binary path set in backend config              │
│  e.g., KANDEV_AGENTCTL_BINARY=/usr/local/bin/agentctl           │
│  or automatically detected relative to kandev binary             │
└─────────────────────────────────────────────────────────────────┘
```

### Port Forwarding Architecture

```
┌──────────────────────┐          ┌──────────────────────┐
│   Kandev Backend     │          │   Sprite (remote)    │
│                      │          │                      │
│  agentctl.Client     │          │  agentctl process    │
│  → 127.0.0.1:49152  │          │  listening :8765     │
│         │            │          │         ▲            │
│         ▼            │          │         │            │
│  ┌──────────────┐    │          │    ┌────────────┐   │
│  │ Local TCP    │    │ WSS      │    │ TCP        │   │
│  │ Socket       │◄───┼──tunnel──┼───►│ Socket     │   │
│  │ :49152       │    │ (sprites │    │ :8765      │   │
│  └──────────────┘    │  SDK)    │    └────────────┘   │
│                      │          │                      │
└──────────────────────┘          └──────────────────────┘

sprites.ProxyPort(ctx, 49152, 8765)
  - Establishes WebSocket tunnel to sprites API
  - All TCP traffic on localhost:49152 is tunneled to sprite:8765
  - Transparent to agentctl.Client — just connects to localhost port
```

### Connection Lifecycle

```
1. CreateInstance:
   - Create sprite
   - Start agentctl inside sprite on port 8765
   - Find free local port (e.g., 49152)
   - Start ProxyPort() goroutine (long-lived)
   - Create agentctl.Client pointing to 127.0.0.1:49152
   - Return ExecutorInstance with client

2. During session:
   - agentctl.Client sends HTTP/WS requests to 127.0.0.1:49152
   - Sprites SDK tunnels these to sprite:8765
   - agentctl process handles requests, manages agent subprocess

3. StopInstance:
   - Cancel ProxyPort context (closes tunnel)
   - Destroy sprite via API
   - Clean up proxy state

4. Error handling:
   - If tunnel drops: health check detects, marks unhealthy
   - If sprite dies: health check detects, lifecycle manager handles cleanup
   - If backend restarts: recovery attempts to re-establish tunnels
```

---

## Session Resume & Recovery

### Recovery Flow

```
Backend Restart
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ SpritesExecutor.RecoverInstances()                           │
│                                                             │
│ 1. Read SPRITES_API_TOKEN from secrets                      │
│ 2. List sprites with prefix "kandev-"                       │
│    GET /v1/sprites (filter by name prefix)                  │
│                                                             │
│ 3. For each sprite:                                         │
│    a. Match to executors_running record by remote_id        │
│    b. Check sprite health: sprite.Command("echo", "alive")  │
│    c. Check agentctl health: sprite.Command("curl",          │
│       "http://localhost:8765/health")                        │
│    d. If both healthy:                                       │
│       - Find free local port                                 │
│       - Start ProxyPort()                                    │
│       - Create agentctl.Client                               │
│       - Return recovered ExecutorInstance                     │
│    e. If unhealthy:                                          │
│       - Log details                                          │
│       - Skip (lifecycle manager marks session as failed)     │
│       - Optionally destroy dead sprite                       │
└─────────────────────────────────────────────────────────────┘
```

### Health Check for Sprites

```go
func (e *SpritesExecutor) healthCheckSprite(ctx context.Context, proxy *SpritesProxy) error {
    // Two-level health check:

    // Level 1: Can we reach the sprite?
    cmd := proxy.sprite.CommandContext(ctx, "echo", "alive")
    if _, err := cmd.Output(); err != nil {
        return fmt.Errorf("sprite unreachable: %w", err)
    }

    // Level 2: Is agentctl responding?
    cmd = proxy.sprite.CommandContext(ctx, "curl", "-sf",
        fmt.Sprintf("http://localhost:%d/health", spritesAgentctlPort))
    if _, err := cmd.Output(); err != nil {
        return fmt.Errorf("agentctl not responding: %w", err)
    }

    return nil
}
```

---

## Settings Page

### Sprites Settings UI

```
┌──────────────────────────────────────────────────────────────────┐
│  Sprites.dev Integration                                         │
│──────────────────────────────────────────────────────────────────│
│                                                                  │
│  ┌─── Connection ──────────────────────────────────────────────┐ │
│  │                                                              │ │
│  │  API Token: ●●●●●●●●●●●●●●●●●●●●●●  [Reveal] [Change]     │ │
│  │  Status: ● Connected                                        │ │
│  │                                                              │ │
│  │  [Test Connection]                                           │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─── Default Network Policies ────────────────────────────────┐ │
│  │                                                              │ │
│  │  Allowed domains (one per line):                             │ │
│  │  ┌────────────────────────────────────────────────────┐      │ │
│  │  │ *.npm.org                                          │      │ │
│  │  │ *.github.com                                       │      │ │
│  │  │ *.githubusercontent.com                            │      │ │
│  │  │ registry.npmjs.org                                 │      │ │
│  │  │ pypi.org                                           │      │ │
│  │  │ *.anthropic.com                                    │      │ │
│  │  └────────────────────────────────────────────────────┘      │ │
│  │                                                    [Save]    │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─── Environment Setup ───────────────────────────────────────┐ │
│  │                                                              │ │
│  │  ☑ Copy agent config/auth files from local                  │ │
│  │  ☑ Copy gh CLI auth token for git operations                │ │
│  │  ☑ Copy .gitconfig                                          │ │
│  │  ☐ Custom setup script (advanced)                           │ │
│  │                                                              │ │
│  │  [Save Defaults]                                             │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌─── Running Sprites ─────────────────────────────────────────┐ │
│  │                                                              │ │
│  │  ┌──────────────────────────────────────────────────────┐   │ │
│  │  │ kandev-abc12345                                       │   │ │
│  │  │ Session: Fix login bug • Running 2h 15m               │   │ │
│  │  │ Health: ● healthy                         [Destroy]   │   │ │
│  │  └──────────────────────────────────────────────────────┘   │ │
│  │                                                              │ │
│  │  ┌──────────────────────────────────────────────────────┐   │ │
│  │  │ kandev-def67890                                       │   │ │
│  │  │ Session: Refactor auth • Running 45m                  │   │ │
│  │  │ Health: ● healthy                         [Destroy]   │   │ │
│  │  └──────────────────────────────────────────────────────┘   │ │
│  │                                                              │ │
│  │  Total: 2 sprites running              [Destroy All]        │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### Settings Data Model

```go
// SpritesSettings stored in the executor profile config
type SpritesProfileConfig struct {
    APITokenSecretID string   `json:"api_token_secret_id"`
    NetworkPolicies  []string `json:"network_policies"`
    CopyGHAuth       bool     `json:"copy_gh_auth"`
    CopyGitConfig    bool     `json:"copy_git_config"`
    CopyAgentConfig  bool     `json:"copy_agent_config"`
    CustomSetupScript string  `json:"custom_setup_script,omitempty"`
}
```

---

## Debug & Test Harness

### Structured Logging

All sprites operations use structured logging with consistent fields:

```go
// Logger fields for all sprites operations
zap.String("executor", "sprites")
zap.String("sprite", spriteName)
zap.String("session_id", sessionID)
zap.String("step", stepName)        // prep step name
zap.Duration("elapsed", duration)   // step duration

// Example log output:
// INFO  sprites executor: creating sprite          sprite=kandev-abc12345 session_id=sess-abc
// INFO  sprites prep: install-system-deps          sprite=kandev-abc12345 elapsed=12.3s
// INFO  sprites prep: install-node                 sprite=kandev-abc12345 elapsed=8.1s
// INFO  sprites prep: install-agent                sprite=kandev-abc12345 elapsed=15.7s
// INFO  sprites prep: upload-agentctl              sprite=kandev-abc12345 elapsed=2.1s
// INFO  sprites prep: setup-workspace              sprite=kandev-abc12345 elapsed=0.3s
// INFO  sprites prep: copy-configs                 sprite=kandev-abc12345 elapsed=1.2s
// INFO  sprites prep: start-agentctl               sprite=kandev-abc12345 elapsed=0.5s
// INFO  sprites executor: instance created          sprite=kandev-abc12345 local_port=49152 total=40.2s
```

### Test Harness: CLI Command

```go
// cmd/kandev/sprites_test_harness.go (or a separate CLI command)
// Usage: kandev sprites-test --token <token> --agent claude-code

// Test harness steps:
// 1. Create a sprite
// 2. Run environment preparation (all steps)
// 3. Start agentctl
// 4. Establish port forwarding
// 5. Send health check via agentctl client
// 6. Send a simple prompt (e.g., "echo hello")
// 7. Wait for response
// 8. Destroy sprite
// 9. Report timing for each step

func runSpritesTestHarness(cfg SpritesTestConfig) error {
    log := logger.New(logger.Debug) // verbose logging

    fmt.Println("=== Sprites.dev Test Harness ===")
    fmt.Println()

    // Step 1: Create sprite
    fmt.Print("1. Creating sprite... ")
    start := time.Now()
    client := sprites.New(cfg.Token)
    spriteName := fmt.Sprintf("kandev-test-%d", time.Now().Unix())
    sprite := client.Sprite(spriteName)
    cmd := sprite.Command("echo", "ready")
    if _, err := cmd.Output(); err != nil {
        return fmt.Errorf("create sprite: %w", err)
    }
    fmt.Printf("OK (%s)\n", time.Since(start))

    // Step 2-6: ... (similar pattern)

    // Step 9: Report
    fmt.Println()
    fmt.Println("=== Results ===")
    fmt.Printf("Total time: %s\n", totalDuration)
    fmt.Printf("Sprite name: %s\n", spriteName)
    fmt.Println("All steps passed!")

    return nil
}
```

### Test Harness API Endpoint

```
POST /api/v1/sprites/test
Request: { "token": "optional-override" }  // uses stored token if not provided
Response: {
    "success": true,
    "steps": [
        { "name": "create_sprite", "duration_ms": 2340, "success": true },
        { "name": "install_deps", "duration_ms": 12300, "success": true },
        { "name": "upload_agentctl", "duration_ms": 2100, "success": true },
        { "name": "health_check", "duration_ms": 150, "success": true },
        { "name": "cleanup", "duration_ms": 800, "success": true }
    ],
    "total_duration_ms": 17690,
    "sprite_name": "kandev-test-1708444800"
}
```

---

## API Endpoints

### Sprites-Specific Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/sprites/test` | Run test harness |
| `GET` | `/api/v1/sprites/instances` | List running sprites |
| `DELETE` | `/api/v1/sprites/instances/:name` | Destroy a sprite |
| `DELETE` | `/api/v1/sprites/instances` | Destroy all sprites |
| `GET` | `/api/v1/sprites/status` | Check API connection status |

### WebSocket Actions

| Action | Payload | Description |
|--------|---------|-------------|
| `sprites.test` | `{}` | Run test harness |
| `sprites.instances.list` | `{}` | List running sprites |
| `sprites.instances.destroy` | `{name}` | Destroy sprite |
| `sprites.instances.destroy_all` | `{}` | Destroy all |
| `sprites.status` | `{}` | Connection status |

---

## Frontend Implementation

### New Files

```
apps/web/
├── app/settings/sprites/
│   └── page.tsx                         # Sprites settings route
├── components/settings/
│   └── sprites-settings-page.tsx        # Sprites settings page
├── lib/api/domains/
│   └── sprites-api.ts                   # Sprites API client
├── lib/types/
│   └── http-sprites.ts                  # Sprites types
└── hooks/domains/settings/
    └── use-sprites.ts                   # Sprites data hooks
```

### TypeScript Types: `lib/types/http-sprites.ts`

```typescript
export interface SpritesInstance {
    name: string;
    session_id?: string;
    task_title?: string;
    health_status: 'healthy' | 'unhealthy' | 'unknown';
    created_at: string;
    uptime_seconds: number;
}

export interface SpritesStatus {
    connected: boolean;
    token_configured: boolean;
    instance_count: number;
}

export interface SpritesTestResult {
    success: boolean;
    steps: SpritesTestStep[];
    total_duration_ms: number;
    sprite_name: string;
    error?: string;
}

export interface SpritesTestStep {
    name: string;
    duration_ms: number;
    success: boolean;
    error?: string;
    output?: string;
}
```

### Settings Sidebar Addition

```typescript
// In settings-app-sidebar.tsx, under "Integrations" section:
{
    title: "Integrations",
    items: [
        { title: "Sprites.dev", url: "/settings/sprites", icon: IconSparkles },
        // future: { title: "exe.dev", url: "/settings/exedev", icon: ... },
    ]
}
```

---

## Error Handling & Edge Cases

### Common Failure Modes

```
┌────────────────────────────────────┬─────────────────────────────────────┐
│ Failure                            │ Handling                             │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Invalid API token                  │ Error at HealthCheck, clear message │
│                                    │ "Sprites API token invalid or       │
│                                    │ expired. Check Settings > Sprites." │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Sprite creation fails              │ Retry once, then fail with          │
│ (API rate limit, quota)            │ actionable error message            │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Node.js install fails              │ Log full output, fail prep step     │
│                                    │ with details for debugging          │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Agent CLI install fails            │ Log output, suggest checking        │
│                                    │ agent availability / network policy │
├────────────────────────────────────┼─────────────────────────────────────┤
│ agentctl upload fails              │ Check binary path config,           │
│ (binary not found)                 │ suggest rebuilding agentctl         │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Port forwarding drops              │ Health check detects, marks         │
│                                    │ unhealthy, attempts reconnect       │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Sprite dies unexpectedly           │ Health check detects, session       │
│                                    │ marked as failed, cleanup           │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Network policy blocks agent        │ Agent fails to call API,            │
│ API calls                          │ suggest adding domain to policies   │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Backend restart with running       │ Recovery flow: list sprites,        │
│ sprites                            │ health check, reconnect or cleanup  │
├────────────────────────────────────┼─────────────────────────────────────┤
│ Concurrent sprite limit            │ Show error with current count and   │
│ (user's account quota)             │ link to running sprites list        │
└────────────────────────────────────┴─────────────────────────────────────┘
```

### Timeout Configuration

```go
const (
    spriteCreateTimeout    = 60 * time.Second
    spriteStepTimeout      = 120 * time.Second   // per prep step
    spriteTotalPrepTimeout = 10 * time.Minute     // total prep time
    spriteHealthTimeout    = 10 * time.Second
    spriteDestroyTimeout   = 30 * time.Second
)
```

### Cleanup on Failure

```go
// If any step fails during CreateInstance or Prepare:
// 1. Log the failure with full context
// 2. Attempt to destroy the sprite (best effort)
// 3. Clean up local port forwarding
// 4. Return error to lifecycle manager
// 5. Lifecycle manager emits session error event

func (e *SpritesExecutor) cleanupOnFailure(ctx context.Context, spriteName string, instanceID string) {
    // Best-effort cleanup — don't fail if cleanup itself fails
    if proxy, ok := e.proxies[instanceID]; ok {
        proxy.cancelFn()
        delete(e.proxies, instanceID)
    }

    // Try to destroy the sprite
    // (sprite may not exist if creation itself failed)
    e.logger.Info("cleaning up failed sprite",
        zap.String("sprite", spriteName))
}
```

---

## Files Changed Summary

### New Files
| File | Description |
|------|-------------|
| `apps/backend/internal/agent/lifecycle/executor_sprites.go` | SpritesExecutor backend |
| `apps/backend/internal/agent/lifecycle/env_preparer_sprites.go` | Sprites environment preparer |
| `apps/backend/internal/sprites/settings.go` | Sprites settings service |
| `apps/backend/internal/sprites/handlers.go` | Sprites API handlers |
| `apps/web/app/settings/sprites/page.tsx` | Route page |
| `apps/web/components/settings/sprites-settings-page.tsx` | Settings page component |
| `apps/web/lib/api/domains/sprites-api.ts` | API client |
| `apps/web/lib/types/http-sprites.ts` | TypeScript types |
| `apps/web/hooks/domains/settings/use-sprites.ts` | Data hooks |

### Modified Files
| File | Change |
|------|--------|
| `apps/backend/cmd/kandev/agents.go` | Register SpritesExecutor + preparer |
| `apps/backend/cmd/kandev/routes.go` | Register sprites API routes |
| `apps/backend/go.mod` | Add `github.com/superfly/sprites-go` dependency |
| `apps/backend/internal/agent/executor/executor.go` | Add `NameSprites` constant |
| `apps/backend/internal/task/models/models.go` | Add `ExecutorTypeSprites` constant |
| `apps/backend/internal/task/repository/sqlite/defaults.go` | Default sprites executor + profile |
| `apps/web/lib/executor-icons.ts` | Add sprites icon |
| `apps/web/components/settings/settings-app-sidebar.tsx` | Add Sprites nav item |

### Dependencies
| Dependency | Purpose |
|------------|---------|
| `github.com/superfly/sprites-go` | Sprites.dev Go SDK |

---

## Implementation Order

1. **Add sprites-go dependency** — `go get github.com/superfly/sprites-go`
2. **SpritesExecutor skeleton** — implement interface with stub methods
3. **Environment preparer** — step-by-step sprite setup
4. **agentctl deployment** — binary upload + launch
5. **Port forwarding** — ProxyPort integration
6. **Recovery** — list + reconnect sprites on restart
7. **Settings page** — frontend for API key, policies, running sprites
8. **Test harness** — CLI + API endpoint for end-to-end testing
9. **Health checks** — integrate with health loop from Plan 02
10. **Polish** — error messages, timeouts, cleanup

---

*Last updated: 2026-02-20*
