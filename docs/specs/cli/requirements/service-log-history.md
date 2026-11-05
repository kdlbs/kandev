---
status: active
system: cli
created: 2026-10-28
owners:
  - kandev
---

# Service log history requirements

## Overview

Operators need to select how much service log history to inspect, including
following only new output. The CLI system owns this command's operator-facing
contract; the platform service managers retain their existing log sources.

## Requirements

### REQ-CLI-SERVICE-LOG-HISTORY-001: Select service log history

**Intent:** Let an operator choose a bounded history count without using a
manager-specific command.

#### Acceptance criteria

- **AC-CLI-SERVICE-LOG-HISTORY-001.1:** `kandev service logs` accepts
  `--lines N`, `--lines=N`, and `-n N`, where N is a nonempty sequence of ASCII
  decimal digits representing an integer from 0 through 1000000 inclusive.
  If omitted, the count is 200; if repeated, the last valid count is used.
- **AC-CLI-SERVICE-LOG-HISTORY-001.2:** Missing, negative, fractional,
  overflowing, out-of-range, or otherwise malformed counts produce a nonzero
  usage error before invoking a service manager. These options are rejected on
  service actions other than `logs`.
- **AC-CLI-SERVICE-LOG-HISTORY-001.3:** Linux user and system services apply
  the selected count to their existing journal source. macOS applies the count
  separately to each of its existing service output and error files. Existing
  user/system selection, custom home selection and ordinary default behavior
  remain available.
- **AC-CLI-SERVICE-LOG-HISTORY-001.4:** Both `-f` and `--follow` remain usable
  with any valid count, including zero to follow new output without history.
- **AC-CLI-SERVICE-LOG-HISTORY-001.5:** CLI help and public service documentation
  describe the forms, bounds, default, zero-follow example, and macOS per-file
  behavior.

## Out of scope

New time filters, JSON output, shell filtering, configuration, Makefile knobs,
service lifecycle changes, and additional operating-system support are excluded.
