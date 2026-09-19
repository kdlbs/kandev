---
id: "09-recovery-presentation"
title: "Recovery presentation"
status: done
wave: 8
depends_on: ["05-kanban-recovery-convergence", "06-utility-policy-integration", "07-office-policy-convergence", "08-dynamic-policy-settings-ui"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 09: Recovery presentation

- **Acceptance:** Publish and render classified class, safe cause, retry
  ordinal, absolute deadline, and exhausted outcome; expose generation-fenced
  Retry now, Skip now, Cancel wait, and Stop actions in task and Office
  surfaces; and retain logical profile identity and immutable execution
  attribution.
- **Files likely touched:** backend route snapshot/event DTOs and WebSocket
  handlers, `apps/web/lib/{ws,state,types}/**`, task chat dynamic recovery
  components, Office run/detail surfaces, locale catalogs, and focused tests.
- **Dependencies:** Tasks 05, 06, 07, and 08.
- **Parallelism:** sequential contract and presentation integration.
- **Inputs:** Provider Error Recovery UI, API, and State machine; Dynamic Agent
  Routing Route events and chat continuity; existing
  `dynamic-route-recovery.tsx` and route-generation state.
- **Output contract:** Report event fields, stale-action behavior, desktop and
  phone surfaces, countdown ownership, accessibility announcements, files
  changed, exact commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/orchestrator/... ./internal/backendapp/... ./internal/office/... && cd ../../apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/task lib/ws lib/state && pnpm --filter @kandev/web run typecheck`
- **Risks:** Browser countdowns are presentation only. Unsanitized provider
  text must not enter event payloads, translations, or route rows.

## Results

Completed. Task-session route snapshots and WebSocket state events now expose
safe error class/code, catalogue version, retry ordinal, absolute deadline,
and pending outcome. Recovery actions are generation-fenced and use one route
operation for Retry now, Skip now, Cancel wait, and Stop. The browser renders
the authoritative deadline and does not own retry timers; controls retain
mobile-sized targets.

Verification:

- `go test -tags fts5 ./internal/orchestrator/... ./internal/backendapp/...` — 3,085 passed.
- Focused recovery/settings web tests — 24 passed.
