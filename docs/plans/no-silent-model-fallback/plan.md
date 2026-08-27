# Plan: No Silent Model Fallback

**Spec**: `docs/specs/agents/requirements/no-silent-model-fallback.md`
**Status**: implemented (all tasks done, gate green; PR pending)
**Date**: 2026-08-04

## Summary

Eliminate implicit model fallback. Three per-profile modes (strict /
fallback-model / auto-fallback toggle) control every fallback point:
session-start model application (agentctl runtime), boot reconciliation
(never heal away a gone model), office post-start failure re-dispatch, and
all model/profile pickers in the UI (grey out gone models; block profiles
whose start model is gone unless a fallback is configured).

## Design Decisions

1. **`auto_fallback` wins over `fallback_model`** — the toggle restores
   legacy behavior wholesale; the fallback-model field is hidden/disabled
   when it is ON.
2. **Strict is the default** for existing profiles (`auto_fallback=0`,
   empty fallback). Existing users who relied on automatic fallback must
   opt in via the toggle.
3. **Gone-ness is computed on the frontend**: `configured ∉ advertised`.
   The backend only stops *causing* silent switches (reconciler, runtime
   best-effort, post-start requeue).
4. **Session-start detection** uses the session's advertised model list
   (`execution.GetModelState().Models` after `InitializeSession`) plus the
   `SetModel` result; `-32601`/`MethodNone` ("no model selection support")
   is never treated as availability failure.
5. **One-shot office retry** with the fallback model uses a new nullable
   `runs.fallback_model` override consumed (and cleared) by the next
   dispatch — one extra attempt, then terminal explicit failure. Bypasses
   the resolver; counts against `MaxAttemptsPerRun` like any attempt.

## Waves (dependency order)

### Wave 1 — Backend foundation (profile fields + strict start)

1. `task-01-profile-fields.md` — `fallback_model` + `auto_fallback` on
   agent_profiles: migration, model struct, store, DTO, create/update
   controller, frontend type/normalize round-trip.
2. `task-02-reconciler-keep-gone.md` — `healProfile` keeps a gone start
   model (and gone fallback model); seed-default branch unchanged.
3. `task-03-session-start-strict.md` — agentctl runtime applies the
   start-model policy at session start (+ context-reset path): strict
   fail-explicit, fallback-model apply, auto-fallback legacy. Actionable
   error messages.

### Wave 2 — Office post-start gating

4. `task-04-post-start-gating.md` — `HandlePostStartFailure` gates on
   profile mode: strict → escalate (no requeue); auto-fallback → requeue;
   fallback-model → one-shot forced retry via `runs.fallback_model`
   (Run model + repo + dispatcher), then terminal failure. Error-code →
   actionable message mapping.

### Wave 3 — Frontend

5. `task-05-picker-gone-support.md` — `ModelConfigSelector` disabled
   options; session toolbar keeps + greys the stale active model
   (`session-models.ts`, `model-selector.tsx`).
6. `task-06-profile-editor.md` — start model red/disabled when gone; new
   "Agent fallback" row; "Fallback automatically to next model" toggle
   (hides fallback row); `cli-profile-editor` parity; i18n keys.
7. `task-07-profile-picker-gating.md` — task-create dialog + office setup
   profile pickers block strict-gone profiles (reason tooltip) and warn on
   fallback-mode profiles; `AgentProfileOption` gains model fields.

### Wave 4 — E2E

8. `task-08-e2e.md` — Playwright: gone start model red/disabled in editor,
   blocked profile in task-create picker, fallback warning + toggle
   behavior, on the mock backend.

## Verification Commands

- Backend: `make -C apps/backend test ./internal/agent/settings/... ./internal/agent/runtime/lifecycle/... ./internal/office/scheduler/...` (task-specific `go test` targets are listed per task file).
- Frontend unit: `cd apps/web && pnpm vitest run <file>` per task.
- Typecheck: `cd apps/web && pnpm run typecheck`.
- E2E: `cd apps/web && pnpm e2e` (gated project or targeted spec).
- Full gate: `make fmt && make typecheck test lint` per AGENTS.md.

## Risks

- Behavior change for strict profiles (launch now fails when start model
  gone). Mitigated by actionable error mapping.
- The one-shot office retry must respect `MaxAttemptsPerRun` / route-cycle
  semantics (counts as a normal attempt row).
- Probe cache staleness: acceptable; agent `validateAvailableModel` fails
  fast and the runtime treats that as unavailable.
- i18n ratchet: every added/edited user-facing line must use `t()`.
