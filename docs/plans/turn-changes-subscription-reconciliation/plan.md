---
created: 2026-10-27
status: done
requirements:
  - REQ-TASKS-TURN-CHANGES-005
system_design:
  - ../../specs/tasks/system-design/turn-changed-files-subscription.md
legacy_specs: []
---

# Implementation plan: Reconcile turn changes after subscription

## Overview

A first history read can observe a pending capture before the WebSocket session
subscription is registered. If the terminal update falls into that gap, the
card remains absent despite finalized history. Revalidate the first history
page after subscription readiness, preserving newer live revisions and offline
reads.

## Scope

Share the correction across desktop and phone transcript and historical-diff
consumers. Keep capture, message ordering, and persisted data unchanged.

## UI-01: Transcript after initial capture

```text
[assistant reply]
[Changed files] [Open diff]
```

The same card appears on desktop and in the phone transcript, using the
existing touch actions and scroll owner. The structure illustrates
AC-TASKS-TURN-CHANGES-005.1 and .9, not exact spacing.

## Tests and work order

A focused hook regression covers a delayed subscription and overlapping
pre-subscription request. Existing desktop and phone changed-file scenarios
cover the rendered card and diff navigation.

- [x] [Task 01: Reconcile subscription gap](task-01-reconcile-subscription.md)

## Verification results

The regression failed before the correction and the focused hook, pagination,
and revision suites passed (8 tests). Desktop E2E passed (1) and mobile E2E
passed (3), with retries disabled. The full web suite passed (3,113 files,
28,285 tests, 4 skipped). Focused lint, typecheck, and specification validation
passed. The full suite preceded one additional passing error-recovery test.

## Risks

Subscription ownership and initial in-flight requests must not leak, regress
pagination, or overwrite a newer live summary.
