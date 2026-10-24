---
id: "01-terminal-primary-queue-handoff"
title: "Hand off a terminal primary's queue behind a flag"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001
acceptance_criteria:
  - AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4
  - AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.5
system_design:
  - ../../specs/tasks/system-design/primary-session-ownership.md
---

# Task 01: Hand off a terminal primary's queue behind a flag

## Summary

Add `features.terminalPrimaryQueueHandoff` (off in every profile). With it on,
moving the primary role off a terminal session transfers that session's queue
to the new primary and drains it when Auto-run allows.

## In scope

- `FeaturesConfig.TerminalPrimaryQueueHandoff`, runtime flag registry entry,
  `KANDEV_FEATURES_TERMINAL_PRIMARY_QUEUE_HANDOFF` in `profiles.yaml`, and
  `orchestrator.ServiceConfig.TerminalPrimaryQueueHandoff`.
- `SetPrimarySession` reads the previous primary and schedules
  `handOffTerminalSessionQueue` on the service-owned worker lifetime when it is
  terminal; hand-offs are serialized and resolve the current primary.
- `messagequeue.TransferRetiredSessionWithDurableAttachmentPreparation` keeps
  the receiver's Auto-run inside the transfer's admission, and the hand-off
  re-checks the receiver's primary role there, retrying when it moved.

## Verification

`go test ./internal/orchestrator/ -run 'TestPrimary|TestExplicitPrimary'` covers
automatic promotion, an explicit choice, a live previous primary, the
flag-off default, a paused previous primary with an enabled receiver,
successive terminal promotions, no hand-off after `Stop`, a promotion that
lands between receiver resolution and transfer, and a receiver Auto-run pause
made during the hand-off.
