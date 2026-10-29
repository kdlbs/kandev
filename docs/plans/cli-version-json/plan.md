---
created: 2026-10-28
status: implemented
requirements:
  - REQ-CLI-VERSION-JSON-001
system_design:
  - ../../specs/cli/system-design/version-json.md
legacy_specs: []
---

# Implementation Plan: Native CLI version JSON

## Overview

Add the opt-in machine-readable version path as one sequential vertical slice.
First prove the missing behavior through `launcher.Run`, then implement and
document it, and validate the adjacent contracts before delivery.

## Scope

### In scope

- Version JSON invocation, stamp fallback, output failure handling and help.
- Focused native launcher, parser, help and maintenance regression evidence.
- A concise example and field descriptions in the public CLI reference.

### Out of scope

Release and artifact behavior, backend metadata APIs, service and maintenance
behavior, runtime startup changes, UI and historical CLI documentation drift.

## Technical approach

Extend `internal/launcher/options.go` for a version-only JSON option and
`launcher.go` for the early JSON exit using `BuildInfo`; use the existing
normalization and narrow broken-pipe convention. Keep service and maintenance
dispatch intact. Update `internal/launcher/cli/help.go` and the option section
of `docs/public/cli.md`. The npm shim and producer already pass the required
data; they do not need a redesign.

## Tests

`version_json_test.go` exercises the real `Run` entrypoint, both flag orders
and aliases, exact JSON bytes and decoded escaping, stamps and fallbacks,
plain compatibility, standalone rejection and absence of startup side effects
(AC-CLI-VERSION-JSON-001.1 through .4). Writer error and short-write tests plus
an isolated Unix broken-pipe subprocess cover .5. Existing parser, help,
version, main dispatch and maintenance JSON tests cover adjacent boundaries;
the work order lists exact commands. A stamped native binary smoke check
confirms the public invocation without UI or browser tests.

## E2E tests

The native executable smoke check and actual `Run` tests cover the public CLI
flow. No Playwright or visual UI is involved.

## Work orders

- [x] [Task 01: Implement native version JSON](task-01-version-json.md)

## Verification results

The causal `Run` RED, targeted launcher and adjacent tests, changed-Go lint,
stamped native build and smoke check, specification catalog and lint, public
documentation validators and local PR documentation coverage preflight passed
after the implementation. Details and logs are recorded in the work order.
Normal commit hooks and hosted checks are delivery gates, not yet recorded here.

## Risks

- JSON output failures must be observable without changing plain-version
  SIGPIPE behavior; process-global signal tests need subprocess isolation.
- Concurrent work may touch CLI help or public docs; preserve unrelated edits
  and refresh complete open-PR overlap before publication.
