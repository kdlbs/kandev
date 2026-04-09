# 0002: Host Utility Agentctl for Sessionless ACP Flows

**Status:** accepted
**Date:** 2026-04-08
**Area:** backend

## Context

Utility agent calls (enhance-prompt, commit-message, etc.) historically required an active task session: `lifecycle.Manager.ExecuteInferencePrompt` took a `sessionID`, used `GetOrEnsureExecution` to locate that task's agentctl instance, and spawned a one-shot ACP subprocess inside it. This coupled *any* one-shot inference to an existing workspace, blocking three emerging needs:

1. Boot-time capability probes (list models/modes/auth for each installed agent so the UI can populate settings).
2. On-demand "refresh" of those capabilities from settings.
3. Running `enhance-prompt` in the new-task modal — before any task or workspace exists.

Built-in utility prompt templates are self-contained (they inject context as template variables rather than reading files), so the task workspace cwd was incidental — the coupling was structural, not semantic.

## Decision

Introduced a **host utility** subsystem (`internal/agent/hostutility/Manager`) that maintains one long-lived, workspace-only agentctl instance per ACP-native inference agent type, each bound to a process-scoped tmp directory. The manager is started after `waitForAgentctlControlHealthy` in `cmd/kandev/main.go`, runs parallel boot probes via `errgroup`, and owns an in-memory capability cache.

Key pieces:

- **Probe path.** New `ACPInferenceExecutor.Probe` in `internal/agentctl/server/utility/acp_executor.go` runs `initialize` + `session/new` against an ephemeral subprocess and returns the parsed `ProbeResponse` (agent info, auth methods, models, modes, prompt capabilities). Exposed over HTTP as `POST /api/v1/inference/probe` with a matching `Client.Probe` client method.
- **Warm instances, ephemeral ACP.** The per-agent-type instance is workspace-only — no persistent agent subprocess. Each probe/prompt call still spawns its own short-lived ACP subprocess via the existing `/api/v1/inference/prompt` and new `/api/v1/inference/probe` routes. The "warm" part is the agentctl instance host, not the ACP session.
- **Sessionless utility execute.** `POST /api/v1/utility/execute` now treats `session_id` as optional. When absent, `internal/utility/handlers/handlers.go:executeSessionless` routes through `HostUtilityExecutor.ExecutePrompt`, template substitution uses `MissingAsEmpty: true` for variables that are meaningless without a session (GitDiff, TaskTitle, etc.), and the `UtilityAgentCall` record is still written.
- **Public HTTP surface** (`internal/agent/capabilities/handlers/`, prefix `/api/v1/agent-capabilities` — separate from `/api/v1/agents/:id/...` to avoid a Gin wildcard name collision):
  - `GET  /api/v1/agent-capabilities` — list all cached
  - `GET  /api/v1/agent-capabilities/:type` — single agent
  - `POST /api/v1/agent-capabilities/:type/probe` — re-probe + refresh cache
  - `POST /api/v1/agent-capabilities/:type/prompt` — raw one-off inference (no utility agent record)
- **Tmp dir lifetime.** Each kandev process creates `kandev-host-utility-<pid>-*` with per-agent subdirs; `Stop()` removes only dirs owned by this process. Concurrent kandev processes never share or clobber each other's tmp state.
- **Scope.** v1 probes only agents whose `Runtime().Protocol == ProtocolACP` (claude-acp, codex-acp, copilot-acp, amp-acp, opencode-acp, auggie). Non-ACP adapters (streamjson, codex, copilot, amp, opencode) are explicitly out of scope; they can be extended later.
- **Model resolution** for sessionless calls: explicit arg → cached probe `currentModelId`. Static per-agent model lists have been removed; `InferenceAgent` no longer exposes an `InferenceModels()` method.
- **Failure isolation.** Per-agent-type probe failures land in the cache as `{Status: failed|auth_required|not_installed, Error}` without aborting other probes or blocking boot.

## Consequences

**Easier:**
- Settings can show per-agent status/auth/models driven by a single endpoint.
- Enhance-prompt works before a task exists without synthesizing a throwaway session.
- Probe and sessionless prompt share the same underlying path — one mechanism, three features.
- Cold-start is bounded to the ACP subprocess spawn (~100–300ms); the agentctl host is already warm.

**Harder:**
- One more long-lived subsystem to supervise. Crashes are lazy-repaired on next use; there is no background health loop.
- Host instances run with backend privileges in a tmp dir. The capabilities handlers do not expose shell/files endpoints, but raw agent-level permissions on the underlying agentctl instance are not fenced off in code — discipline is required if future endpoints are added.
- Utility templates that genuinely need to read workspace files cannot use the sessionless path. For v1 this is fine (all built-ins are self-contained); a `requires_workspace` flag on `UtilityAgent` is the future escape hatch.

## Alternatives Considered

1. **Ephemeral per-call agentctl.** Spawn agentctl, run inference, tear down. Cleanest isolation but adds cold-start latency (agentctl boot + `npx` resolution) to every Enhance click. Rejected — feels sluggish for UI-driven flows.
2. **Direct vendor SDK calls from Go.** Skip ACP entirely for inference and call Anthropic/OpenAI/etc. APIs directly. Simpler runtime but requires reimplementing auth and model listing per vendor, and diverges from the ACP-first design. Rejected as the default; may still be the right call for vendor-specific model listing in a later iteration.
3. **Reserved execution IDs in `ExecutionStore` with a `GetOrEnsureExecution` bypass.** Would have forced host utility through `lifecycle.Manager` and required a new `host-utility:` ID prefix branch in `manager_execution.go`. Rejected once it became clear that `ControlClient.CreateInstance` already does everything needed without touching the execution store.
4. **One host agentctl (not per-agent-type).** A single instance that gets re-used for all ACP agents. Rejected because probes would either have to serialize or share subprocess state; per-agent-type gives natural isolation and cheaper refresh.
