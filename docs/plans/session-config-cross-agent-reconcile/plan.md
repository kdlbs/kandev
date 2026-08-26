---
spec: docs/specs/platform/requirements/session-config-cross-agent-reconcile.md
created: 2026-08-19
status: draft
---

# Implementation Plan: Session Config Reconciliation Across Agent Types

## Overview

Two coordinated, independently verifiable changes fix a cross-agent config
contamination bug. Backend first: the startup/resume replay path filters
persisted runtime config options against the current agent's advertised catalog
before sending them, so options a prior agent wrote (e.g. `effort`, `thinking`)
are neither applied nor reported as startup failures. Frontend second: the chat
model-selector render gate stops treating persisted-but-unadvertised option keys
as required, so the selector renders for the resumed session. The two tasks are
disjoint (Go vs TS, no shared file) and each carries its own regression test.

## Confirmed root cause

- Backend replay `applyRuntimeSessionLayers`
  (`apps/backend/internal/agent/runtime/lifecycle/session.go:602-651`) sanitizes
  persisted options only with `profileconfig.SanitizeConfigOptions`, which keeps
  every non-model/mode key. `SetConfigOption("effort"/"thinking", …)` fails on an
  agent that does not advertise them; each key is added to `failed`.
- The orchestrator turns `failed` into the user warning
  (`apps/backend/internal/orchestrator/event_handlers_streaming.go` around the
  `warnWorkflowSessionConfig` call).
- The frontend gate `hasCompleteDynamicConfig` requires every key from
  `requiredConfigKeys` (which reads `session.metadata.runtime_config(_overrides)
  .config_options`); un-advertised keys keep `configHydrated=false` and hide the
  selector (`apps/web/components/task/model-selector.tsx:90-133`).

The catalog-aware helper `sanitizeRuntimeConfigOptionsWithCatalog(options,
execution.GetModelState())` already exists and is used on the reset/restore path
(`apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go:627-648,
696`). The startup replay path simply does not use it.

---

## Backend

### Runtime replay layer (`internal/agent/runtime/lifecycle/session.go`)

Change `applyRuntimeSessionLayers` so that, after the runtime model is applied
(the `SetModel` block at lines 617-630), the persisted `runtimeConfigOptions`
are filtered against the current agent's advertised catalog before the
`SetConfigOption` loop.

- Replace the `sanitizedOptions := profileconfig.SanitizeConfigOptions(
  runtimeConfigOptions)` call at line 638 with a catalog-aware filter that reuses
  the existing `sanitizeRuntimeConfigOptionsWithCatalog(runtimeConfigOptions,
  execution.GetModelState())` from `manager_interaction.go`. That helper already
  drops model/mode/blank keys (via `isRestorableRuntimeConfigOption`) and, when
  the catalog is known, drops any key not present in the agent's advertised
  option set. When the catalog is not yet known it returns the restorable set
  unfiltered — acceptable and unchanged from today for same-agent resumes.
- Only keys surviving the filter reach `SetConfigOption`, so unsupported prior-
  agent keys never enter the `failed` slice and never reach the warning.
- Fail-safe note from spec: because the model is applied first in this same
  function and `waitForFreshSessionModelState` runs before the layers
  (`session.go:371-374`), the catalog is normally settled by replay time for the
  resuming agent.

Do not change `applyProfileSessionLayers`; profile options come from the current
agent's own profile and are not the cross-agent contamination source. Keep
`profileconfig.SanitizeConfigOptions` there.

If reusing `sanitizeRuntimeConfigOptionsWithCatalog` across files is awkward
(it lives in `manager_interaction.go`, same package `lifecycle`), call it
directly — both files are in package `lifecycle`, so no export change is needed.

---

## Frontend

> User-facing: the chat model/mode selector visibility.

### `apps/web/components/task/model-selector.tsx`

Make the render gate resilient to persisted-but-unadvertised keys. Two options;
plan picks the narrowest:

- Keep `requiredConfigKeys` as the record of "what the session has configured",
  but change `hasCompleteDynamicConfig` so a required key sourced only from
  persisted `runtime_config(_overrides)` is not blocking when the current agent
  has settled its config options and does not advertise that key. Concretely:
  the `required.every(...)` check (lines 127-132) treats a key as satisfied when
  it is advertised (`available.has(key)`) OR it is a persisted-metadata-only key
  absent from the settled agent catalog. The agent catalog is considered settled
  using the existing `sessionModelsData.configOptionsSettled === true` signal
  already read in this file (line 126) and/or a non-empty `configOptions`.
- Keys that come from the current agent profile snapshot / matched profile
  (`session.agent_profile_snapshot.config_options`, profile `configOptions`)
  remain required as today; only the persisted-runtime-only keys become
  non-blocking. This preserves the existing legacy-agent and flat-model-list
  exceptions (lines 124-131).

Net effect: for the resumed Cursor session, `effort`/`thinking` (persisted-only,
not advertised, catalog settled) no longer block, `configHydrated` becomes
`true`, and the selector renders from the agent's live advertised options.

### State / API client

No store, hook, or API-client change. `sessionModelsData` and its
`configOptions` / `configOptionsSettled` fields already exist.

---

## Tests

- **Backend unit (replay filter):** a table-driven test on
  `applyRuntimeSessionLayers` (or a focused test on the shared
  `sanitizeRuntimeConfigOptionsWithCatalog` usage from the runtime path) proving
  that given persisted options `{reasoning, fast, effort, thinking}` and an agent
  catalog advertising `{reasoning, fast, context}`, only `reasoning` and `fast`
  are sent and `effort`/`thinking` are not in the returned `failed` slice. Real
  failure (advertised key, `SetConfigOption` returns error) still lands in
  `failed`. File: `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
  (new test in a new file if the existing one is near the 800-line limit).
- **Frontend unit (gate):** extend the existing `requiredConfigKeys` /
  `hasCompleteDynamicConfig` tests to assert that a persisted-only, unadvertised
  key with a settled agent catalog yields `configHydrated=true`, while a key that
  the agent profile snapshot requires and the agent has not advertised still
  yields `false`. File: the existing model-selector test
  (`apps/web/components/task/model-selector.test.ts` or `.test.tsx` if present;
  otherwise add beside the component per repo test conventions).

No integration or E2E is planned: the observable outcome is unit-testable on both
sides, and the spec adds no new contract. (If a reviewer wants end-to-end proof,
an E2E resuming a session with stale metadata could be added later; not required
for this repair.)

---

## Verification Results

Both tasks complete.

- **task-01 (backend):** `applyRuntimeSessionLayers` now filters via
  `sanitizeRuntimeConfigOptionsWithCatalog(runtimeConfigOptions, execution.GetModelState())`.
  New regression test `TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions`
  (`session_runtime_layers_test.go`). `go test ./internal/agent/runtime/lifecycle/`
  → ok; changed-file `golangci-lint` → 0 issues; `gofmt` clean.
- **task-02 (frontend):** `hasCompleteDynamicConfig` no longer requires
  persisted-runtime-only keys the current agent does not advertise once the
  catalog is settled. Extended `model-selector.test.ts` (26 passed);
  `typecheck` clean; `lint` 0 warnings; `i18n:ratchet` clean.

---

## Implementation Waves And Parallel Candidates

```
Wave 1 (parallel candidates — user authorization required; disjoint Go vs TS):
- [x] [task-01-backend-replay-catalog-filter](task-01-backend-replay-catalog-filter.md)
- [x] [task-02-frontend-selector-gate](task-02-frontend-selector-gate.md)
```

Default is sequential execution in the primary conversation. The two tasks touch
no shared file, schema, migration, generated contract, or lockfile, so they are
parallel-safe if the user explicitly authorizes parallel work.

## Open Questions

- None.
