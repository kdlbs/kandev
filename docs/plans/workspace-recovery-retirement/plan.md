---
status: implemented
created: 2026-11-10
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
system_design:
  - ../../specs/agents/system-design/workspace-recovery-retirement.md
---

# Workspace recovery during runtime retirement

## Outcome

Repair the mobile session-recovery E2E failure found during attachment-hover
PR validation. Workspace lookup must not return the cached controller while
failed-start cleanup is retiring it. Preserve existing restoration admission,
terminal state, conversation identity, and concurrent-request deduplication.

## Delivery

[Task 01](task-01-wait-for-runtime-retirement.md) is the single work order.
Use the existing session operation gate before cached reads, then the existing
runtime lifecycle lock to wait for retirement and recheck registration.
This changes no controls, layout, API, or persistence schema.

## Verification

Targeted lifecycle regressions reproduce both cached-read bypasses before the
correction. The existing mobile recovery scenario must pass with retries
disabled, followed by repeated mobile and corresponding desktop checks.
The work order records exact commands and local results; remote CI remains a
separate publication gate.
