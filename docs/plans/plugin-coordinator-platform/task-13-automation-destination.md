---
id: "13-automation-destination"
title: "Native automation delivery to managed conversations"
status: complete
wave: 13
depends_on:
  [
    "01-exact-host-foundation",
    "03-managed-lifetime",
    "05-durable-input",
    "06-workspace-observations",
  ]
plan: "plan.md"
requirements:
  - REQ-OFFICE-PLUGIN-TARGETS-001
acceptance_criteria:
  - AC-OFFICE-PLUGIN-TARGETS-001.1
  - AC-OFFICE-PLUGIN-TARGETS-001.2
  - AC-OFFICE-PLUGIN-TARGETS-001.3
  - AC-OFFICE-PLUGIN-TARGETS-001.4
system_design:
  - ../../specs/office/system-design/plugin-conversation-targets.md
---

# Task 13: Native automation delivery to managed conversations

## Summary

Add a generic managed-conversation automation destination, including native editor and portable binding behavior.

## Implementation checklist

- [x] Add the managed destination mode to shared automation target validation.
- [x] Persist destination identity, ownership, schedule revision, and delivery receipts.
- [x] Add installation-owned Exact Host schedule operations and reauthorize at delivery.
- [x] Dispatch and recover the same occurrence without creating or deleting a task.
- [x] Add desktop/phone destination editing, unavailable repair, and portable rebinding.
- [x] Add the named backend and desktop/phone E2E coverage, docs, and required checks.

## In scope

- Expose exact read/create/update/pause/delete for installation-owned managed-conversation schedules, with separate automation capabilities and shared service validation.
- Extend destination persistence, admission validation, occurrence-key enqueue, receipt observation, and history status.
- Preserve existing target defaults and cleanup ownership; stopping/deleting a schedule cannot delete the shared conversation.
- Add target selector and explicit portable-import rebinding on desktop and phone. Show missing/paused/revoked states and retry with the same occurrence identity.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Exact schedule management respects installation ownership; repeated firing delivery enqueues once and history distinguishes accepted from completed.
- Disable/restart/pause and automation cleanup preserve destination ownership and pending inputs.
- Desktop/phone editing and portable import/export bind the intended instance explicitly.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-04: Automation destination

```text
Desktop: automation editor > target
+------------------------------------------------------------+
| Target: [Managed conversation v]                           |
| Plugin: Coordinator   Instance: [Delivery lead v]           |
| Schedule: [Weekdays 09:00]                                  |
| Prompt:   [Summarize blocked work...]                       |
|                                               [Save]      |
| Last firing: Accepted. Waiting for conversation            |
+------------------------------------------------------------+
Phone: automation editor
+------------------------------+
| < Automation          [Save]|
| Target                      |
| [Managed conversation v]    |
| Instance                    |
| [Delivery lead v]           |
| Schedule                    |
| [Weekdays 09:00]            |
| Prompt                      |
| [Summarize blocked work...] |
| Accepted; not yet running   |
+------------------------------+
```

Use a full-height phone editor and bottom-drawer instance selector. Hide task-only repository/workflow fields for this destination. An unavailable target remains visible with Repair action. History separates delivery from agent outcome.

Applicable criteria: `AC-OFFICE-PLUGIN-TARGETS-001.1`, `AC-OFFICE-PLUGIN-TARGETS-001.2`, `AC-OFFICE-PLUGIN-TARGETS-001.3`, `AC-OFFICE-PLUGIN-TARGETS-001.4`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/automation -run 'TestManagedConversationDestination|Test.*Target|Test.*Cleanup' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/managed-automation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-managed-automation.spec.ts)
```

Evidence to create:

- `apps/backend/internal/automation/managed_destination_test.go`: `TestManagedConversationDestination`.
- `apps/web/e2e/tests/plugins/managed-automation.spec.ts`: `schedule delivery, cleanup and portable rebinding`.
- `apps/web/e2e/tests/plugins/mobile-managed-automation.spec.ts`: `phone destination editor`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/automation/`
- `apps/web/components/automations/`
- `apps/web/src/locales/`
- `apps/backend/internal/plugins/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 03](task-03-managed-lifetime.md)
- [Task 05](task-05-durable-input.md)
- [Task 06](task-06-workspace-observations.md)

## Risks

Existing automation cleanup owns disposable runs. A shared conversation is only a destination reference and must never become cleanup-owned.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/plugin-conversation-targets.md) and [design](../../specs/office/system-design/plugin-conversation-targets.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented native scheduling to retained managed conversations, installation-owned Exact Host schedule APIs, revisioned schedule updates, durable occurrence delivery receipts, retry-safe dispatch, destination repair, and portable YAML rebinding on desktop and phone. A dispatch E2E exposed that the automation run-list SQL projection omitted the managed receipt columns; added the fields and a regression test. Durable input admission now returns before queue wake-up completes, with a race regression proving the boundary.

Validation passed: `make build`; `make e2e-plugin-package`; automation destination/target/cleanup and managed receipt projection race tests; managed-input queue race tests; 41 focused web tests; web typecheck and i18n checks; desktop and mobile managed automation E2E (one each); public-doc validator tests (62) and all-page validation (47); and `git diff --check`. The mock provider still fails closed at the managed-tool support gate after a durable delivery receipt; no provider is advertised as supporting managed execution without adapter evidence.

### PR fixup (2026-09-27)

Each admitted run now persists its resolved destination identity, instance key,
revision, and conversation. Enqueue retries and receipt reads remain pinned to that
snapshot after a schedule is rebound. Temporary receipt-read failures preserve the
accepted run and do not consume enqueue attempts; receipt recovery can still settle
the run. Automation package tests passed for destination rebinding and repeated
receipt-read failures at the retry limit.
