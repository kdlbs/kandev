---
created: 2026-09-26
status: planned
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
step-transition ledger row, per AC-OFFICE-RUN-CAUSATION-001.25.

## Implementation Wave

- [ ] [task-01-step-entry-causation-from-ledger](task-01-step-entry-causation-from-ledger.md)

## Verification

```bash
cd apps/backend && go test ./internal/workflow/engine ./internal/office/service \
  ./internal/runs/service ./internal/orchestrator ./internal/backendapp
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```
