---
status: active
system: cli
created: 2026-10-28
owners:
  - kandev
---

# Native CLI version JSON requirements

## Overview

Scripts and operators need the native CLI's build identity without starting
Kandev or parsing a startup banner. The CLI owns this invocation and output.

## Requirements

### REQ-CLI-VERSION-JSON-001: Machine-readable native version

**Intent:** Expose the existing build identity as a small, opt-in JSON object
while preserving the existing plain version contract.

#### Acceptance criteria

- **AC-CLI-VERSION-JSON-001.1:** `kandev --version --json` and `kandev -V
  --json` shall accept either flag order and, on success, emit exactly one
  newline-terminated JSON object on stdout with exactly three string fields:
  `version`, `commit`, and `build_time`. They shall exit 0 without a banner.
- **AC-CLI-VERSION-JSON-001.2:** `version` shall equal the ordinary normalized
  `--version` value, including `dev` for an empty version. `commit` and
  `build_time` shall preserve supplied stamps verbatim, using `unknown` for
  each empty stamp.
- **AC-CLI-VERSION-JSON-001.3:** Plain `--version` and `-V` shall retain their
  existing stdout bytes, exit behavior and development fallback. Version output
  shall not read configuration or application data, access a network, or launch
  a backend or another process.
- **AC-CLI-VERSION-JSON-001.4:** In the ordinary launcher, `--json` without
  `--version` or `-V` shall produce a usage error and exit 2 before startup.
  The independently dispatched `maintenance database --json` contract remains
  unchanged.
- **AC-CLI-VERSION-JSON-001.5:** A failed or incomplete JSON stdout write shall
  report a diagnostic on stderr and exit 1. Help and public CLI documentation
  shall explain the JSON option, command and field meanings.

## Out of scope

Backend metadata APIs, release stamps and channels, artifact layout, signing,
startup policy, service and maintenance behavior, and a general JSON option
or parser redesign are excluded.
