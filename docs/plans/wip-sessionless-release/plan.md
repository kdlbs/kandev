---
created: 2026-10-20
status: complete
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation plan: Sessionless WIP slot release

## Overview

An admitted task keeps its workflow step's WIP slot until it leaves the step,
even when its agent failed or all of its sessions ended. On a WIP-1 step that
blocks every queued task behind it. This plan adds a per-workspace opt-in,
off by default, that stops counting such a task after a grace period, and an
always-available read that explains who holds each slot and why.

## Work orders

- [x] [Task 01: Sessionless release and occupancy read](task-01-sessionless-release.md) (done)

## Verification results

- Repository regressions for off, after-grace release, grace, live session,
  re-entry, uncovered states, step listing, and the occupancy read passed.
  The release and re-entry cases fail when the count ignores the policy.
- The orchestrator sweep regression passed for both policy values, including
  a repeated pass that does not over-admit.
- Workspace setting, HTTP read, frontend card, draft, WebSocket, and boot
  mapping regressions passed. Frontend lint, typecheck, and `i18n:check`
  passed.
