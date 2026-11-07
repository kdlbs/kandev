---
created: 2026-10-30
status: in_progress
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Office, taskless, and native-route shared admission plan

Extend the one existing session-ceiling admission controller so Office
run-sessions, taskless runs, and native routes are admitted, counted, and
released through the same owner lock and reservation set as task launches, while
the existing waiter priority and the opt-in-off default are preserved.

- [Office and native-route shared admission](task-01-office-native-admission.md)
- [Native utility and review route ownership](task-02-native-route-consumers.md)

## Scope and risks

This delivery joins existing launch kinds to the current shared capacity
contract; it does not replace the admission seams, the task repository, or the
provider execution path. The optional policy stays off by default. Changes that
would alter disabled defaults, manual-launch behavior, workflow WIP limits, or
dynamic candidate ordering remain out of scope.

The port depends on native route producers owned by separate delivery groups. Where
those producers are not part of this slice, the corresponding admission entry
points stay unconnected and are reported as such rather than treated as finished.

## Verification strategy

Reuse the completed focused implementation checks recorded in the work order and
the existing documentation-coverage evaluator. Only remaining review fixes require
their affected checks to run again.
