---
created: 2026-11-25
status: in_progress
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Bind the interrupted upgrade session's recovery notice

## Evidence

Both PostgreSQL 16.16 desktop jobs found the interrupted session's resume token and then returned the same error from notice publication. The later run passed both SQLite projects. The failing artifact includes the baseline snapshot and runtime logs. It omits the resolved database rows. [Parent job](https://github.com/kdlbs/kandev/actions/runs/45598454917/job/139151259453) and [README merge job](https://github.com/kdlbs/kandev/actions/runs/45628259707/job/139255522485).

A temporary Go test overlay exercised the real native resume transaction with one authenticated interrupted submission, an absent recovery snapshot and a single generic unresolved-work block. SQLite and PostgreSQL 17.11 committed the pending marker for the submission. Both committed the resolved block with empty delivery_submission_id and delivery_stream_id. VerifyCommittedRecoveryNoticeSource returned false on both engines. This reproduces the observed error path. The artifact does not establish which open block the historical run selected.

## Delivery

[Bind and verify the incident in the recovery transaction](task-01-bind-incident.md) is the only work order. The existing [requirement](../../specs/platform/requirements/durable-agent-delivery.md) and [manual resume design](../../specs/platform/system-design/interrupted-prompt-recovery.md) govern the repair. Criterion AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12 covers identity-less incident blocks.

The principal risk is attributing a generic block to the wrong submission when multiple rows qualify. Require unique authenticated evidence before creating a submission-specific binding. Verify using both engines before opening the PR. Keep auto-merge disabled and the PR outside the merge queue. Keep DECISIONS.md uncommitted.
