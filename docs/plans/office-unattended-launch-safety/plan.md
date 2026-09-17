---
created: 2026-09-17
status: done
requirements:
  - REQ-OFFICE-LAUNCH-SAFETY-001
  - REQ-OFFICE-LAUNCH-SAFETY-003
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
  - ../../specs/office/system-design/unattended-launch-safety-02.md
---

# Implementation Plan: Office Unattended Loop Launch Ceilings and Causation Depth

## Overview

Bound how many Office agent processes may run at once and how deep a chain of
agent-triggered launches may go, so an unattended loop with no human present
cannot saturate the machine or run away. This delivers the two "at an instant"
controls from
[Office Unattended Launch Safety Requirements](../../specs/office/requirements/unattended-launch-safety.md):
claim-time concurrency ceilings (instance, workspace, and agent) and an
enqueue-time causation-depth refusal, both attached to the authoritative
enqueue/claim seams so no insert or claim path can bypass them.

## Scope

- Evaluate instance, workspace, and agent claim ceilings atomically with the
  `queued` to `claimed` transition, on every supported database engine.
- Resolve the effective agent ceiling from `agent_profiles.max_concurrent_sessions`,
  clamped to at least `1` and to the configured workspace/instance ceilings.
- Defer rather than claim when a ceiling input is unreadable, and record the
  failure through the launch-backpressure gate-failure telemetry.
- Track causation depth end to end (routine fire, task-boundary carrier,
  runtime actions, approvals, workflow steps) and refuse enqueue past the
  configured maximum without consuming the request's idempotency key.
- Serialize the depth check, self-trigger window counts, idempotency
  resolution, and the insert in one transaction per the authoritative enqueue
  seam.
- Consolidate every direct run-insert call site onto the authoritative
  `runs/service` enqueue path so the ceilings and depth gates cannot be
  bypassed by a narrower seam, and guard that consolidation with a structural
  regression test.

## Implementation Wave

- [x] [task-01-launch-safety-ceilings-and-depth](task-01-launch-safety-ceilings-and-depth.md)

## Verification

```bash
cd apps/backend && env -u KANDEV_HEALTH_TIMEOUT_MS go test \
  ./internal/office/scheduler ./internal/office/service ./internal/office/shared \
  ./internal/runs/service ./internal/runs/repository/sqlite ./internal/workflow/engine
python3 scripts/lint-spec-files.py --all
```
