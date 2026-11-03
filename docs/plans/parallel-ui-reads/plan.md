---
created: 2026-11-03
status: done
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
  - ../../specs/ui/system-design/composer-file-search-ownership.md
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
- Restore saved editor files in batches of four. Create Dockview panels in saved
  order and retain session guards. Center-panel results retain saved order and
  omit failed reads.
- Start task and file mention searches together. Retain committed-owner guards,
  ranking, and file-error fallback.
- Refresh plan comments and preview feedback together after direct, queued, or
  recovered message acceptance. Each refresh retains its own error handling.

Writes, authorization gates, cursor pagination, retries, plugin initialization,
public APIs, and rendered composition are outside this refactor. Desktop and
phone use the existing shared loading paths. No public documentation or new
browser scenario is needed for this request-scheduling change.

## Tests

The [work order](task-01-parallel-reads.md) names exact regression commands.
Deferred requests prove overlap, tab ordering, failure handling, and stale-session
rejection. Existing composer cases retain session ownership and fallback coverage.
These preserve `AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5` and
`AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6`.

## Work orders

- [x] [Task 01: Parallelize independent UI reads](task-01-parallel-reads.md)

## Verification results

The five changed suites passed 61 tests. Eight related suites passed 107 tests.
Changed-file ESLint, the i18n ratchet, and typecheck passed. Typecheck required
an 8 GB Node heap. The affected 19-test message suite passed again after the
commit hook reformatted its fixture. Hosted CI and review completion remain
external delivery gates. No measured latency improvement is claimed.

## Risks

Concurrency must not weaken stale-response guards or reorder saved tabs.
Unbounded restoration could overload the request transport, so file loading
retains a fixed batch size. Shared mutable error state can require sequencing
elsewhere and was excluded from this refactor.
