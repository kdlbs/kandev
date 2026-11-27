---
id: "01-project-watchdog-sessions"
title: "Project watchdog session observations"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-OFFICE-STALL-VISIBILITY-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1
  - AC-OFFICE-STALL-VISIBILITY-001.1
  - AC-OFFICE-STALL-VISIBILITY-001.4
  - AC-OFFICE-STALL-VISIBILITY-001.5
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/office/system-design/stall-visibility.md
---

# Project watchdog session observations

## Scope and ownership

Own `apps/backend/internal/task/repository/sqlite/session_watchdog.go`, its
test, `apps/backend/internal/orchestrator/stuck_signal_watchdog.go`, and the
focused consumer test. No dependencies. Existing watchdog decisions, active
turn checks, locked full-session reads, Office handling, generation and
delivery semantics remain authoritative.

## Acceptance

1. The watchdog uses the narrow query. A real repository fixture with 32 KiB
   MCP metadata preserves signal, identity, state, and passthrough status,
   including archived-task rows, but transfers no unrelated payloads.
2. The general reader stays full. Missing, null, scalar, and object signals
   retain their decoder meaning on SQLite and PostgreSQL; malformed metadata
   fails rather than granting recovery authority.
3. The existing recovery, cancellation, liveness, waiting-retry, Office
   detection and dedupe cases pass through the new reader.

## RED and GREEN

`TestStepSignalWatchdogProjection` first fails because the repository lacks
the narrow reader. A consumer regression refuses the broad reader and must
still complete a genuine pending workflow transition. Both must pass after
the smallest query and consumer substitution.

## Exact verification

From `apps/backend`, sequentially:

```sh
go test -trimpath -race ./internal/task/repository/sqlite -run 'TestStepSignalWatchdog|TestListActiveTaskSessionsFiltersTerminalStates' -count=1
go test -trimpath -race ./internal/orchestrator -run 'TestReclaimStuckSignal|TestStuckSignal|TestReconcileWaitingStuckSignal|TestHandleAutomationTurnComplete_(FinalizesWithoutReapingTheWorktree|LeavesASuccessfulRunAnswerable|KeepsFailureStatesTerminal)' -count=1 -v
go run ./cmd/sqlguard ./internal
go test -trimpath -race ./internal/persistence/storeconformance -count=1
```

Set `KANDEV_TEST_POSTGRES_DSN` to an owned disposable PostgreSQL database for
the first and last commands. From the repository root run scoped Go lint for
changed Go files, `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

## Results

All checks passed on base/head
`ac9531eb78439355fb034e8ca8702673831a01c7`, with the uncommitted patch:

- Repository RED: narrow reader absent. Real-consumer RED: full active-session
  reader rejected. Both failed for those intended reasons before production edits.
- Repository GREEN: SQLite and isolated PostgreSQL 16, including archived
  automation scope, all active/terminal states, passthrough status, signal
  shapes, malformed metadata, and unchanged general full-reader payloads.
- Final repository/diagnostic command added
  `Test4531DisposableScanProbe` to the first command's expression, with `-v`:
  PASS 11.255s. Diagnostic source was removed after completion.
- Consumer GREEN: 24 top-level tests, PASS 2.747s, including cancellation,
  successor-turn protection, activity, Office exclusions/dedupe, waiting-signal
  retry, and intentionally answerable successful automation sessions.
- SQL guard PASS. SQLite/PostgreSQL store conformance PASS 191.666s.
- Scoped lint PASS, zero issues. The first attempt raced temporary probe
  removal and failed typecheck for that missing file; the final-file rerun passed.
- Catalog validation PASS (396 decisions, 1691 specifications), spec lint PASS,
  and diff whitespace PASS.
- Existing contract command with `-trimpath -race -count=1 -v` across
  `./internal/mcp/handlers ./internal/task/service ./internal/agent/runtime/lifecycle`,
  selecting `TestAuthorizeAutomationRequest_SelfArchiveSucceeds`,
  `TestArchiveTaskCleanupRemovesNonEmptyTaskDir`,
  `TestArchiveTaskDirectoryCaptureCompatibility`, and
  `TestLaunchResolveWorkspacePath_NonEphemeralRepoLessGetsScratchDir`, passed
  1.586s / 2.738s / 1.552s. These use disposable SQLite, filesystem, Git, and
  service fixtures; they do not establish a repo-free scratch deletion policy.

The disposable SQLite probe held 1280 WAITING_FOR_INPUT sessions with 32 KiB
of unrelated metadata each. Metadata returned per scan fell from 41,977,600
to 49,920 bytes. Go allocations over three scans were 545,468,448 versus
6,680,840 bytes; elapsed time was 7.029s versus 0.122s under the race detector.
The existing state index and ordering sort remain; execution/environment joins
are absent from the projection. SQL JSON parsing still reads stored metadata.
These synthetic results are not a production-load or reporter-instance claim.
Allocation and elapsed measurements include one metadata serialization pass
per variant to count the first scan's returned bytes.

Fresh main `2530afe4e53c481c937bf39eb2b10c5e6382988e` has the identical backend
tree `4f3c8c8deaed32cf2a3f659341a8c474ed2435e5`; its dev-server web/docs change is
disjoint. Evidence and disposable diagnostic source are retained outside the
checkout in `../4531-evidence/`. Public docs need no change for this internal
read optimization.

The parent validated all eight files and the original candidate patch hash,
then independently passed focused `-trimpath -race` checks: SQLite repository
2.208s, watchdog/automation consumers 2.534s, and an owned disposable
PostgreSQL 16 projection fixture 2.721s. Protected delivery is authorized;
material source/test corrections require renewed parent validation. Before
commit, the actual PR-documentation validator detected missing work-order
metadata. The required identity/wave/dependency fields and plan frontmatter
references were completed without changing production or test bytes.
Exact-head CI/review results remain pending outside this delivery record.
