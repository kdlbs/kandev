---
spec: docs/specs/agents/requirements/agent-stall-recovery.md
decision: docs/decisions/2026-08-02-agent-terminal-diagnostics-over-stderr.md
created: 2026-08-02
status: done
---

# Implementation Plan: OpenCode Terminal Error Surfacing

## Overview

Enable OpenCode's error-only stderr output, normalize only session-correlated
foreground `stream error` records inside agentctl, and use that terminal signal
to release a prompt whose ACP request never returns. Carry a bounded safe
provider-error record through the existing lifecycle, classify usage-limit
messages, and render a localized recovery card with collapsed technical details
on desktop and mobile. Ambiguous silence continues to use the shipped advisory
stall notice.

---

## Backend

### Managed OpenCode diagnostic output

- Change `OpenCodeACP.ManagedNPMRuntime` in
  `apps/backend/internal/agent/agents/opencode_acp.go` so every managed OpenCode
  ACP entry point uses `acp --print-logs --log-level ERROR`. Because normal
  launch, capability probes, and one-shot inference all consume this shared
  runtime spec, none may construct a divergent OpenCode command.
- Update the command contracts in
  `apps/backend/internal/agent/agents/opencode_acp_test.go`,
  `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`,
  `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`, and
  `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`.

### Non-blocking stderr observation

- Add an optional `StderrLineConsumer` interface beside `StderrProvider` in
  `apps/backend/internal/agentctl/server/adapter/adapter.go`. The interface
  receives an ANSI-stripped line as it is appended to the existing bounded
  stderr ring; it is not a browser API and does not expose historical files.
- Update `Manager.readStderr` in
  `apps/backend/internal/agentctl/server/process/manager.go` to deliver each
  cleaned line to an installed consumer without allowing parsing or channel
  backpressure to block draining the child process. For OpenCode, apply a
  protocol-specific safe projection before generic logging, the recent-stderr
  ring, or process-exit events; malformed or unrelated records are excluded.
  Other managed agents retain their existing stderr behavior.
- Add focused process-manager regressions proving delivery, ANSI stripping,
  non-consumer behavior, and that a nonzero OpenCode exit cannot expose raw
  workspace URLs or identifiers through logs or exit events.

### OpenCode diagnostic normalization and prompt correlation

- Add `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
  with a narrow key/value parser for OpenCode's emitted error records. Accept a
  record only when `level=ERROR`, `message="stream error"`, `small` is not
  `true`, `session.id` is present, and `error.error` is non-empty. Parse the
  timestamp, provider ID, and model ID; strip URLs and identifier-bearing
  suffixes from the error text before constructing the normalized record.
- Add a protocol-neutral safe record in
  `apps/backend/internal/agentctl/types/streams/provider_error.go` and attach it
  optionally to `streams.AgentEvent`. Fields are limited to `source`,
  `provider_id`, `model_id`, `message`, `occurred_at`, and `reset_at`.
- Let the ACP adapter implement `StderrLineConsumer` only for
  `agentID == "opencode-acp"`. Route a validated diagnostic to the current
  prompt only when its provider session ID equals the adapter's active ACP
  session and a foreground prompt turn is still pending. The delivery path must
  be non-blocking and must discard stale lines after the prompt settles.
- Extend the prompt-turn wait in
  `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
  so a correlated terminal diagnostic competes with the ACP RPC and user
  cancellation. Whichever terminal path claims the prompt first cancels the
  losing operation, preserves generation ownership, and emits one outcome.
- Return a typed prompt error carrying the safe provider-error record. Update
  `handleWSPrompt` and `Manager.SendErrorEvent` in
  `apps/backend/internal/agentctl/server/api/agent.go` and
  `apps/backend/internal/agentctl/server/process/manager.go` so the existing
  error event includes those structured details. Do not send a second error if
  the ACP RPC loses the race after the diagnostic has settled the prompt.

### Lifecycle propagation and quota classification

- Add the optional safe provider-error record to `AgentExecution`,
  `AgentEventPayload`, and `watcher.AgentEventData` in
  `apps/backend/internal/agent/runtime/lifecycle/types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, and
  `apps/backend/internal/orchestrator/watcher/watcher.go`. Also add `AgentID` to
  the lifecycle payload so downstream classification uses `opencode-acp`
  rather than inferring an agent from prose.
- In `manager_events.go`, copy structured details from the winning generation's
  error event before `MarkCompleted` publishes `agent.failed`. Do not retain
  details from a stale generation or a later duplicate terminal event.
- Extend `apps/backend/internal/agent/runtime/routingerr/rules.go` so OpenCode's
  exact `usage limit reached` signature classifies as high-confidence
  `quota_limited`. Preserve the existing sanitizer and ensure account/workspace
  URLs and long identifiers never enter `RawExcerpt`.
- In `createRecoveryStatusMessage` in
  `apps/backend/internal/orchestrator/event_handlers_agent.go`, classify the
  failure using the event's `AgentID`, validate the structured provider record,
  and persist the existing recovery actions with metadata:
  `failure_kind: provider_quota_limited`, safe `provider_name`, optional
  `model_id`, optional RFC3339 `reset_at`, and bounded sanitized `error_output`.
  Fall back to the generic recoverable-error card when classification or
  structured correlation is absent.
- Continue using the current `MarkCompleted` -> `agent.failed` ->
  `handleRecoverableFailure` path. Non-Office sessions become
  `WAITING_FOR_INPUT`; Office sessions keep their existing `FAILED` behavior.
  This feature does not invent a second lifecycle state or retry loop.

---

## Frontend

### Localized provider-limit recovery

- Extend `ActionMeta` / `MessageAction` metadata typing in
  `apps/web/components/task/chat/types.ts` and
  `apps/web/components/task/chat/messages/action-message.tsx` for the safe
  provider-limit fields.
- Add a `ProviderQuotaRecovery` branch beside `MissingBranchRecovery`. It uses
  `failure_kind`, not string matching, to render a localized heading naming the
  model when available, localized reset guidance when `reset_at` is present,
  the existing recovery actions, and the existing collapsed **Technical
  details** disclosure for sanitized `error_output`.
- Add all new copy to `apps/web/src/locales/en/chat.json` and
  `apps/web/src/locales/pseudo/chat.json`; resolve chat keys through
  `useTranslation` at render time. Do not translate tokens that are compared
  in control flow.
- Keep the generic settled-error behavior unchanged for unknown provider
  failures and keep the existing neutral running-stall notice unchanged.

### Mobile design contract

- **Desktop outcome:** the active spinner disappears when the correlated error
  settles the turn. A compact error card identifies the affected model and
  reset timing, keeps sanitized technical details collapsed, and exposes the
  existing recovery actions.
- **Mobile entry point and outcome:** the same card appears inline in the task
  chat. No drawer or route is added; quota recovery is short, contextual
  content already owned by the chat transcript.
- **Nearest shipped exemplars:** `MissingBranchRecovery` supplies the localized
  alert-card/details composition, while
  `mobile-transient-retry.spec.ts` and
  `mobile-pause-resume-recovery.spec.ts` supply the phone recovery-action and
  geometry patterns.
- **Hierarchy and geometry:** heading, reset guidance, collapsed details, then
  recovery actions. Chat remains the single scroll owner; technical output has
  its own bounded vertical overflow only when expanded. Mobile actions retain
  the existing full-width 44px touch geometry, and the page gains no horizontal
  overflow.
- **Shared logic:** metadata interpretation, reset-time formatting, details,
  and action handlers are shared across viewports; only responsive classes
  control layout.

---

## Tests

- **What:** every managed OpenCode ACP command enables error-only stderr while
  other managed agents remain unchanged.
  **Files:** `apps/backend/internal/agent/agents/opencode_acp_test.go`,
  `managed_npm_runtime_test.go`, and lifecycle launch tests.
  **How:** exact argv assertions across runtime, probe, and inference builders.
- **What:** process stderr reaches an optional consumer as cleaned live lines
  without changing the bounded recent-stderr contract.
  **File:** `apps/backend/internal/agentctl/server/process/manager_test.go` or a
  focused sibling test.
  **How:** fixture process writes ANSI stderr; assertions cover ordered delivery,
  no consumer, bounded ring behavior, a backlogged diagnostic channel that does
  not block the drain loop, and a nonzero OpenCode exit whose log, ring, and
  exit event contain only the safe projection.
- **What:** only a main-session OpenCode stream error is normalized.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`.
  **How:** table tests cover the observed quota line, quoted fields, URL
  sanitization, `small=true`, wrong level/message, missing fields, malformed
  input, and unrelated session IDs.
- **What:** a correlated diagnostic releases a held prompt exactly once while
  stale/background/wrong-session diagnostics do not.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_test.go`.
  **How:** fake ACP agent holds `session/prompt`; inject stderr through the
  consumer and race it against RPC completion and cancellation.
- **What:** structured provider details survive the lifecycle event boundary
  only for the winning prompt generation.
  **Files:** lifecycle `manager_events_test.go`, event publisher tests, and
  orchestrator watcher tests.
  **How:** direct error events with matching and stale generations.
- **What:** `usage limit reached` becomes high-confidence `quota_limited`, raw
  excerpts are sanitized, and recovery metadata contains only safe model/reset
  fields plus existing actions.
  **Files:** `routingerr/classify_test.go` and
  `orchestrator/event_handlers_test.go` or a focused provider-failure test.
  **How:** table classifier tests plus direct recoverable-failure handler tests.
- **What:** provider quota metadata renders localized model/reset copy,
  collapsed sanitized details, and 44px recovery actions without changing
  generic errors or running stall notices.
  **File:** `apps/web/components/task/chat/messages/action-message.test.tsx`.
  **How:** store-backed rendered component tests using English and pseudo
  locale fixtures.

## E2E Tests

- **Scenario:** given a settled OpenCode quota failure, opening the task shows
  model/reset guidance rather than a running spinner; expanding technical
  details reveals the sanitized message but no OpenCode workspace URL or ID.
  **File:** `apps/web/e2e/tests/session/provider-quota-recovery.spec.ts`.
- **Scenario:** the same provider-limit explanation, details disclosure, and
  recovery actions are usable by touch with no horizontal overflow.
  **File:** `apps/web/e2e/tests/session/mobile-provider-quota-recovery.spec.ts`.
- Seed the settled session and persisted status-message metadata through the
  existing E2E-only task/session/message helpers. Do not require a live quota
  exhaustion or write provider credentials; the real stderr-to-error boundary
  is covered by the agentctl and lifecycle integration tests.

## Verification Results

- Task 01: `cd apps/backend && go test ./internal/agent/agents ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp -run 'Test(OpenCode|ManagedNPMRuntime|Manager.*Stderr|ParseOpenCode)' -count=1` — 24 tests passed.
- Task 02: `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(OpenCodeDiagnostic|Prompt.*Diagnostic|HandleWSPrompt.*ProviderError|SendErrorEvent)' -count=1` — 3 tests passed.
- Task 03: `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agent/runtime/routingerr ./internal/orchestrator ./internal/orchestrator/watcher -run 'Test(.*ProviderError|.*OpenCode.*UsageLimit|HandleRecoverableFailure.*Quota|CreateRecoveryStatusMessage.*Quota)' -count=1` — 13 tests passed.
- Task 04: `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx` — 17 tests passed; `cd apps/web && pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Task 05: managed desktop and mobile Playwright runs passed after the E2E seed was corrected to include `completed_at`: one Chromium test and one `mobile-chrome` test passed.
- Fixup: `cd apps/backend && go test ./internal/agent/agents ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp -run 'Test(OpenCode|ManagedNPMRuntime|Manager.*Stderr|ManagerProcessExit|ParseOpenCode)' -count=1` — 26 tests passed; `go test ./...` passed with 10,237 tests.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation. Each task consumes a
contract introduced by the previous task, so none is marked parallel-safe.

Wave 1:

- [x] [Task 01: Capture OpenCode error diagnostics](task-01-capture-opencode-error-diagnostics.md)

Wave 2:

- [x] [Task 02: Settle prompts from correlated diagnostics](task-02-settle-correlated-opencode-prompt.md)

Wave 3:

- [x] [Task 03: Classify and persist provider failures](task-03-classify-provider-failure.md)

Wave 4:

- [x] [Task 04: Render localized quota recovery](task-04-render-provider-quota-recovery.md)

Wave 5:

- [x] [Task 05: Prove desktop and mobile recovery](task-05-e2e-provider-quota-recovery.md)

## Risks and non-goals

- OpenCode's stderr record is an agent compatibility dialect. Parsing fails
  closed: a format change loses the immediate error but cannot fail an
  unrelated turn.
- The stderr reader must never block, because blocking it can deadlock the
  supervised process; the OpenCode diagnostic channel is bounded and its
  consumer drops records when backlogged.
- Prompt RPC completion, user cancellation, and the diagnostic may race; prompt
  generation ownership and a single terminal claim are correctness boundaries.
- Only the sanitized normalized provider record crosses agentctl. OpenCode raw
  stderr is inspected in memory by its parser but is never written to generic
  logs, the recent-stderr ring, process-exit events, persisted recovery
  metadata, or browser payloads.
- The plan does not read OpenCode log files, add an automatic model switch,
  schedule reset-time retries, change Office routing, or replace the upstream
  ACP fix.
