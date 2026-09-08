# E2E Adapter Tests

End-to-end tests that exercise the full adapter lifecycle against real agent binaries.
They confirm message parsing, turn detection, and normalization still work when agent versions change.

**Location:** `internal/agentctl/server/adapter/e2e/`

## Prerequisites

- Agent CLI installed and on PATH, or `npx` available for npm-distributed agents (`auggie`, `gemini`, `opencode`)
- The harness checks only the first command token with `exec.LookPath`. For `npx` commands, it does not check the requested npm package before the test starts. Missing packages or network access can fail the test instead of skipping it.
- Active API subscription for the agent you're testing
- Tests for agents whose command binary is not installed are skipped automatically

## Running Tests

All commands run from `apps/backend/`.

### All agents at once

```bash
make test-e2e
```

Agents whose command binary is not on PATH are skipped. The mock agent is always built and tested.

### Single agent

**Auggie** (requires `npx` + Augment auth):

```bash
go test -tags e2e -v -timeout 10m -run TestAuggie ./internal/agentctl/server/adapter/e2e/
```

**Gemini** (requires `npx` + Google auth):

```bash
go test -tags e2e -v -timeout 10m -run TestGemini ./internal/agentctl/server/adapter/e2e/
```

**OpenCode** (requires `npx` + OpenCode auth):

```bash
go test -tags e2e -v -timeout 10m -run TestOpenCodeACP ./internal/agentctl/server/adapter/e2e/
```

**Mock Agent** (free, no API cost — binary is auto-built):

```bash
go test -tags e2e -v -run TestMockAgent ./internal/agentctl/server/adapter/e2e/
```

### Single test

```bash
go test -tags e2e -v -timeout 5m -run TestAuggie_BasicPrompt ./internal/agentctl/server/adapter/e2e/
```

## Available Tests

| Test | Agent | Protocol | What it checks |
|------|-------|----------|----------------|
| `TestAuggie_BasicPrompt` | Auggie | acp | Simple prompt completes a turn |
| `TestGemini_BasicPrompt` | Gemini | acp | Simple prompt completes a turn |
| `TestOpenCodeACP_BasicPrompt` | OpenCode | acp | Simple prompt completes a turn |
| `TestMockAgent_BasicPrompt` | Mock Agent | acp | Harness smoke test (no API cost) |

The mock agent has additional tests beyond `TestMockAgent_BasicPrompt` covering tool
call events, permission flow, multi-turn conversations, session reset/reload, and
thinking events — see `mock_agent_test.go` for the full list.

## Debugging Failures

### Debug message logs

Set `KANDEV_DEBUG_AGENT_MESSAGES` **before** running the test binary (the variable is read at package init time):

```bash
KANDEV_DEBUG_AGENT_MESSAGES=true \
KANDEV_DEBUG_LOG_DIR=/tmp/e2e-debug \
  go test -tags e2e -v -timeout 10m ./internal/agentctl/server/adapter/e2e/
```

This writes two JSONL files per agent run into the log dir:

- `raw-{protocol}-{agentId}.jsonl` — every message from the agent subprocess
- `normalized-{protocol}-{agentId}.jsonl` — normalized `AgentEvent` objects

### OTel tracing

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
KANDEV_DEBUG_AGENT_MESSAGES=true \
  go test -tags e2e -v -timeout 10m ./internal/agentctl/server/adapter/e2e/
```

### Test failure output

When a test fails, `DumpEventsOnFailure` prints all collected events with type, session ID, tool name, text length, and any error messages.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_WORKDIR` | temp dir with `git init` | Workspace directory for the agent |
| `E2E_TIMEOUT` | `2m` | Timeout per test (Go duration format) |
| `KANDEV_DEBUG_AGENT_MESSAGES` | `false` | Write raw/normalized message logs |
| `KANDEV_DEBUG_LOG_DIR` | cwd | Directory for debug log files |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTel collector endpoint for tracing |

## How It Works

Each test defines an `AgentSpec` with a hardcoded command (e.g. `npx -y @augmentcode/auggie --acp`).
The harness runs the full production code path:

1. Check binary on PATH via `exec.LookPath` — skip if not found
2. Create temp workspace with `git init`
3. Build `config.InstanceConfig` and create `process.Manager`
4. `Manager.Start()` — spawns subprocess, creates adapter via factory, wires stdin/stdout pipes
5. `Adapter.Initialize()` — protocol handshake
6. `Adapter.NewSession()` — create session
7. `Adapter.Prompt()` — send message, blocks until turn completes
8. Collect all `AgentEvent` objects from the manager's updates channel
9. Assert structural invariants (turn completed, no errors, at least one visible event)

## What Is Asserted

Tests assert **structural invariants only** — never specific text or tool names, since agent responses are non-deterministic:

- At least one event was received
- Session ID is non-empty
- No error events
- A complete event was received (turn finished)
- At least one user-visible event (message chunk, tool call, or reasoning)

## Adding a New Agent

1. Create `{agent}_test.go` with `//go:build e2e`
2. Define the command as a const (derive from the agent's definition in `internal/agent/agents/` — e.g. `auggie.go`, `opencode_acp.go`; the `_acp` suffix varies by agent)
3. Write test functions using `RunAgent(t, AgentSpec{...})`
4. Use `AssertTurnCompleted` for basic turn validation
