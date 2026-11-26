---
created: 2026-11-25
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
  - ../../specs/platform/system-design/durable-delivery-failure.md
legacy_specs: []
---

# Delivery writer failure recovery

## Outcome

Show the cause of a blocked delivery write when agentctl has evidence. Preserve the
same diagnostic in backend logs. Bound automatic reconciliation and correct the
running indication after an exact cancelled turn settles.

`Agent output → agentctl journal → backend → chat`

The agent can keep running after a journal write fails. The backend uses the
existing failure snapshot to report delivery health. It uses a projected
terminal record to decide what happened to the turn.

## Evidence

The reporter observed a sticky writer failure, a generic storage notice, and a
SQL submission marked `cancelled/cancelled` while its session stayed `RUNNING`.
The status warning `delivery_writer_failed` covers most sticky failure codes.
The installed release leaves `MaxStreamBytes` and `MaxJournalBytes` unset.
The event count and filesystem sample leave the original write error undetermined.
[GitHub issue #5513](https://github.com/kdlbs/kandev/issues/5513).

| Observed boundary on `85c41ef5373d` | Reproduction and result |
| --- | --- |
| agentctl pressure wait | A test stream limit paused an append even with ample disk space. The existing ACK path let the writer continue. |
| agentctl sticky failure | A same-prompt secondary error replaced an existing `stream_limit_reached` snapshot. A host error write also lost its tracked submission ID when the journal was closed. |
| launcher relay | A child record with `stream_limit_reached` lost `reason`. Its valid byte fields survived when the child included them. |
| automatic reconciliation | Eight fake-time polls with one revision, 11 seconds apart, started eight reconciliation cycles. |
| session state | The recovery handler left a real SQLite fixture in `RUNNING` after it received an event for an exact projected and settled cancellation with an idle owner. |
| composer | A rendered component with ample free space and absent typed evidence selected the disk-cleanup copy. |

The reproductions use local fixtures. The failed operation and exact journal
state on the reporter's Mac remain unknown.

## Scope

- Preserve validated evidence for a recorded failure in the existing status snapshot and log relay.
- Bound automatic cycles and reuse the exact terminal settlement path.
- Use the existing chat notice and recovery controls on desktop and phone.

### Related scope

Capacity waits use the current ACK path. Delivery limits and display budgets
follow the current durable-delivery contracts. The public recovery guide documents
the stock journal and stream limit defaults.
[Sessions and review](../../public/sessions-and-review.md).

## Implementation order

1. Preserve and log the storage evidence at the rejected operation.
2. Bound automatic recovery and connect the already-cancelled branch to
   existing session convergence.
3. Use the evidence in the chat notice and verify both viewports.

## ASCII UI preview

**UI-01 Missing cause**, in the task chat footer above the composer.

```text
Current
Delivery is blocked on storage   Disk 65.4% used   [Storage] [Details] [x]
Details   Free space, then resume or retry the session.

Proposed
Delivery is blocked                                         [Details] [x]
Details   Kandev could not determine the cause.
          Check the runtime logs or use session recovery.
```

**UI-02 Known delivery limit**, in the same footer.

```text
Delivery is blocked: stream limit reached                   [Details] [x]
Details   Inspect retained output and delivery progress before resuming.
```

Both viewports use the same row and in-place disclosure. On phones the headline
wraps and the two controls stay visible with 44 px touch targets. The existing
chat footer owns the scroll region. Details is the primary action in these
states. An independent high-usage advisory can appear in Details with its own
label. Wording and spacing in the sketch are illustrative. The cause, controls,
and touch reachability are required.
[AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.11](../../specs/platform/requirements/delivery-storage-diagnostics.md).

## Evidence by work order

| Requirement | Targeted proof |
| --- | --- |
| Storage diagnostics 001.7, 002.1, 002.5 and diagnostic signal 001.5 | Go tests exercise failed-write snapshots, runtime projection, and the launcher relay. |
| Durable delivery 005.2, 006.2, 006.4 and 006.5 | Fake-time monitor tests and real-repository orchestrator tests cover a stable failure, late terminal, cancelled proof, and live successor. |
| Storage diagnostics 003.1, 003.6, 003.8 and 003.11 | The existing chat component test and desktop and phone storage-diagnostics specs cover both previews and the physical-capacity control case. |
| Durable delivery 006.2 and 006.5 | The existing resilient-output-delivery specs cover Retry connection, Stop, reload, and a distinct prompt after verified settlement. |

## Work orders

- [x] [01 Preserve storage evidence](task-01-preserve-storage-evidence.md)
- [x] [02 Bound recovery and converge cancelled sessions](task-02-converge-writer-recovery.md)
- [x] [03 Match chat guidance to the evidence](task-03-match-storage-guidance.md)

## Implementation verification

All three work orders passed their targeted checks. The backend build and web
production build passed through the managed desktop E2E run. Targeted Go tests,
the component suite, desktop and phone E2E, web typecheck, localization checks,
and public-doc validation passed. PostgreSQL-specific cases were not run because
`KANDEV_TEST_POSTGRES_DSN` was not configured.

An earlier full changed-Go scan timed out while analyzing the affected
packages. The final changed-file lint passed with zero issues, as did the
focused `cyclop` check for the affected process package. `gofmt` reported no
changes, and the targeted backend tests, race test, and production build
passed.

## Risks

- An untyped follow-up can hide a useful original refusal. Scope retained
  evidence to the same write failure and owner.
- A cancelled row can coexist with an active successor or incomplete
  projection. Use the exact terminal and the existing transactional checks.
- A settled recovery event can arrive while its cancellation guard is busy.
  Its one lifecycle-bounded follow-up must retain the exact terminal and
  recheck current ownership and successors after the guard becomes available.
- Older peers can supply only state. They need the neutral fallback and remain
  eligible for explicit recovery.

## Verification results

All three work orders passed their targeted checks. The commands below record
the product, browser, localization, specification, and documentation checks
that passed. PostgreSQL-specific cases were not run because
`KANDEV_TEST_POSTGRES_DSN` was not configured. An earlier full changed-Go scan
timed out; the final changed-file scan and the focused cyclop check passed
with zero issues.

Backend regression and recovery tests passed:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle -run 'Test(DeliveryStorage|DeliveryFailure|PipeOutput)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(DeliveryHealth|DeliveryReconcile|DeliverySessionState|DeliveryWriterFailure|AgentctlDisconnect)' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'Test(SQLiteDeliverySessionState|DeliverySessionStatePostgres)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process -run '^(TestDeliveryHealthRetriesUnstartedFailureAfterCancelledStop|TestDeliveryStorageSecondaryErrorPreservesSnapshot)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor.go apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor_test.go apps/backend/internal/agentctl/server/process/delivery_failure.go apps/backend/internal/agentctl/server/process/delivery_failure_storage_test.go
```

The storage notice component suite passed 19 tests. Desktop E2E passed 8
tests; phone E2E passed 6 tests. Web typecheck, targeted Oxlint, localization,
and public-documentation validation passed. The public-doc test suite passed
63 tests, and its validator checked 56 pages:

```bash
pnpm --dir apps/web exec vitest run components/task/chat/session-delivery-storage-notice.test.tsx
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/delivery-storage-diagnostics.spec.ts tests/session/resilient-output-delivery.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts tests/session/mobile-resilient-output-delivery.spec.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec oxlint components/task/chat/session-delivery-storage-notice.tsx components/task/chat/session-delivery-storage-notice.test.tsx
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:pseudo
pnpm --dir apps/web run i18n:check
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The listed specification and diff gates were rerun after the review updates
and passed.
