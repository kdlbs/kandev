---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001
system_design:
  - ../../specs/platform/system-design/windows-background-process-console.md
---

# Keep background helpers hidden with Windows Terminal

Reuse the existing conditional console flag for agentctl launch, managed
processes, ACP utility commands and Cursor native MCP commands. Preserve caller
flags, stdio, suspension, job ownership and existing termination fallbacks.

## Delivery and boundaries

- [Implementation and verification](task-01-no-console-window-flag.md)
- [System design](../../specs/platform/system-design/windows-background-process-console.md)

Managed Git, PTY, named-job ownership, suspended-child resume and cooperative
backend shutdown changes are outside this slice. Existing public PRs own those
features. This slice adds no runtime toggle or permanent test case.

## Verification strategy

Run the affected Windows creation-flag fixtures with temporary console-present
and console-absent probes. Use a detached parent to enter the actual managed
child path and capture its hidden-window and stdout result. Reuse selected
Windows job lifecycle checks and compile the four caller packages for Linux.
The Windows five-package selection and Linux caller compilation passed.
The linked work order records the exact scope and measured limits.

The source design explicitly leaves the WM_CLOSE effect for a windowless helper
unmeasured. Retaining existing stdin-close, grace, force and Job Object code is
not a claim that a windowless helper receives the same close request.
