---
status: done
---

# Task 02: Aggregate ring-buffer overflow warnings

## Objective

Prevent sustained event-buffer pressure from producing one warning per dropped
event while retaining enough information to diagnose the pressure.

## Scope

- `apps/backend/internal/plugins/delivery/deliverer.go`
- Related delivery tests and logger observers.

## Requirements

- Keep the ring-buffer size, TTL, oldest-drop policy, and delivery order intact.
- Rate-limit overflow warnings independently per plugin to one per 60 seconds.
- Log the first drop immediately; aggregate suppressed drops and include
  `dropped_count` in the next warning.
- Use an injectable clock so tests do not sleep.

## Test-first acceptance

- One drop emits one warning with `dropped_count=1`.
- More drops for the same plugin inside the interval emit no additional warning.
- The next drop after the interval emits one warning with the accumulated count.
- Two plugins have independent warning windows and counters.
- Existing ring-buffer recovery/TTL tests remain unchanged in behavior.

## Dependencies

None. This task may land before task 01.
