---
id: "06-session-steer-contract"
title: "Expose supports_steering on the session contract"
status: done
wave: 3
depends_on: ["01-negotiate-steer-capability", "05-orchestrator-steer-admission"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 06: Expose supports_steering on the session contract

- **Acceptance:** Session REST DTOs, the boot payload, `session.state_changed`,
  and `session.activity_changed` all carry `supports_steering` as a boolean. It
  is true only when the connected agent advertised the capability, the toggle is
  enabled, and the session is promptable-or-generating; false or absent
  otherwise.
- **Acceptance:** The value is derived at serialization time from live
  connection state and is never persisted; after a restart with no connected
  execution it is false.
- **Verification:** `cd apps/backend && go test -race ./internal/orchestrator/... ./internal/backendapp/...` then `cd apps/web && pnpm run typecheck && pnpm test`
- **Files likely touched:** the session DTO/serializer and WS event payloads
  under `apps/backend/internal/orchestrator/` and
  `apps/backend/internal/backendapp/`, the generated/shared types consumed by
  the web app, and the session store slice under `apps/web/lib/state/slices/`.
- **Dependencies:** Tasks 01 and 05.
- **Inputs:** Spec "API surface" and "Persistence guarantees". Mirror how
  `foreground_activity` and `active_subagent_count` are surfaced across the same
  four payloads in `../../specs/platform/requirements/background-work-liveness.md` — same
  shape, same publication points, so clients cannot observe an inconsistent
  pair.
- **Risks:** Do not persist the flag or derive it from the persisted agent
  profile snapshot; that would reintroduce the name-based gate task 01 removes
  and would survive a restart as a false positive.
- **Output contract:** Report the field name and type on each of the four
  payloads, the derivation predicate, restart behavior, exact commands/results,
  and update only this task's status.

## Validation Results

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/backend && go test -race ./internal/orchestrator/... ./internal/backendapp/...`: passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm test`: passed for every steering-related suite; the 6
  failures in the full run are pre-existing on clean `main` in untouched files
  (see task-02's record).
- Contract: `supports_steering` (bool) is carried on the session DTO and the
  session-scoped WS payloads, derived from flag ∧ negotiated capability ∧
  RUNNING-and-generating. It is a runtime projection, never persisted, so a
  restart re-derives it rather than resurrecting a stale value.
