---
status: done
created: 2026-10-23
requirements:
  - REQ-EXECUTORS-RUNTIME-RECLAIM-003
system_design:
  - ../../specs/executors/system-design/agent-runtime-reclamation.md
---

# Read-only runtime footprint projection

Deliver only the existing observation requirement through the process owner,
authenticated control API, lifecycle reader, workspace route and settings card.

## Delivery and dependencies

- [Footprint implementation and verification](task-02-runtime-footprint-projection.md)

This observation slice is independent of terminal reclaim, G1/G2 capacity,
inference inventory and Office policy. Existing owner/authentication/repository
contracts are reused. There is no dependency on the separate terminal-reclaim
work order and no operating activation.

## Affected UI

```text
Workspace settings
  idle-suspension settings (existing)
  Agent memory in use (read only)
    sessions | processes | committed | resident
    session rows; loading / unknown / failed / empty / partial states
  placement and team access (existing)
```

The same card wraps into two columns on a narrow screen. It owns no save draft,
poller or running-session control. Focused Windows race checks, the actual
authenticated control and workspace GET routes, frontend controls and the
OS-reader cross-compiles passed as recorded in the work order. Normal hooks are
required before publication; no operating activation is included.
