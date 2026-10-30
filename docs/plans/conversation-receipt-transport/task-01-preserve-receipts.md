---
id: "01-preserve-receipts"
title: "Preserve conversation receipts through transport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.2
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.3
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.8
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Preserve conversation receipts through transport

## Scope

Port the retained changes from de1c61bfa5bb, 7196cdf7d952, and 30db679e150e.
The reviewed dense-workbench work order and explicit extraction request authorize implementation.
Keep the research checkout read-only and exclude benchmark experiments from this contribution.

Owned production boundaries are the gateway receipt decoder, canonical SQL projector,
lifecycle receipt transport/coalescing, orchestrator notification, backend service adapter,
and frontend message timestamp guard. Tests remain beside those owners, with two focused
browser scenarios and their shared canonical-stream helper.

## Acceptance

1. Typed and serialized ordinary writes deliver contiguous updates without source reset.
2. Canonical receipts capture the transaction's final message and turn snapshots before commit.
3. Replay and rollback produce no new receipt. Incomplete or noncontiguous evidence retains recovery.
4. Published messages use committed receipt snapshots rather than a later database read.
5. Desktop and phone display the final durable stream content with no normal-write resets.
6. Timestamp ordering preserves equivalent instants, fractions, offsets, and nanosecond precision.

## Validation

Run the gateway, task service, and SQLite package suites. Run focused race tests for canonical
projection, lifecycle coalescing, and orchestrator publication. Use an owned PostgreSQL cluster
for receipt parity and persistence conformance, with no skipped PostgreSQL receipt leaves.
Run SQL guard, changed-file Go lint, strict changed-file ESLint, and web typecheck.

Run these web checks from `apps/web`:

```bash
pnpm exec vitest run lib/ws/client.test.ts lib/ws/handlers/messages-timestamp.test.ts hooks/domains/session/use-session-messages.live-refresh.test.tsx
pnpm e2e:run --project chromium tests/plugins/conversation-recovery.spec.ts tests/performance/dense-workbench-correctness.spec.ts
pnpm e2e:run --project mobile-chrome tests/performance/mobile-dense-workbench-correctness.spec.ts
pnpm run typecheck
```

Browser builds and runs hold `/tmp/kandev-contribution-validation-5936a872.lock`.
All data comes from isolated fixtures. The production frontend has no markup or interaction
change, so screenshots are not a visual acceptance criterion. Browser checks verify delivery.

## Results

RED: The serialized gateway test resets at the first contiguous revision. The repository
lacks the receipt-returning projection capability. Four timestamp cases fail under text ordering.
GREEN: The three focused frontend files pass all 48 tests. Web typecheck passes with
`NODE_OPTIONS=--max-old-space-size=8192`; the default 4 GiB run exhausted its heap.
Strict ESLint and Prettier checks pass for every changed frontend file.

`go test -trimpath -race -p 2` passes the gateway, SQLite, lifecycle, and orchestrator
packages with `-run 'Test(ConversationDeliveryPreservesSerializedReceipt|Canonical|StreamCoalescer|DurableAgentEvent)' -count=1`.

A private PostgreSQL 17.11 cluster with TCP disabled and isolated schemas passes
`go test -trimpath -race -p 1 -json -count=1 -timeout=10m ./internal/task/repository/sqlite -run '^TestCanonicalConversationReceipt'`.
New-turn, existing-turn, rollback, and thinking leaves run and pass on both SQL dialects.
Separate private clusters pass the existing concurrent-writer receipt test and
`^TestPostgresCanonicalDeliveryTurn` cases, including contention retry and cancellation.
`go test -trimpath -race -p 1 -timeout=15m ./internal/persistence/storeconformance -count=1`
and `go run -trimpath ./cmd/sqlguard ./internal` pass with the private PostgreSQL DSN.
All owned clusters stop after their checks.

The first managed browser run failed before test execution because inherited configuration
bound the backend to a Tailscale address. The rerun removes internal config overrides and
sets `KANDEV_SERVER_HOST=127.0.0.1`. Existing readiness deadlines remain unchanged.
Go lint passes with the repository changed-package runner and `GOMEMLIMIT=5GiB`.
Earlier attempts exceeded the five-minute limit during package loading/analysis and are not counted as passes.
The full gateway and task-service suites pass.

The next browser attempt reached fixture readiness but Chromium could not load Nix shared libraries.
The passing attempt uses a process-local `LD_LIBRARY_PATH` resolved from installed Nix libraries.
No repository browser settings or test deadlines change.

Desktop managed E2E passes all three tests: canonical streaming, core gap recovery,
and plugin missed-update/pagination recovery. Phone managed E2E passes its canonical-stream test.
Both use the freshly built candidate artifacts under the shared validation lock.
Each canonical-stream test verifies durable and displayed final content, zero source resets,
nonempty revision intervals, and viewport containment.

The first serial SQLite package run was stopped after 35 minutes because it had reached
only about 340 of 2,110 registered tests. It is not counted as a pass. The replacement
runs eight disjoint name partitions with `GOMAXPROCS=1`, `go test -trimpath -p 1 -json
-count=1 -timeout=60m ./internal/task/repository/sqlite -run <anchored-partition>`.
The audit records all 2,110 registered tests without missing or duplicate results:
1,746 passed, 363 skipped, and one failed. The existing
`TestTaskGitHubIssueSQLiteCancellation` exhausted its 15-second context while creating
the fixture, before reaching its cancellation assertion. Its deadline starts before
schema setup. This is the same fixture limitation recorded in the source work order;
this extraction leaves that unrelated test unchanged. PostgreSQL skips remain separate
from the fresh parity evidence.

The cancellation test passes three consecutive isolated reruns (`-count=3`, 38.373 seconds).
The initial sharded run remains recorded as failed; the reruns do not erase its setup timeout.
All scoped acceptance checks are complete. Remote CI and review remain delivery gates.
