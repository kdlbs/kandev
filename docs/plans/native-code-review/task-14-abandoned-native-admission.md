---
id: "14-abandoned-native-admission"
title: "Recover admission after an abandoned native review"
status: building
wave: 11
depends_on: ["04-run-orchestration"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.14
system_design:
  - ../../specs/agents/system-design/native-code-review-recovery.md
---

# Task 14: Abandoned native review admission

Restore explicit review admission when a native worker has exited but its
terminal-state write did not settle the retained run.

## Scope and acceptance

- Apply AC-AGENTS-NATIVE-CODE-REVIEW-001.14 through
  [the recovery design](../../specs/agents/system-design/native-code-review-recovery.md).
- Confirm terminal settlement before fresh admission. Keep original rows,
  findings, duration, agent ownership, live-worker deduplication and entry IDs.
- Preserve read and settlement errors without dispatching a duplicate worker.
  Do not change inference timeouts, candidate order, provider fallback, schema,
  startup cancellation, polling or UI behavior.

## Files and verification

`apps/backend/internal/review/runner.go` and `recovery.go` own admission and
cancellation ordering. Permanent coverage lives alongside them:

- `recovery_sql_test.go`: manual/workflow pending/running settlement; original
  history, accounting and finding preservation; multiple retained native rows;
  agent/unclassified ownership, including an older owner hidden by a native row;
  workflow-entry redelivery and a distinct new entry.
- `recovery_test.go`: active-read and settlement errors, nil/nonterminal
  settlement, inconsistent terminal active rows and errors after settlement.
  Every stopped admission checks zero inference and uses a successful request
  after storage recovery to prove claim release.
- `recovery_ordering_test.go`: a live worker rejoins without duplicate inference;
  inference leaves settlement and independent task claims available; real
  ReviewService/SQLite cancellation and settlement preserve the acknowledged
  terminal state in both write orders. Repository barriers pause the actual
  service between its reads and SQL write.

Run from `apps/backend`:

```sh
go test -trimpath -tags fts5 -race ./internal/review -count=1 -v
```

Run changed-file Go lint, documentation catalog and specification validation
from the repository root. Execution for this review is hosted only. Focused
proofs must show assertion failures for disabled native recovery, newest-only
settlement and missing cancellation serialization, with unchanged positive
controls. They are deliberate source mutations, not claims that the final
source fails. Full final-head backend, frontend and E2E workflows remain
acceptance gates; a path-filter wrapper is not full-suite evidence.

## Results and evidence

Permanent tests now replace the disposable preparation probes as the standing
regression contract. Historical package timings, temporary overlays and the
interrupted synctest fixture are not acceptance evidence for this revision.
Hosted focused and full-suite results are pending and must be recorded against
actual source, workflow and attempt identities in the PR delivery receipt.
No live-provider or installed-runtime acceptance is claimed by these tests.
