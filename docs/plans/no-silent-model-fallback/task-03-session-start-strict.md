---
id: task-03-session-start-strict
title: Session start applies start-model policy (strict/fallback/legacy)
status: done
wave: 1
depends_on: [task-01-profile-fields]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 3 — Session start: strict model application

## Change

The profile-model application block in
`apps/backend/internal/agent/runtime/lifecycle/session.go`
(`InitializeSession` flow, `if profileModel != "" && execution.agentctl !=
nil`) is currently best-effort (Warn + continue). Replace with the
policy-driven logic, using the session's advertised model list
(`execution.GetModelState()` after `InitializeSession`) and the profile's
`AutoFallback` / `FallbackModel`:

1. `AutoFallback` ON → keep today's warn-and-continue (legacy).
2. Start model **gone** (advertised list non-empty and model absent):
   - `FallbackModel` set → `SetModel(fallbackModel)`; on success record an
     explicit "using fallback model Y because X is unavailable" signal (log
     + extend the model state or emit an event the UI renders); on failure
     → fail session init explicitly.
   - else → fail session init explicitly with the actionable message from
     the spec's error mapping ("start model X is no longer available —
     change the model in the profile or configure a fallback").
3. Start model present → `SetModel(model)`; on error:
   - `sessionmodel.IsMethodNotFound(err)` (agent has no model selection) →
     continue silently (unchanged no-op).
   - else → strict + fallback-model modes: fail explicitly; auto-fallback:
     warn + continue.

The explicit failure must surface as the session/run error message (chat +
run detail). Shared helper `applyStartModelPolicy(...)` extracted so the
context-reset re-application paths
(`reapplySessionModelAfterReset` / `effectiveSessionModelForReset` in
`manager_interaction.go`) use the same policy — a context reset must not
silently drop a gone model either.

The manager needs the profile's `AutoFallback` / `FallbackModel`: resolve
them alongside `resolveProfileSessionConfig` (extend it or add a
`resolveProfileFallbackConfig`).

## Acceptance

1. Test: strict mode, start model gone → session init fails with the
   actionable message; no session starts.
2. Test: fallback-model mode, start model gone → fallback model applied;
   "using fallback" signal recorded; fallback application failure → init
   fails.
3. Test: auto-fallback mode → legacy warn-and-continue (no failure).
4. Test: model present + `SetModel` error that is NOT method-not-found →
   init fails (strict + fallback modes).
5. Test: `-32601` / `MethodNone` error → continues silently.
6. Test: context reset with a gone model follows the same policy (strict →
   error, not silent re-apply).

## Verification

```sh
make -C apps/backend test ./internal/agent/runtime/lifecycle/...
make -C apps/backend test ./internal/agentctl/sessionmodel/...
```

## Risks

- The advertised list can be empty (agent never sent `models_updated`):
  treat empty list as "unknown" — fall back to `SetModel` result only.
- Do not break the existing model-application happy path (finalConfigID /
  config-option interplay in the same block).
- The "using fallback" signal must be visible: pick the lightest reliable
  channel (extend `CachedModelState` with an `AppliedFallbackModel` field
  surfaced via the existing session models WS event, or a dedicated
  `session.model_fallback` event) — check what the frontend `session-models`
  handler already renders before adding a new event type.
