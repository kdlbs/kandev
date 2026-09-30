---
created: 2026-09-30
status: done
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
legacy_specs: []
---

# Implementation Plan: Stale stall notice

## Overview

Hide advisory inactivity notices when the same turn resumes producing agent
activity. The user authorized investigation, implementation, PR delivery,
and merge without a separate handoff checkpoint.

## Root cause

Commit `b734d3f8a5a517dec6a9b3c65b0e0995aaa90361` (PR #2035, July 29,
2026) introduced persisted running notices gated only by session state and
active turn identity. Completing compaction leaves both unchanged, so the
notice incorrectly continues to describe the earlier tool.

## Scope and technical approach

One sequential slice: derive subsequent agent activity from existing message
timestamps, subscribe the action renderer to that boolean, and cover live
updates and reload. No protocol, persistence schema, provider-specific logic,
or watchdog frequency changes. Ignore queued user and system status rows.
Amend AC-AGENTS-AGENT-STALL-RECOVERY-001.6 to capture resumed-turn visibility.

## ASCII UI preview

UI-01: Task chat, shared desktop and phone composition.

```text
Quiet running turn:
Agent is running
Still waiting on Compact conversation.  [Cancel turn]

Same turn after agent activity resumes:
Agent is running
<latest agent content or completed tool>
```

Only notice visibility changes. Phone uses the existing inline row and 44px
cancel target from `mobile-pause-resume-recovery.spec.ts`; chat retains its
scroll owner and navigation. Neither breakpoint behavior nor controls change.

## Tests and E2E

AC-AGENTS-AGENT-STALL-RECOVERY-001.6: existing action-message harness proves
the defect before the fix. Pure resolution tests cover message classes, a tool
created before the notice and updated afterward, timestamp precision, and
session/turn isolation. Desktop and mobile E2E show a running notice, update
the existing compaction row, verify the notice disappears while RUNNING, and
reload to verify persistence. Capture synthetic before/after evidence.

## Work orders

- [x] [Task 01: Resolve resumed-turn notices](task-01-resolve-notices.md)

## Verification results

- RED: the compaction-row live-update assertion failed in the existing
  action-message harness and in Chromium E2E before the production change.
- GREEN: targeted Vitest, 57 tests passed across two files.
- TypeScript typecheck and targeted ESLint passed with no errors or warnings.
- Desktop Chromium and phone mobile-chrome E2E each passed, including
  the same running turn before/after activity and after reload.
- Specification catalog validation, specification lint, and diff whitespace
  checks passed.
- Fresh synthetic quiet/resumed screenshots captured for both viewports;
  screenshot binaries are published only on a separate media ref.
- PR CI, automated review, and merge are tracked in the platform task plan.

Public documentation terminology and controls are unchanged. This repair is
recorded in the owning internal requirement and design.

## Risks

Using only creation timestamps misses updates to an existing tool. Resolving
from arbitrary system rows or queued input could hide a legitimate notice.
