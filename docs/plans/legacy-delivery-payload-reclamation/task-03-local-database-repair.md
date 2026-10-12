---
id: "03-local-database-repair"
title: "Verify and repair the local database"
status: in_progress
wave: 3
depends_on:
  - "01-canonical-payload-proof"
  - "02-historical-sweep-progress"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.4
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.3
  - AC-PLATFORM-DELIVERY-RETENTION-005.3
  - AC-PLATFORM-DELIVERY-RETENTION-005.4
  - AC-PLATFORM-DELIVERY-RETENTION-006.5
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 03: Verify and repair the local database

## Summary

Prove historical reclamation preserves chats, replay and recovery on an isolated
snapshot, then apply the same corrected shared operations to the current local
database during a coordinated stop. Verify physical compaction and post-restart cleanup.

## In scope and owned files

- `apps/web/e2e/helpers/delivery-retention.ts`,
  `apps/web/e2e/tests/session/delivery-retention.spec.ts` and
  `apps/web/e2e/tests/session/mobile-delivery-retention.spec.ts`: historical-gap fixture
  and existing unchanged history/recovery flows. Reuse the isolated database fixture
  helpers in `delivery-retention.ts` and `delivery-retention-compatibility.ts`; direct
  fixture writes must target only the disposable worker database.
- `docs/public/operations.md`: explain historical raw-body reclamation, retained
  unresolved evidence, upgrade catch-up and physical maintenance limits through
  `/docs-maintainer`. No new UI or screenshot is required.
- This work order and `plan.md`: isolated and live before/after measurements and
  maintenance/restart receipts. The operational runner and output remain outside
  production source unless a separately justified public CLI requirement emerges.

## Out of scope

Do not delete tasks, sessions, canonical messages, plan HEADs, prompts or protected
terminal evidence. Do not replace a live open SQLite database, deploy a stale cloned
snapshot, disable authentication, or remove other people's backups/caches/processes.

## Acceptance

1. Desktop and phone existing history/recovery flows remain complete after old-gap
   cleanup and reload. Identical replay adds no duplicate history; changed replay
   fails; pending/uncertain terminal recovery remains guarded. No extra dispatch occurs.
2. On an isolated copy of the verified snapshot, shared bounded batches reclaim old
   eligible bodies, preserve canonical table digests/counts and protected evidence,
   and maintain projected/retired cursor bounds. Record real savings and remaining
   categories. A protected-only page cannot finish the operator run prematurely.
3. A fresh verified backup precedes any live deletion. Acquire normal exclusive
   ownership, drain through a complete captured-cutoff traversal within an explicit
   run budget, compact using the existing verified path, verify integrity, restart
   the corrected compatible backend and measure at least five normal intervals.

## Targeted verification

Read `/e2e` and `/mobile-parity` before modifying fixtures. Existing layout is unchanged;
exercise its desktop and phone contracts through the original guarded runner.

From `apps/web`:

```sh
pnpm e2e:run --project chromium tests/session/delivery-retention.spec.ts
pnpm e2e:run --project mobile-chrome tests/session/mobile-delivery-retention.spec.ts
```

Run the projects sequentially; preserve one worker per shard and never overlap full
suites. Run `pnpm install --frozen-lockfile`
from `apps` first if this task worktree lacks dependencies.

Build the corrected runtime using `make -C apps/backend build`. Keep the test binary
separate from the running artifact until the maintenance window.

### Isolated operational proof

Use the verified snapshot recorded in `plan.md` as immutable input. Copy it to a new
disposable home, acquire that home's normal ownership lock, and use the corrected
repository's retention methods. Keep native transaction, hash, row, byte and duration
limits. Revisit all discovery phases through their complete traversals; finish at
exhaustion with no eligible work at a fixed cutoff, or report explicit partial progress
at the runner deadline. Record only paths/counts/byte totals and structural outcomes.

Compare canonical content digests/counts, prompt/terminal/block/claim evidence and
cursor positions. Verify duplicate/conflicting replay through the real repository on
owned fixtures; never replay live user tools or prompts for testing. Full snapshot
`PRAGMA integrity_check` must return `ok` before and after compaction.

### Live operational execution

The user authorized unnecessary-data cleanup. Interruption timing remains unresolved
because nine sessions were running and the prior answer requested larger reclamation
rather than choosing a stop time. Coordinate the timing after isolated results are
concrete, explaining that the normal ownership lock requires a backend stop.

Capture the then-current launch identity, database pointer, diagnostic baseline and
active sessions. Stop only the identified instance. Acquire the same lock as normal
backend startup. Take and verify a fresh snapshot before the first transaction, reuse
the isolated-tested runner and corrected methods, and report committed partial results
on failure. Never bypass the backend ownership lock or synthesize settlement metadata.

After logical cleanup, the standard compaction command is:

```sh
<corrected-kandev-binary> maintenance database --home-dir /home/jcfs/.kandev --execute --compact --keep-plan-revisions 5
```

It takes its own fresh verified backup and preserves a pre-compaction rollback file.
This invocation alone runs only a bounded delivery batch; it is not a complete backlog
drain. Check disk capacity before both backup/staging operations and avoid repeated
full-backup CLI calls merely to advance the delivery cursor.

Restart the corrected runtime through the original launch mode. Verify `/health`,
canonical history, existing recovery state and aggregate `agent_delivery.retention.*`
logs across at least five intervals. Report raw payloads, compact evidence, main-file
bytes, reusable pages, WAL and backup sizes separately. Five intervals demonstrate
near-term behavior, not a permanent quota.

## Results and cleanup

Record the verified backup and rollback paths, canonical comparison results, actual
removed bytes/rows, physical before/after size, restart version and interval samples.
Remove only the disposable copy, test-only runner source/binary and temporary logs
created for this work order. Retain the user's verified recovery backup.

Results: isolated verification complete; live repair pending interruption coordination.
The repaired snapshot reclaimed 3452073598 raw bytes and compacted to 3118968832 bytes,
with preserved digests unchanged and integrity `ok`. Desktop and phone E2E passed.
The task-specific runner uses the production 50-row quantum after larger admitted
batches repeatedly timed out; complete zero-removal traversal, not a protected-only
sample, ends its drain. An independently running coordinator is prepared at
`/home/jcfs/.kandev/retention-repair-20261011/live-repair.py`; it acquires normal ownership
through the runner, verifies a fresh backup, compares protected/canonical digests,
uses native compaction, installs the corrected runtime, restores the original launch
environment and records five normal interval samples. It is not yet executing.

At the interruption checkpoint the user asked whether merging the repair and
restarting from the new main would automatically clean the database. Yes: the normal
bounded worker performs historical catch-up without this operator runner. It can
take hours for this backlog; SQLite file shrinkage still requires Vacuum after
logical reclamation. No downtime option was selected, so no live maintenance,
runtime replacement or restart was started. The coordinator refuses a changed
runtime base until its maintenance procedure is revalidated.

PR preparation rebased the repair onto the current main. Retention race tests,
typecheck, documentation validation and desktop history/recovery passed there.
The phone capacity-continuation fixture then failed twice: the post-restart send
raced lazy agent startup, entered the ordinary queue path, and was blocked as
an unknown prompt outcome rather than exercising retained-runtime continuation.
The fixture now completes a normal warm-up turn and waits for idle before
capturing its capacity trace. The unchanged continuation, side-effect, touch,
history and recovery assertions passed on phone after that readiness correction.
This changes only the fixture setup; queue startup behavior is outside this
retention repair. The final desktop rerun passed (one test, optional frozen-binary
case skipped), phone passed (one test), and final typecheck passed for the new helper.

PR #4418 CI found a pre-existing frontend lint failure on the rebased main:
`useSecrets` contained 105 lines against the 100-line limit. The failure reproduced
locally. Extracting its existing item selection into a private pure helper preserved
scope filtering, stale workspace suppression and array identity. All 44 existing
secret-hook tests, full frontend lint and typecheck passed. This is behavior-preserving
CI remediation, with no new user-facing contract or documentation requirement.

Conflict resolution merged main at `e8c1761cb5b7006b905c0aa995925372ce8664e1`.
Both branches had extracted the same secret-selection helper; the resolution keeps
main's typed helper and matching call. All 44 secret-hook tests, typecheck, full web
lint, documentation checks, retention race tests (with the previously documented
million-row exclusions), and desktop cleanup/recovery E2E passed on the merge.
The phone rerun was interrupted when the user requested immediate commit and push;
CI owns the remaining validation.


CI remediation on merge head `f2d5da2504c6f7da27288ce8ea4d174efa9067f0`
reproduced all three browser failure scenarios from E2E run `38162790010`.
The Quick Chat Resume guard rejected historical blocks without a delivery recovery
notice. Resolution now requires the persisted ACP identity to match the current
harness generation and the exact interrupted SQL submission; the authenticated
journal retirement prerequisite is unchanged. Missing native metadata, mismatched
identity, superseding generation, and mismatched submission remain blocked. A
failing legacy regression preceded the repository change; SQLite and PostgreSQL
native-resume tests passed under the race detector, including existing rollback,
newer-notice, missing-metadata, and stream-binding tests.

The live-block fixture now retains a real native conversation, uses its generation
and stream, and seeds an un-emitted journal-only submission. The separate record
reconstruction fixture keeps its retained output, using the actual journal append
sequence rather than assuming an empty stream. Both preserve the no-resend checks.
The runtime replacement tests capture history before killing the runtime and await
automatic restoration before checking browser reload persistence and follow-up.
They verify replacement without restarting the backend during candidate launch;
these scenarios cover runtime replacement and reload rather than a second backend
restart incident. Native identity, original turns/messages, no implicit recovery
request, no interrupted-prompt replay, and phone geometry remain asserted.

All affected desktop scenarios passed with retries disabled in focused runs. The
phone Quick Chat Resume, automatic runtime replacement/reload, missing-record
reconstruction, and live-block tests passed together (four tests, zero retries).
Web typecheck and changed-file ESLint, backend lint against `e8c1761cb5`, SQL guard,
store-conformance race tests, documentation validation, and specification lint
passed. The original CI blob audit accounted for all 20 shards and all failed
attempts; the current-head review snapshot contained no unresolved threads.

Backend Postgres job `114539277378` subsequently reported a protected-gap stress
assertion that measured reclaimed bytes only from the contiguous retirement cursor.
That measurement is valid for prefix deletion, but not for independent compaction
past a protected terminal. At the user's request, both million-row storage tests
and their exclusive measurement/query-plan helpers were removed. A 200-row
protected-gap regression checks bounded batches, the unchanged protected frontier,
canonical content, the protected terminal payload, and the exact count of compact
receipts after the gap. It passed on SQLite and PostgreSQL under the race detector.
The other focused retention and query-plan regressions remain. Full CI remains
owned by the user after the remediation push; no live database maintenance or
restart was performed in this CI remediation.


The next CI run (`38169809017`, head `ab28e56691`) passed backend and frontend
checks but failed the desktop retention recovery scenario in all four attempts.
After runtime loss, Retry raced automatic restoration: it returned an ownership
change and the banner disappeared once restoration committed, while the test
expected a permanent generic block. The unmodified focused test passed locally,
confirming the expectation depended on timing.

The shared desktop/phone retention fixture now seeds an independent native-resume
block for the current session incarnation before killing the runtime. SQLite
creates its timestamp in the same sortable form as the stored recovery rows so
the earlier independent cause is selected consistently. The test awaits the
correlated Retry response, requires `blocked` with `independent_recovery_block`,
and checks that Retry and Stop preserve the independent cause. Existing actual
payload cleanup, protected historical gap, saved history, capacity continuation,
no-resend, and phone touch-target assertions remain. This is fixture-only CI
remediation; production cleanup and recovery behavior are unchanged.

The corrected desktop and phone retention scenarios each passed with retries
disabled. Changed-file ESLint and web typecheck passed. All 20 CI blob reports
were inspected; the retry summary identified this one failing test plus two
flaky tests in passing jobs. CI and review verification for the next pushed head
remain pending with the user. No live database maintenance or restart occurred.
