---
id: "01-end-removed-targets"
title: "End connections to removed terminal environments"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TERMINAL-TARGET-001
acceptance_criteria:
  - AC-TASKS-TERMINAL-TARGET-001.1
  - AC-TASKS-TERMINAL-TARGET-001.2
  - AC-TASKS-TERMINAL-TARGET-001.3
  - AC-TASKS-TERMINAL-TARGET-001.4
  - AC-TASKS-TERMINAL-TARGET-001.5
system_design:
  - ../../specs/tasks/system-design/terminal-target-lifecycle.md
---

# End removed terminal targets

## Scope

Own gateway missing-environment handling, the shared terminal reconnect loop,
identity-scoped unavailable state, and desktop/phone regression coverage.
No live task, journal, recovery-block, or environment mutation is authorized.

## Verification

- Gateway tests: a real WebSocket receives the normal missing-environment close;
  plain HTTP receives 404; temporary lookup errors remain 503.
- Frontend tests: permanent outcomes stop retries, ordinary closes retry, and
  replaced sockets cannot tear down their successors.
- E2E: delete a disposable test task while withholding application updates and
  dropping its terminal connection. The retained desktop/phone pane receives
  the explicit missing-target close, shows ended feedback, and then
  issues no connection requests beyond the existing five-second retry cap.
- Typecheck, scoped lint, docs validation, and normal commit hooks.

## Results

RED: the gateway regression failed with a bad handshake; the browser regression
made six connection attempts instead of one. Both now pass with the repair.
GREEN: gateway race tests and 36 browser unit tests passed. Desktop Chromium
and mobile Chrome E2E passed with retries disabled against freshly built
production code. Both observed the real `1000/environment_not_found` close,
the ended overlay, and no reconnects beyond the retry cap. The fixture blocks
both WebSocket updates and HTTP reconciliation to retain the stale pane.

Frontend typecheck, scoped Go/TypeScript lint, and documentation validation
passed. Desktop and phone screenshots were captured from isolated synthetic
tasks. Public documentation needs no change: existing terminal controls and
localized ended feedback remain the user-facing contract.
