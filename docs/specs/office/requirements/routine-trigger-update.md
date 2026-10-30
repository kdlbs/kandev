---
status: active
system: office
created: 2026-10-29
owners:
  - kandev
---

# Office Routine Trigger Update Requirements

## Overview

A routine schedule change was expressed on the wire as creating a replacement
trigger and deleting the old one. That replaced the trigger identity, discarded
its creation time and fire history, and made a scheduler-claimed cron slot
awkward to edit without disturbing the claimed fire.

Operators need to change a trigger's schedule or enabled state in place through
the routine API, while the scheduler keeps ownership of a claimed fire and its
re-arm.

## Requirements

### REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001: Update an existing routine trigger in place

**Intent:** Let a caller change an existing trigger's schedule fields without
replacing the trigger or racing a scheduler claim.

**User story:** As an operator, I want to correct a routine's cron expression,
timezone, or enabled state without losing its identity or fire history, so that
a claimed fire is not disturbed.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-TRIGGER-UPDATE-001.1:** An update to an existing trigger
  shall change only `cron_expression`, `timezone`, and/or `enabled`. The trigger
  ID, routine, kind, webhook authentication, creation time, last fire, and past
  runs shall remain unchanged. Schedule fields apply only to cron triggers. The
  update shall compare the observed trigger identity and schedule, including the
  observed `next_run_at` (as a null or non-null value), with the stored row, and
  shall return 409 when the stored row no longer matches the observed values,
  including when a scheduler dispatch claimed the trigger or another schedule
  edit won the race. Invalid cron expressions or timezones shall return 400.
  Disabling a trigger clears its next fire but does not cancel an already
  claimed fire.
