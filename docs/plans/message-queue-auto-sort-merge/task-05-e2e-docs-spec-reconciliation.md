---
id: "05-e2e-docs-spec-reconciliation"
title: "E2E, docs, and spec reconciliation"
status: done
wave: 4
depends_on:
  - "03-session-compaction-pass"
  - "04-auto-sort-merge-controls"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.2
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.4
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.5
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.11
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.16
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 05: E2E, docs, and spec reconciliation

## Summary

Prove the user flows in the browser on desktop and phone, document
Auto-sort-merge publicly, and reconcile the active queue specifications that
this feature amends. Then promote the requirement and design.

## In scope

- `apps/web/e2e/tests/chat/message-queue-auto-sort-merge.spec.ts`:
  - Setup: a parent task with two child tasks that send interleaved messages
    through `e2e:mcp:kandev:message_task_kandev(...)` while the parent is busy.
  - Assert the A1, B1+B2, A2 merge on admission with the session pill ON.
  - Turn the pill OFF, queue more messages, then turn it ON and assert the
    compaction.
  - Assert that the pill is disabled and rendered OFF when Auto-merge is OFF.
  - Assert that a global change does not compact.
- New `chat/mobile-message-queue-auto-sort-merge.spec.ts` (`mobile-chrome`):
  tap the pill, observe compaction and a later sorted admission, toggle
  Auto-merge to check the disabled-OFF dependency and preference restoration,
  and reload. This is the mobile parity evidence for `.16`.
- Extend `system/message-queue-settings.spec.ts` and
  `system/mobile-message-queue-settings.spec.ts` with the default OFF and the
  dependent saved preference, and `chat/mobile-message-queue-management.spec.ts`
  with touch-target and overflow checks.
- Add an `apps/web/e2e/helpers/api-client.ts` `setQueueAutoSortMerge` helper,
  used for preconditions only.
- Public docs: `docs/public/coordination.md`, `operations.md`,
  `sessions-and-review.md`, `configuration.md` (if it covers queue settings),
  and `websocket-api.md` (action and status fields).
- Specs: amend `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5`, `.6`, `.7`,
  and its no-compaction exclusion with explicit Auto-sort-merge exceptions
  (behaviour with Auto-sort-merge OFF stays unchanged), and amend
  `AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.1` to include the third pill.
  Update `docs/specs/ui/system-design/message-queue-automation-controls.md`
  (admission flow tail-only and full-capacity folding, "policy changes never
  scan pending rows", the global policy snapshot, and the header groups) with
  cross-links to the new design. Link the new requirement from both
  requirements. Then set the requirement to `active`,
  the design to `current`, and the plan to `implemented`.

## Out of scope

- New behaviour.

## Acceptance

- The new and extended specs pass on `chromium` and `mobile-chrome`.
- The existing `message-queue.spec.ts` auto-merge scenarios still pass
  unchanged with the default OFF.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` pass.

## ASCII UI preview

See [plan UI-01, UI-02, and UI-03](plan.md#ascii-ui-preview). This work order
verifies those structures and adds no new UI.

## Verification

```bash
pnpm --dir apps/web run build:e2e
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/message-queue-auto-sort-merge.spec.ts tests/chat/message-queue.spec.ts tests/system/message-queue-settings.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-message-queue-auto-sort-merge.spec.ts tests/chat/mobile-message-queue-management.spec.ts tests/system/mobile-message-queue-settings.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs
```

## Files likely touched

- New `apps/web/e2e/tests/chat/message-queue-auto-sort-merge.spec.ts`
- New `apps/web/e2e/tests/chat/mobile-message-queue-auto-sort-merge.spec.ts`
- `apps/web/e2e/tests/system/{message-queue-settings,mobile-message-queue-settings}.spec.ts`, `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/public/{coordination,operations,sessions-and-review,configuration,websocket-api}.md`
- `docs/specs/ui/requirements/{message-queue-auto-merge-session-overrides,message-queue-automation-controls,message-queue-auto-sort-merge}.md`
- `docs/specs/ui/system-design/{message-queue-auto-sort-merge,message-queue-automation-controls}.md`, `docs/plans/message-queue-auto-sort-merge/plan.md`

## Dependencies

Tasks 03 and 04.

## Risks

- Child-message timing in the mock agent can be flaky. Keep the parent busy
  with a slow mock turn and wait on the queue status, not on sleeps.

## Parallelism

`sequential`

## Inputs

- `apps/web/e2e/tests/chat/agent-message-attribution.spec.ts` (sender-task
  messaging) and `message-queue.spec.ts` (Auto-merge pill flows).

## Results

- `pnpm --dir apps/web run build:e2e`: passed.
- Both exact `e2e:run --host --shards 1` commands above ran through
  `scripts/run-quiet e2e --summary --`: Chromium 23 passed (5.2m),
  `/tmp/kandev-run.e2e.5ZjXqmJD.log`; mobile-chrome 14 passed (3.1m),
  `/tmp/kandev-run.e2e.cGKZJ0hP.log`. No E2E cases skipped.
- Post-lint-refactor replay rebuilt the production E2E assets:
  `/tmp/kandev-run.e2e-build.wa4NVpWx.log`. The exact Chromium command passed
  again: 23 passed (4.9m), `/tmp/kandev-run.e2e.mO64inf3.log`.
- The mobile replay initially reported 13 passed and one flaky retry:
  `/tmp/kandev-run.e2e.O5C7oY4i.log`. The existing Send Now geometry assertion
  sampled a 43.999969px height without awaiting the panel's finite entrance
  animation. Added the existing `waitForFiniteAnimations(panel)` readiness
  helper; both strict `>= 44` assertions are unchanged.
- The same three mobile specs then passed with `-- --retries=0`: 14 passed
  (2.9m), `/tmp/kandev-run.e2e.lisFhqND.log`. No E2E cases skipped.
- Real child agents sent the A/B messages through MCP. The desktop flow proves
  inherited global enable does not compact, then sorted admission, explicit
  OFF/ON compaction, dependency restoration, reload, unchanged Auto-run, and an
  incompatible new sender session. The phone flow uses taps, effective 44px
  targets, no document overflow, compaction, and later sorted admission.
- Desktop and phone settings cover default OFF, discarded dependent unsaved
  edits, omitted sort PATCH while disabled, saved preference retention, reset,
  and reload. Existing default-OFF queue tests remain unchanged.
- Additional cross-repository SQL coverage closes the reviewed race matrix:
  `go test ./internal/orchestrator/messagequeue/... -run
  'AutoSortMergePolicyAndCompactionRaceAcrossConnections|AutoSortMergeAdmissionRetriesChangedSortOverride'`
  and `go test ./internal/backendapp/... -run
  'AutoSortMergePolicyRetryWithSharedSingleConnectionSettingsLoader'`: passed.
- `go test -trimpath -race ./internal/orchestrator/messagequeue/...
  ./internal/backendapp/... -run
  'AutoSortMergePolicyAndCompactionRaceAcrossConnections|AutoSortMergePolicyRetryWithSharedSingleConnectionSettingsLoader'`:
  passed. Uses independent SQL repositories/connections for policy changes and
  compaction, and the real settings envelope plus queue on one SQLite writer
  for retry. PostgreSQL variants were not verified.
- `go test ./internal/orchestrator/messagequeue/... -run
  'AutoSortMergePolicyAndCompactionRaceAcrossConnections/postgres' -v`:
  both PostgreSQL subcases explicitly skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset. All other PostgreSQL matrix cases
  have the same environment prerequisite and remain unverified.
- `python3 scripts/list-docs.py validate`: passed (431 decisions,
  1920 specifications). `python3 scripts/lint-spec-files.py --all`: passed
  after removing excess table padding to fit the existing design byte budget.
  `git diff --check -- docs`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
  `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- Updated public coordination, operations, sessions/review, wire reference, and
  coverage evidence. `configuration.md` does not describe these UI settings
  and is intentionally unchanged. Amended the existing merge/control
  requirements and design without changing default-OFF behavior.
- Manual smoke used a disposable `__backend` runtime with its own home/database
  and a managed browser (`relay:false`): observed default sort OFF, disabled-OFF
  dependency after discarding an unsaved edit, actual persisted GET results,
  both policies saved ON, and sort ON after reload. Captured visual proof and
  closed the browser. Desktop/mobile queue screenshots were also inspected.
- Requirement promoted to `active`, design to `current`, and plan to
  `implemented`. Final repository gates and commit are recorded in plan.md.
