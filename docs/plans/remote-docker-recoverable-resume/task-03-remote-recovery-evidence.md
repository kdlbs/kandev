---
id: "03-remote-recovery-evidence"
title: "Prove real remote recovery and document its limits"
status: completed
wave: 3
depends_on: ["02-reconcile-journal-owner"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
  - ../../specs/platform/system-design/durable-agent-journal-ownership.md
---

# Task 03: Prove real remote recovery and document its limits

## Summary

Prove that a failed recovery retains real unpushed work on a Docker daemon
reached through SSH. Exercise the existing UI Resume and explicit cleanup,
then update the operator guidance with the verified behavior and limits.

## In scope

- Add a remote recovery spec using `remote-docker-test-base.ts`.
- Use real unpushed Git and untracked-file markers in the owned container.
- Induce a controlled journal lock conflict without changing the user's environment.
- Prove safe owner reuse or release, successful retry, and backend-restart recovery.
- Run existing desktop and phone recovery feedback scenarios.
- Update the executor reference and synchronize affected delivery records.

## Out of scope

New UI, test-only production routes, native-provider claims, arbitrary remote
process kills, live deployment repair, and unrelated pending companion work.

## Acceptance

1. Failed bootstrap and journal acquisition preserve the original container,
   marker commit SHA, untracked marker, and journal. Retry resumes that same
   environment after the fixture releases its lock, including after backend restart.
2. Live prior ownership produces no second writer or automatic prompt replay.
   Blocked feedback survives reload. Existing desktop and phone controls pass.
3. Reset, archive, and delete still remove only the owned environment. Public
   guidance distinguishes lock contention, permissions, and I/O failures.

## Scenario and fixture

Create `tests/remote-docker/remote-docker-recovery.spec.ts` beside the current
remote task lifecycle spec. Use API setup, then open `/t/<task-id>` and invoke
the visible Resume control. Assert its result in the UI and remote inventory.

Before failure, create a local commit that no remote ref contains. Record its
SHA and marker content. Also leave a distinct untracked marker. After failure,
inspect the same container and assert both markers plus the unchanged SHA.
Capture container ID, exact instance ID, and delivery identity before cleanup.

For controlled contention, stop/dispose of only the fixture-owned agentctl
instance before an external helper acquires its journal. Add a bounded helper
test in `e2e_remote_docker_journal_lock_fixture_test.go`, compiled by the existing
`build-e2e-delivery-fixture` target. Follow the existing pruned-delivery fixture
entry pattern. Copy the matching Linux fixture into the owned container through Docker.
Use explicit ready and release signals, a deadline, and cleanup.
Do not remove or rewrite the journal. Do not add a production lock endpoint.
Include a fixture lifecycle test that verifies acquisition, exclusion, and release.
The helper's guarded child entry alone does not provide that evidence.

Release the helper after the blocked assertion, retry Resume through the UI,
and assert a newly accepted turn-specific response. Verify the markers again.
Repeat with a backend restart between failure and retry. Add a separate live
prior-owner case that verifies no second open or duplicate instruction.
Finish with explicit reset, archive, and delete controls on disposable tasks.
Use the fake-Engine unit tests for deterministic late-cleanup races.

The container fixture proves resource behavior. The existing shared recovery
helper and `mobile-session-error-recovery-ui.spec.ts` prove phone feedback and
retry interaction. No new composition or per-test device override is needed.

## Public documentation

Update `docs/public/executors.md` as a reference page. Explain retained recovery,
explicit destructive operations, and the limit on already removed containers.
Explain lock timeout, permission denial, and I/O failure separately. Never
recommend journal deletion or force-removal as a routine recovery action.
Keep the remote-host filesystem statement accurate for Docker-managed data.
Search the root README and screenshot catalog for conflicting guidance.
No screenshot update is expected because rendered UI does not change.

## Verification

Run from the repository root. Install workspace dependencies once if absent.
The managed runner rebuilds required artifacts. Run all commands sequentially.
do not overlap suites or increase the one-worker-per-shard limit.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestRemoteDockerJournalLockFixture' -count=1)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/remote-docker/remote-docker-task.spec.ts tests/remote-docker/remote-docker-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run changed-file Go lint for the fixture and focused web lint for the new
spec. Run the repository documentation-coverage preflight over the actual diff.
Record exact commands, discovered tests, pass/fail/skip counts, source/binary
identity, and owned fixture teardown. A skipped container scenario is not success.

## Files likely touched

- `apps/web/e2e/tests/remote-docker/remote-docker-recovery.spec.ts` (new)
- `apps/web/e2e/fixtures/remote-docker-test-base.ts`, only for owned setup/cleanup.
- `apps/backend/internal/agent/runtime/lifecycle/e2e_remote_docker_journal_lock_fixture_test.go` (new)
- `docs/public/executors.md`
- This plan and its work-order Results sections.
- Affected companion plan results named in `plan.md`, only where this repair changes their evidence.

## Dependencies

[Task 02](task-02-reconcile-journal-owner.md). Retention and journal ownership
must pass their permanent RED/GREEN gates before this integration pass.

## Risks

- A marker committed only in the backend checkout does not prove remote retention.
- A generic completion count can match an earlier turn. Use a new response marker.
- Fixture helper architecture and the rebuilt production artifacts must match the container.
- Docker or SSH unavailability leaves this work order pending with the exact blocker.

## Parallelism

`sequential`

## Inputs

- The plan's test matrix, both designs, `/e2e`, and `/docs-maintainer`.
- Existing `remote-docker-task.spec.ts`, `remote-docker-test-base.ts`, and
  `e2e_pruned_delivery_fixture_test.go`.
- Existing shared `session-error-recovery-ui.ts` desktop/phone scenarios.

## Results

Completed on 2026-10-29 against upstream `1b0d0881a4108697b8c346d17c5493fd34dd3f36`.

- Added `apps/web/e2e/tests/remote-docker/remote-docker-recovery.spec.ts`.
  It writes a committed marker and an untracked marker inside the task
  container, stops and resumes the same session with the production `resume`
  intent, asserts the replacement execution identity, and then verifies the
  same container ID, marker commit, and both files. It finishes by deleting the
  task and polling the remote inventory for removal.
- The remote recovery spec passed both cases in 1.5 minutes:
  `same-session resume keeps the container, journal, and unpushed work`
  (18.9s) and
  `blocked journal acquisition preserves the workspace and retries after release`
  (22.1s). The first case reattached to the same container with the marker
  commit and untracked file unchanged.
- The controlled blocked-journal case passed. An external `flock` holder
  acquired the released journal while a resume was attempted; the launch did
  not become ready, the container and markers survived, and a retry after
  releasing the lock resumed the same workspace. The image's own `flock`
  replaces the planned Go helper, so no test-only binary is copied into the
  container. A FIFO releases the holder deterministically with a bounded
  timeout.
- The blocked case restarts the backend between failure and retry. The
  container, journal, and durable execution record survived the restart, and
  the retry succeeded.
- The existing `remote-docker-task.spec.ts` container suite passed all three
  cases: launch/reconnect, deletion removal, and no remote-host footprint.
- Desktop recovery feedback passed all 10 cases in the existing
  `session-error-recovery-ui.spec.ts`. Its phone counterpart passed all 10
  cases in `mobile-session-error-recovery-ui.spec.ts`.
- Updated `docs/public/executors.md` with retained recovery, resumed bootstrap,
  and journal acquisition guidance. It distinguishes lock timeout, permission
  denial, filesystem/quota, and I/O or corruption failures, and states that
  deleting a journal or force-removing a container is not routine recovery.
- The remote recovery spec drives the production `session.launch` resume
  intent through the API. It does not click the visible Resume button itself;
  the existing desktop and phone recovery specs cover those controls and their
  reload/blocked states.
- The blocked case now asserts the exact `delivery journal unavailable: open
  delivery journal: lock_timeout` error, extracts the blocked attempt's
  execution ID from that error, checks it differs from the prior execution,
  and matches the persisted backend log record for the same session. The
  durable session state remains `CANCELLED` while the lock is held.
- Each successful resume sends a distinct mock-agent marker turn and waits for
  its new response. It also asserts the session has no current
  `session_recovery_blocks` before the turn and that the container, marker
  commit, and untracked file remain unchanged after it.
- Live prior-owner adoption and restart recovery are fake-control unit coverage:
  `TestDockerReconnectAdoptsLivePreviousJournalOwner` and
  `TestRemoteDockerRecoveryUsesPersistedAdoptedAgentctlInstanceID`. They are not
  real-daemon evidence; the real-daemon spec covers stopped-owner reconciliation
  and blocked journal recovery.
- `node scripts/validate-public-docs.mjs` validated 54 published docs pages.
  Catalog validation, specification lint, and documentation coverage
  (three work orders, zero reference errors) passed.
