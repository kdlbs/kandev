---
status: done
created: 2026-11-16
requirements:
  - REQ-OFFICE-STALL-VISIBILITY-001
  - REQ-OFFICE-STALL-VISIBILITY-004
system_design:
  - ../../specs/office/system-design/stall-visibility.md
---

# Office dynamic successor completion

## Overview

An Office task whose first dynamic candidate failed was relaunched on a
successor that had no Kandev turn. The successor's completion carried no turn
ID, `on_turn_complete` was deferred, and the `IDLE` Office session sat outside
every reconcile path, so the task stopped silently with its step-completion
signal unapplied.

## Technical approach

Bind the successor prompt to a turn. Bind a turn-less delivery terminal to the
turn its handler closes only after its prompt generation still owns the
session. Save that binding with the exact workflow receipt after the repository
checks the delivery identity and cursor. Report any completion that is still
deferred. The report never advances an Office step.

## Waves

Wave 1:

- [x] [Task 01: Keep the dynamic successor's completion applicable](task-01-successor-turn-and-deferred-surface.md)
