---
created: 2026-09-26
status: done
requirements:
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-LAUNCH-SAFETY-003
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
---

# Implementation Plan: Office Step-Handoff Causation

## Overview

Wakes queued when a task enters a workflow step (review, approval, rework, and
Office `auto_start_agent`) are recorded as fresh `system` roots at depth `0`.
Because of that, the causation-depth refusal of REQ-OFFICE-LAUNCH-SAFETY-003
cannot see a review and rework loop, and a human move is recorded as `system`
instead of human-rooted. This plan resolves step-entry wakes from the committed
step-transition ledger row, per AC-OFFICE-RUN-CAUSATION-001.25. Office task
launches carry a trusted causing run ID through lifecycle events into the
transition record, so dispatch does not depend on session binding or claim
fields that retry and recovery can clear.

## Implementation Wave

- [x] [task-01-step-entry-causation-from-ledger](task-01-step-entry-causation-from-ledger.md)

## Verification

```bash
cd apps/backend && go test ./internal/workflow/engine ./internal/office/service \
  ./internal/runs/service ./internal/orchestrator ./internal/backendapp
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

## PR Integration Results

The current-main integration preserves trusted causing-run identity alongside
landed delivery identity, interrupted-prompt recovery, and retention behavior.
Completion handlers retain delivery settlement and cancellation classification
while carrying the causing run into the transition ledger. Lifecycle and watcher
regressions assert both identities in the same completion event.

Task 01 records the integration validation and its local timeout limitation.
Current-head CI and merge receipt remain delivery checks outside this work order.
No public documentation change is needed for this integration: the existing
causation requirement and design remain the behavior contract.

The integration CI exposed the landed `useSecrets` hook's function-size warning.
Task 01 also records its behavior-preserving helper extraction and focused hook
validation. Fresh remote CI remains required after that remediation push.


## Resumed Main Integration

The resumed delivery resolves actual conflicts with the newer main branch while
preserving causing-run attribution alongside passthrough launch identity,
native-conversation replacement, executor admission, recovery, and database
ownership initialization. The secrets hook uses the canonical main implementation,
which already contains the earlier function-size extraction and newer failure
handling. Task 01 records the focused validation; historical results above do
not establish the new merge head's CI status.
