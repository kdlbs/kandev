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
