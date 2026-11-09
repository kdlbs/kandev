---
title: "Shared circuit admission and health readback"
status: done
created: 2026-10-31
updated: 2026-11-01
requirements:
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
system_design:
  - ../../specs/agents/system-design/dynamic-circuit-health.md
---

# Shared Circuit Admission and Health Readback

Port the existing circuit core, bounded backoff, fixed-profile admission, and
health reader together onto current main. Preserve the current routing and
account-hold contracts. Adapt the original reader UI to the existing integrated
control providers dashboard without importing an unrelated aggregate.

## Work orders

- [Task 01](task-01-circuit-health.md)

The earlier mid-turn and backoff work packages describe original source work.
Their historical checks are not validation of this current-main port. The
current work order records the new integration checks and remaining limits.

The [single-flag circuit defaults package](../dynamic-circuit-defaults/plan.md)
removes the remaining backoff toggle. Its own work order records that change
and its verification. This package's recorded validation results remain intact.
