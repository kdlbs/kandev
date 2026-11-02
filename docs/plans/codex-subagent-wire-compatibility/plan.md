---
status: implemented
requirements:
  - REQ-AGENTS-CODEX-SUBAGENT-WIRE-001
system_design:
  - ../../specs/agents/system-design/codex-subagent-wire-compatibility.md
---

# Codex Subagent Wire Compatibility Plan

Restore the existing recognition contract for Codex ACP 2.2.2 without a runtime
downgrade. The user explicitly requested reporting and repairing this regression.
Upstream report: https://github.com/kdlbs/kandev/issues/4953.

## References

- [Requirements](../../specs/agents/requirements/codex-subagent-wire-compatibility.md)
- [System design](../../specs/agents/system-design/codex-subagent-wire-compatibility.md)

## Delivery

| Work order | Status | Dependencies |
| --- | --- | --- |
| [01: Restore wire recognition](task-01-restore-recognition.md) | done | None |

Execute sequentially in the primary session. No implementation delegation.

## Verification and risks

The work order owns RED/GREEN evidence, the affected ACP package race run,
changed-file Go lint, and specification validation. No browser/UI code changes
are planned. Existing chat components consume the restored payload.

Strict input validation must not lose the empty receiver list sent before spawn
completion. All signal consumers must agree, including capacity continuation.
Native child transcripts, data backfills, runtime upgrades, installation, and
deployment are excluded. Full live application validation is reported separately
from producer replay and package tests.
