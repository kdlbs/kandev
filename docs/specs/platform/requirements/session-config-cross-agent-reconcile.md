---
status: active
system: platform
created: 2026-08-19
owners:
  - kandev
---
# Session Config Reconciliation Across Agent Types Requirements

## Overview

A task session persists its runtime configuration options globally (model, mode,
and provider-defined options like reasoning effort). When a session that ran
earlier under one agent type is resumed under a different agent type, options
that only the earlier agent understood are replayed onto the new agent. The new
agent rejects them, the user sees a startup warning, and the model/mode selector
disappears from the chat input, leaving the user unable to change model or mode
for the resumed session.

## Requirements

### REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001: Session Config Reconciliation Across Agent Types

**Intent:** A task session persists its runtime configuration options globally (model, mode, and
provider-defined options like reasoning effort). When a session that ran earlier under one agent
type is resumed under a different agent type, options that only the earlier agent understood are
replayed onto the new agent. The new agent rejects them, the user sees a startup warning, and the
model/mode selector disappears from the chat input, leaving the user unable to change model or mode
for the resumed session.

#### Acceptance criteria

- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.1:** On session startup and resume, Kandev SHALL only apply persisted runtime config options that the currently running agent advertises. Options the current agent does not advertise are dropped, not sent to the agent.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.2:** Dropping an unsupported option SHALL NOT be surfaced to the user as a failure warning. The startup config warning is reserved for options the agent *does* advertise but fails to apply.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.3:** Persisted options that the current agent still advertises SHALL continue to be applied unchanged, including provider-defined options such as reasoning effort, context, and fast.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.4:** The chat model/mode selector SHALL render for a resumed session whenever the current agent has reported its model list and config options, even if the session's persisted metadata still records option keys the current agent does not advertise. Un-advertised persisted keys SHALL NOT be treated as required for the selector to render.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.5:** Switching model within a single agent (where the agent re-advertises a different option set for the new model) continues to work through the existing live `session.models_updated` path and is unchanged by this repair.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.6:** Backend: the startup/resume replay filters persisted runtime config options against the current agent's advertised option catalog before sending them, so unsupported keys are neither applied nor reported as failures. The catalog is the same information used by the existing `sanitizeRuntimeConfigOptionsWithCatalog` reconciliation on the reset/restore path.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.7:** Frontend: `requiredConfigKeys` / `hasCompleteDynamicConfig` do not require a persisted key that the current agent's advertised config options (`sessionModelsData.configOptions`) do not include. The selector renders from the agent's live advertised options.
- **AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.8:** **GIVEN** a session whose persisted `runtime_config.config_options` contains `effort` and `thinking` (written by a prior agent) plus `model`, `reasoning`, and `fast`, **WHEN** the session is resumed under an agent that advertises only `mode, model, context, reasoning, fast`, **THEN** the backend applies `reasoning` and `fast`, does not call `SetConfigOption` for `effort` or `thinking`, and emits no "could not be applied at startup" warning for them.

## Migrated source detail

## Why

A task session persists its runtime configuration options globally (model, mode,
and provider-defined options like reasoning effort). When a session that ran
earlier under one agent type is resumed under a different agent type, options
that only the earlier agent understood are replayed onto the new agent. The new
agent rejects them, the user sees a startup warning, and the model/mode selector
disappears from the chat input, leaving the user unable to change model or mode
for the resumed session.

## What

- On session startup and resume, Kandev SHALL only apply persisted runtime
  config options that the currently running agent advertises. Options the
  current agent does not advertise are dropped, not sent to the agent.
- Dropping an unsupported option SHALL NOT be surfaced to the user as a failure
  warning. The startup config warning is reserved for options the agent *does*
  advertise but fails to apply.
- Persisted options that the current agent still advertises SHALL continue to be
  applied unchanged, including provider-defined options such as reasoning
  effort, context, and fast.
- The chat model/mode selector SHALL render for a resumed session whenever the
  current agent has reported its model list and config options, even if the
  session's persisted metadata still records option keys the current agent does
  not advertise. Un-advertised persisted keys SHALL NOT be treated as required
  for the selector to render.
- Switching model within a single agent (where the agent re-advertises a
  different option set for the new model) continues to work through the existing
  live `session.models_updated` path and is unchanged by this repair.

## Broken behavior (regression being fixed)

1. Backend replay path `applyRuntimeSessionLayers`
   (`apps/backend/internal/agent/runtime/lifecycle/session.go`) sanitizes
   persisted options only with `profileconfig.SanitizeConfigOptions`, which
   strips `model`/`mode`/blank keys but keeps every other key. Keys from a prior
   agent (e.g. `effort`, `thinking`) are sent via `SetConfigOption`, the agent
   errors, and each failed key is collected into the `failed` list.
2. The orchestrator emits "Some session settings could not be applied at
   startup: effort, thinking." from that `failed` list
   (`apps/backend/internal/orchestrator/event_handlers_streaming.go`).
3. The frontend gate `hasCompleteDynamicConfig`
   (`apps/web/components/task/model-selector.tsx`) treats every key in
   `session.metadata.runtime_config(_overrides).config_options` as required
   (`requiredConfigKeys`). The current agent never advertises `effort`/`thinking`,
   so the gate never passes, `configHydrated` stays `false`, and the selector is
   hidden permanently.

## Desired behavior

- Backend: the startup/resume replay filters persisted runtime config options
  against the current agent's advertised option catalog before sending them, so
  unsupported keys are neither applied nor reported as failures. The catalog is
  the same information used by the existing `sanitizeRuntimeConfigOptionsWithCatalog`
  reconciliation on the reset/restore path.
- Frontend: `requiredConfigKeys` / `hasCompleteDynamicConfig` do not require a
  persisted key that the current agent's advertised config options
  (`sessionModelsData.configOptions`) do not include. The selector renders from
  the agent's live advertised options.

## Data model

No schema change. Existing session metadata keys are unchanged:

- `runtime_config` -> `SessionRuntimeConfig{ model, mode, config_options }`
- `runtime_config_overrides` -> same shape

`config_options` remains a flat `map[string]string` of `{optionId: value}`.
Stale keys already written to metadata are tolerated (filtered on read/apply),
not required to be migrated away.

## API surface

No new HTTP/WS contract. The existing `session.models_updated` WS event and the
existing startup config-warning message are reused. The warning message content
changes only in that it no longer lists keys the current agent never advertised.

## Failure modes

- **Agent config catalog not yet settled at replay time:** if the current
  agent's advertised option catalog is unknown when replay runs, the backend
  MUST fail safe by not sending options it cannot verify are supported, rather
  than sending all of them. It does not emit a spurious warning for options it
  chose not to send.
- **Agent advertises the option but rejects the value:** this remains a real
  failure and is still reported in the startup warning (unchanged).
- **Persisted metadata retains stale keys:** tolerated. The frontend gate and
  backend replay both ignore keys the current agent does not advertise.

## Persistence guarantees

- No change to what survives a restart. Persisted `runtime_config` and
  `runtime_config_overrides` are unchanged on disk.
- This repair does not rewrite or delete persisted stale keys; it makes read and
  apply paths resilient to them. Any future cleanup of persisted stale keys is
  out of scope.

## Scenarios

- **GIVEN** a session whose persisted `runtime_config.config_options` contains
  `effort` and `thinking` (written by a prior agent) plus `model`, `reasoning`,
  and `fast`, **WHEN** the session is resumed under an agent that advertises only
  `mode, model, context, reasoning, fast`, **THEN** the backend applies
  `reasoning` and `fast`, does not call `SetConfigOption` for `effort` or
  `thinking`, and emits no "could not be applied at startup" warning for them.
- **GIVEN** the same resumed session, **WHEN** the frontend evaluates the
  model-selector gate after the agent reports its config options, **THEN**
  `configHydrated` is `true` and the model/mode selector renders.
- **GIVEN** a resumed session whose persisted options are all advertised by the
  current agent, **WHEN** the session resumes, **THEN** all persisted options are
  applied exactly as today and the selector renders (no behavior change).
- **GIVEN** a resumed session where the agent advertises `reasoning` but rejects
  the persisted `reasoning` value, **WHEN** replay runs, **THEN** `reasoning`
  still appears in the startup warning (real failure preserved).
- **GIVEN** a running session under one agent, **WHEN** the user switches model
  and the agent re-advertises a different option set, **THEN** the live
  `session.models_updated` path updates the selector as it does today (unchanged).

## Out of scope

- Migrating or deleting stale option keys already persisted in session metadata.
- Changing how provider option catalogs are discovered or the sessionless
  resolver in `docs/specs/agents/requirements/dynamic-provider-options.md`.
- Per-model nesting of persisted config options.
- Any change to the live in-session `session.models_updated` protocol.

## Open questions

- None.
