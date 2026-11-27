---
created: 2026-10-12
status: completed
requirements:
  - REQ-TASKS-TERMINAL-TARGET-001
system_design:
  - ../../specs/tasks/system-design/terminal-target-lifecycle.md
legacy_specs: []
---

# Fix plan: stale terminal environments

## Evidence and scope

The updated backend includes the three merged recovery fixes. The reported
unknown-outcome block resolved during explicit Resume, and its next prompt
completed. No new prompt recovery defect was observed for that session.

The separate terminal belongs to a task deleted hours earlier. Its environment
and terminal rows are absent, while a retained browser keeps opening its old
URL. The terminal gateway maps missing environments to transient 503 responses;
the browser cannot inspect that failed handshake and retries indefinitely.

Continue the user's authorized recovery repair with an explicit missing-target
close outcome, stopped retries, and existing ended feedback. Preserve live
data and all prompt-recovery admission checks.

## Work orders

- [x] [Task 01: End removed terminal targets](task-01-end-removed-targets.md)

## Desktop and phone behavior

Reuse the shipped ended overlay in `PassthroughTerminal`, including the phone
terminal panel reached through its existing bottom navigation. No new controls,
layout, focus movement, scroll owner, or responsive preferences are introduced.

## Validation

Run gateway race tests, terminal lifecycle unit tests, frontend typecheck/lint,
desktop and phone managed E2E with retries disabled, documentation checks,
and normal commit hooks before PR publication.
