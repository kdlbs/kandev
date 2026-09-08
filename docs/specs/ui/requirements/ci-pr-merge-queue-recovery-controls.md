---
status: draft
system: ui
created: 2026-08-24
owners:
  - Kandev
---
# Merge Queue Recovery Controls Requirements

## Overview

The PR automation surface already separates repair from merge policy. Merge
queue recovery uses those controls because it has the same two decisions.

The surface must explain queue recovery without adding a third switch. It must
also show the current recovery state on desktop and mobile.

## Requirements

### REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001: Queue-aware automation controls

**Intent:** A user can understand and control queue recovery from the existing
per-PR automation surface.

#### Acceptance criteria

- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.1:** The automation surface shall use
  the existing auto-fix and auto-merge switches. It shall not add a separate
  queue-recovery switch.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.2:** The auto-fix control shall keep
  its current label. Its help shall state that actionable merge-queue failures
  can start an auto-fix round.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.3:** The auto-merge control shall use
  the label `Auto-merge or requeue when ready`.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.4:** The automation surface shall show
  one compact merge-queue status line for the selected linked pull request.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.5:** The status line shall distinguish
  an active queue entry, an actionable removal, an accepted repair request, a
  same-head wait, and a new-head check wait.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.6:** The information control shall
  explain the switch combinations and the same-head retry guard.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.7:** The task prompt editor shall
  explain that `{{pr.feedback}}` can include queue-removal context and
  available failed merge-group checks.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.8:** The desktop hover popover and the
  mobile PR status drawer shall show the same controls, status, and errors.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.9:** On a phone, each switch row shall
  keep a 44-pixel touch target. The status shall not cause horizontal document
  overflow.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.10:** The section title and subtitle
  shall distinguish normal automation, active merge-queue automation, and
  recovery after removal.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.11:** The two switch labels shall keep
  stable accessible names across queue states. Contextual supporting text may
  explain what each saved option will do next.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.12:** When the selected pull request is
  already queued, the surface shall explain that auto-fix watches for a future
  queue failure and auto-merge will not create a second queue attempt.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.13:** After removal, the surface shall
  explain whether Kandev will send the failure to the agent, wait for a new
  commit, wait for checks, or stop because the needed option is disabled.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.14:** The removal subtitle shall use a
  localized cause that Kandev classified. When no classified cause exists, it
  shall use a localized generic removal message.
- **AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.15:** The surface shall not show the
  raw provider reason as a localized label or use it as a switch name.

## Related requirements

- [GitHub PR Merge Queue Recovery](../../integrations/requirements/github-pr-merge-queue-recovery.md)
- [Task PR Automation Controls](ci-pr-automation.md)

## Out of scope

- A new queue-management panel.
- A manual requeue action in the automation surface.
- A per-PR auto-fix prompt override.
- GitLab control labels or behavior.
