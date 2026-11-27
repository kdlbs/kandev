---
created: 2026-09-30
status: complete
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
legacy_specs: []
---

# Implementation plan: Workspace idle policy transport

## Overview

Keep the saved workspace idle-suspension policy in frontend state after a page load and after a workspace lifecycle event.
This correction completes the existing workspace settings contract. It does not change policy storage or runtime ownership.

## Work orders

- [x] [Task 01: Workspace policy state transport](task-01-workspace-policy-state.md) (done)

## Verification results

- Backend settings boot regression passed.
- Backend workspace lifecycle event regression passed.
- Frontend workspace WebSocket regressions passed.
- Frontend lint passed after the workspace update merge was split into small helpers.
