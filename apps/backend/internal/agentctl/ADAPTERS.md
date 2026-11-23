## Adapter Model

Protocol adapters in `server/adapter/transport/` normalize different agent CLIs:
- `AgentAdapter` interface defines `Connect()`, `Initialize()`, `NewSession()`, `LoadSession()`, `Prompt()`, `Cancel()`, `Updates()`, `Close()`, among others
- Transports: `acp`, `codexappserver` (native Codex app-server), and `shared` (cross-transport helpers). The native Codex transport is experimental and uses the versioned schema under `pkg/codexappserver/schema/`; do not infer support for other native provider protocols from its protocol types.
- `process.Manager` owns subprocess, wires stdio to adapter
- `NewAdapter` in `server/adapter/factory.go` selects the adapter by protocol (`agent.Protocol`), not by agent type; agent identity and CLI-specific config come from Go constructors in `internal/agent/agents/`, registered by `internal/agent/registry.Registry.LoadDefaults()`

The `acp` transport is split by concern across `adapter_*.go` files: `adapter.go` (core/lifecycle), `adapter_session.go` (initialize/new/load/resume/close), `adapter_prompt.go` (prompt/cancel), `adapter_updates.go` (`session/update` notification fan-out), `adapter_tools.go` (`convertToolCallUpdate` / `convertToolCallResultUpdate` -> normalized payloads), `adapter_permissions.go`, and `adapter_helpers.go`. Agent-specific ACP extensions use the package-private `acpDialect` function table in `dialect.go`; keep observed wire translation in `dialect_<agent>.go`. Dialect hooks return normalized data or request descriptions and never receive `*Adapter` or execute RPCs. Shared capability normalization used by both live sessions and utility probes belongs in `internal/agentctl/acpcompat/`. Tool-call conversion lives in `adapter_tools.go`, not `adapter.go`. See ADR-0043. Session lifecycle transitions are serialized: `NewSession`, `LoadSession`, and `ResetSession` (`session/new` plus the `closeSupersededSessionLocked` cleanup) each run under `Adapter.sessionTransitionMu`, because agentctl dispatches WS requests to the adapter without serialization; any new path that writes `a.sessionID` must hold that mutex for the whole transition, not just the write.

**Prompt handoff and steering are negotiated, not named.** Both the foreground-idle handoff (ADR-0049) and mid-turn steering (ADR-2026-08-04) gate on the agent's `initialize` advertisement `agentCapabilities._meta.claudeCode.promptQueueing`, read once in `prompt_queueing.go` and cached on the adapter — never on `agentID`. A steer transfers the existing prompt-gate token to a human successor while the predecessor `session/prompt` is still open (`adapter_prompt_cancel.go: handOffTurnLocked`), reusing the generation-keyed completion attribution and background-work protection built for handoff. Steering is exposed via the optional `adapter.SteerablePrompter` interface (`PromptSteer` / `SupportsSteering`), so non-steering transports need no change. Delivery is opportunistic: the advertisement asserts the agent accepts a concurrent prompt, not that it folds it — both outcomes must be correct.

### Grok ACP dialect (`dialect_grok.go`)

Grok exposes some model, config, and usage surfaces through implementation-specific ACP metadata. Its dialect normalizes them in the backend so shared ACP paths and the frontend stay provider-agnostic:

| Grok wire | Kandev shape |
|---|---|
| Legacy `models` catalog | model `ConfigOption` used by generic frontend selectors |
| Model `_meta.supportsReasoningEffort` / `reasoningEfforts` / `reasoningEffort` | gated `reasoning_effort` select with category `thought_level` |
| `SetConfigOption(model, …)` or `(reasoning_effort, …)` | Grok has **no** `session/set_config_option`. Rewrite both to `session/set_model` (effort keeps current modelId + `_meta.reasoningEffort`; model switch may carry prior effort in meta when still supported). Frontend always uses set_config_option when a model ConfigOption exists. |
| Mid-session `MODEL_SWITCH_INCOMPATIBLE_AGENT` | return Grok's actionable error; user starts a new session explicitly |
| Notification `_meta.totalTokens` + model `_meta.totalContextTokens` | `EventTypeContextWindow` (used/size/remaining). Compaction may decrease used; clear used on model switch. |
| Prompt `_meta.usage` | input/output/cache/total plus `reasoningTokens` as `PromptUsage.ThoughtTokens`; sibling flat fields are not whole-turn totals |

Grok ACP currently exposes neither per-turn cost nor subscription quota/reset values. Do not estimate them or treat the optional subscription tier label as usage data.
