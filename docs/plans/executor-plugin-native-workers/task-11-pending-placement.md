---
id: "11-pending-placement"
title: "Queued and pending placement and refused provision (M3)"
status: pending
wave: 7
depends_on:
  - "10"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.1
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.2
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.3
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.4
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.5
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.6
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.7
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.8
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.1
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.2
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.3
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.4
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
---

# Task 11: Queued and pending placement and refused provision (M3)

## Summary

Add the pending placement phase, the poller, cancel while queued, distinct failure outcomes, restart resume, the session status label, and the refused-provision outcome that settles a record without recovery.

## Size

L: ~600 source, ~1000 test lines including web and Playwright.

## In scope

- `PollExecutorPlacement` and `CancelExecutorPlacement`; `placement` fields in the envelope; per-operation poller and deadline.
- Cancel under the claim with `cancelled` or `already_placed`; no ready publication after cancelling is committed.
- Status label with reason and phase in the web app, localized in the seven catalogs, with a Playwright spec on the fixture.

## Out of scope

- Preemption, resizing, cross-provider failover.

## Acceptance

- A queued launch can be cancelled, resumes after a restart, and ends in a distinct outcome; no allocation remains after cancel.
- Providers without the capability keep the synchronous launch.

## Failing-first tests

- Queued launch cancelled while placement completes (barrier e2e; sqlite and PostgreSQL race).
- Restart while pending resumes polling without a second environment.
- Deadline with a fake clock (synctest); a pending response without the capability is `conflict`.
- Refusal racing cancel or archive (sqlite and PostgreSQL race); refusal after a checkpoint is `conflict`; a lost reply is never a refusal.
- Playwright: queued, cancel while queued, refused (no sleeps).
- `pnpm run i18n:check` passes in seven languages.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

```bash
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/web (session status label)`
- `apps/web/e2e`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
