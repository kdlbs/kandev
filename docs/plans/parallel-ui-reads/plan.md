---
created: 2026-11-03
status: done
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
  - REQ-AGENTS-BACKGROUND-WORK-010
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/composer-file-search-ownership.md
  - ../../specs/agents/system-design/background-work.md
  - ../../specs/platform/system-design/e2e-duration-aware-sharding.md
legacy_specs: []
---

# Implementation Plan: Parallel UI reads

## Overview

Remove independent sequential reads from task surfaces while retaining existing
session ownership, result ordering, and failure behavior. This delivery record
was added during PR remediation after documentation coverage identified its
absence. It records an implemented internal refactor, not a new product contract.
UI owns the client request scheduling and reply publication in these surfaces.

## Scope and technical approach

- Read working-tree and base-ref diff content together. Settle both requests
  before retrying and retain working-tree error precedence.
- Restore Dockview files in batches of four and retain session guards. Use four
  workers for center-panel files so a slow file cannot block free slots. Store
  center-panel results by saved index and omit failed reads.
- Start task and file mention searches together. Retain committed-owner guards,
  ranking, and file-error fallback. Pass the lookup cancellation signal through
  the existing file-search client API.
- Refresh plan comments and preview feedback together after direct, queued, or
  recovered message acceptance. Each refresh retains its own error handling.

Writes, authorization gates, cursor pagination, retries, plugin initialization,
public APIs, and rendered composition are outside this refactor. Desktop and
phone use the existing shared loading paths. No public documentation or new
browser scenario is needed for this request-scheduling change.

CI remediation also corrects a mobile test that measured an opening menu and
the existing process-end settlement path during resume. Task 02 records the
backend scope and its unchanged background-work requirements.

## Tests

The [work order](task-01-parallel-reads.md) names exact regression commands.
Deferred requests prove overlap, tab ordering, failure handling, and stale-session
rejection. Existing composer cases retain session ownership and fallback coverage.
These preserve `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5` and
`AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6`.

## Work orders

- [x] [Task 01: Parallelize independent UI reads](task-01-parallel-reads.md)
- [x] [Task 02: Settle background work before resume](task-02-resume-settlement.md)
- [x] [Task 03: Retain Kind image-load diagnostics](task-03-kind-load-diagnostics.md)

## Verification results

Initial validation: the five changed suites passed 61 tests. Eight related suites passed 107 tests.
Changed-file ESLint, the i18n ratchet, and typecheck passed. Typecheck required
an 8 GB Node heap. The affected 19-test message suite passed again after the
commit hook reformatted its fixture. Hosted CI and review completion remain
external delivery gates. No measured latency improvement is claimed.

CI remediation passed the executor package and background-settlement tests,
changed-file Go lint, and a fresh production-build replay of 45 desktop and
phone E2E tests with one worker, two CPUs, and no retries. Task 02 records the
resume repair. Hosted checks and review state are tracked on PR #5040.

The merge with `a1013e89` passed 189 targeted Vitest tests across 17 files.
Executor tests, settlement guards, typecheck, and scoped lint passed. A fresh
production build passed all 45 E2E tests without retries.

The later CodeRabbit full review identified two Kind diagnostic edge cases.
Task 03 records their corrections and 17 passing fixture tests, including four
cleanup-failure cases. Final hosted checks remain a PR delivery gate.

## Risks

Concurrency must not weaken stale-response guards or reorder saved tabs.
Unbounded restoration could overload the request transport, so file loading
retains a maximum of four active reads. Shared mutable error state can require sequencing
elsewhere and was excluded from this refactor.


### Review remediation

Cubic identified unused worker capacity, missing file-request cancellation, and
a concurrency test that admitted the reverse sequential ordering. Controlled
responses reproduced the first two gaps. A preview-first sequential mutation
made all three strengthened message cases fail. Final checks passed: 175 tests across 14 files, changed-file ESLint, typecheck,
the i18n ratchet, catalog validation, specification lint, and documentation
coverage preflight.
The separate stop-on-read-failure suggestion does not apply: read failures
return true, while false denotes retired session ownership before publication.


### CodeRabbit summary follow-up

The review body included five test suggestions without inline threads. The
existing regressions now check successful current-session restoration, preview
publication before comment completion, both diff reads settling before failure,
worker capacity after an early settlement, and no duplicate file read after
mention-task completion. The five affected suites passed 65 tests. Together
with the unchanged validated suites, final local coverage is 176 tests across
14 files. No production behavior changed in this follow-up.
