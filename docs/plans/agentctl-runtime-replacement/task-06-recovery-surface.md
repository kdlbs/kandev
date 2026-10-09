---
id: "06-recovery-surface"
title: "Expose authorized retry and prove desktop/mobile recovery"
status: done
wave: 6
depends_on: ["05-session-reconciliation"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.6
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.7
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.8
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.9
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 06: Expose authorized retry and prove desktop/mobile recovery

## Summary

Expose authorized retry and prove desktop/mobile recovery. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Add the asynchronous system admin recovery route and revisioned boot/WebSocket payload from the design.
Reject stale boot/epoch requests, coalesce concurrent retries, and enforce the existing admin middleware.
Keep the recovery capability distinct from full-backend supervisor restart support.
Update shared store/service/hook types and the existing in-flow runtime alert.
Do not reuse a changed-boot restart progress wait for child recovery. Keep session uncertainty visible after global recovery succeeds.
Use six locale catalogs and the existing Traditional Chinese generation command.
Extend docs/public recovery guidance and root/scoped AGENTS ownership/metrics. Reconcile all affected old monotonic-availability tests and comments.
Prove actual child death and replacement through isolated E2E fixtures; a synthetic status event alone is insufficient.
Cover retries, non-admin viewers, late hydration, hidden status bar, remote-session independence, and preserved page/input state.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeRecoverAuthorization; TestRuntimeRecoverStaleEpoch; TestRuntimeRecoverIdempotentRequest.
Add hooks/domains/system/use-agent-runtime-recovery.test.ts for the new shared action hook. Extend availability, gateway replay, store, and alert component tests with out-of-order HTTP/WS responses.
Add agent-runtime-replacement.spec.ts and mobile-agent-runtime-replacement.spec.ts.
Assert unchanged backend boot_id, fresh authenticated child, no duplicate prompt, stable transcript, usable Stop, and explicit uncertain session state.
Recovery success must permit a new independent local session without reloading the page.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## ASCII UI preview

UI-01: Global runtime alert above the route content; existing alert is the layout exemplar.

```text
Desktop recovering:
[Recovering agent runtime... Saved work is retained.]
[Existing route/chat remains usable]

Desktop exhausted (administrator):
[Agent runtime unavailable] [Retry agent runtime] [Restart Kandev*]
[Session: Outcome uncertain. Review before continuing.] [Stop]

Phone exhausted:
[Agent runtime unavailable]
[Saved work is retained.]
[Retry agent runtime]
[Restart Kandev*]
[Session: Outcome uncertain.]
[Stop]
```

The restart fallback appears only when authorized and supported; it is not the primary recovery action.
Non-admin users see status and administrator guidance without runtime mutation controls.
During recovery the retry action is unavailable; Stop remains session-scoped and reachable.
After runtime success the global alert clears; an unresolved session keeps its own notice.
Labels are illustrative localized copy. Requirements are hierarchy, action semantics, and preserved state.
Reuse the shared view model and in-flow alert, not a drawer or a second bottom bar.
Phone controls stack with 44px targets; desktop uses existing 28px controls.
Keep one route scroll owner, safe-area clearance, keyboard focus, and no horizontal overflow.
Use a polite status announcement while recovering and role=alert for persistent failure.
This covers AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-001.6 through 001.8 and 003.5.

See the [combined preview](plan.md#ascii-ui-preview).

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/system/... ./internal/backendapp ./internal/gateway/websocket -count=1)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/app-status-bar/agent-runtime-unavailable-alert.test.tsx lib/ws/handlers/system-events.test.ts lib/state/store.test.ts hooks/domains/system/use-agent-runtime-recovery.test.ts)
(cd apps/web && pnpm lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/agent-runtime-replacement.spec.ts tests/layout/agent-runtime-unavailable.spec.ts tests/session/agent-survival-restart.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-agent-runtime-replacement.spec.ts tests/layout/mobile-agent-runtime-unavailable.spec.ts tests/session/mobile-agent-survival-restart.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/system/system.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/gateway/websocket/system_notifications.go`
- `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.tsx`
- `apps/web/lib/types/agent-runtime.ts`
- `apps/web/lib/ws/handlers/system-events.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/src/locales`
- `docs/public`
- `AGENTS.md`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

Task 05: Recover durable session evidence after runtime replacement.

## Risks

Delayed callbacks and partial binding can affect a successor. Prove generation ownership before every state-changing result.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md).
- [Design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).

## Results

Completed on 2026-09-27.

- Runtime retry remains an admin-only, revision-fenced action. The desktop and phone surfaces preserve the open route and session controls after a real local child exit.
- A durable uncertain outcome stays visible after global runtime recovery, including when the session safely settles to `WAITING_FOR_INPUT`; Stop remains available and no prompt is resent.
- Passed the W06 backend race command, six focused web test files (36 tests), full web lint, TypeScript typecheck, i18n check, and new-copy ratchet.
- Real-child E2E passed: Chromium desktop 4/4 and mobile Chrome 3/3, including runtime replacement, unavailable-state recovery, and session survival across graceful backend restart.
- PostgreSQL conformance and native Windows/macOS process tests remain release gates. Linux browser and backend validation do not cover those platforms.
