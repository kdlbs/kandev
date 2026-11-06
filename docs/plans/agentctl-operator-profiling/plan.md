---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001
system_design:
  - ../../specs/platform/system-design/agentctl-operator-profiling.md
---

# Agentctl operator profiling delivery

Port the existing optional loopback listener from source `c2c48a65a`,
preserving the default-off environment setting and existing session APIs.

## Work order

- [Implementation and verification](task-01-loopback-listener.md)

## Verification strategy

Use the original address cases and an actual HTTP request through the startup
helper in a disposable explicit Go overlay. Reuse existing agentctl parent
liveness checks, then run normal repository commit hooks. No standing test,
new workflow, production setting, or unrelated runtime change is introduced.

## Observed validation

The original seven-address matrix and real helper HTTP request passed with race
detection, together with the three existing parent-liveness checks (1.315s).
The existing documentation coverage evaluator returned covered with no errors.
Normal commit hooks provide the repository lint and documentation checks.
