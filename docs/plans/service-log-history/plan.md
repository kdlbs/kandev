---
created: 2026-10-28
status: implemented
requirements:
  - REQ-CLI-SERVICE-LOG-HISTORY-001
system_design:
  - ../../specs/cli/system-design/service-log-history.md
legacy_specs: []
---

# Implementation plan: Service log history

## Overview

Deliver one vertical CLI slice: validate a bounded log count at the native
entrypoint, carry it to both supported service managers, and document its use.
Implement only after the separate design acceptance and execution release.

## Scope

### In scope

- Log-only count forms, bounds, zero, default, and existing service options.
- Controlled command-executor tests and public CLI and service-guide examples.

### Out of scope

- New log filters or formats, service changes, configuration or Makefile
  settings, unsupported platforms, rendered UI and browser E2E.

## Technical approach

In `apps/backend/internal/launcher/service.go`, initialize an explicit default
in `parseServiceArgs`, validate the count before dispatch, and pass it through
`buildJournalArgs` and `runLaunchdService`. Preserve current flag and argv
ordering where applicable. Adapt direct `serviceArgs` constructions in
`service_test.go` where necessary and place new tests in `service_logs_test.go`
to respect file and function limits. Use the existing command executor seam;
no real journal, tail, service installation, privileges or log reads.

Update the service section in `docs/public/cli.md` (CLI reference) and
`docs/public/run-as-a-service.md` (how-to). The latter must explain that macOS
counts are per file and that a custom home must still be repeated. Update CLI
help in the same implementation. Existing operations guidance remains valid.

## Tests

- AC 001.1 and 001.2: table tests at the public `Run` entrypoint for all forms,
  boundaries, repeat behavior, missing and invalid inputs, and non-log actions;
  assert usage exit and no command execution on failures. Establish a failing
  test before implementing.
- AC 001.3 and 001.4: capture exact Linux and macOS argv using controlled
  execution, with ordinary default, user/system, follow, zero and custom home
  cases. Exercise the actual platform handlers on any test host and retain the
  existing journal argument controls.
- AC 001.5: inspect help and run public documentation validators after examples
  and reference text are updated.

## E2E tests

The CLI entrypoint and actual OS handler paths with injected execution provide
end-to-end command-routing evidence for AC 001.1 through 001.4; no browser
or live manager E2E applies.

## Work orders

- [ ] [Task 01: Service log history](task-01-service-log-history.md)

## Verification results

The entrypoint regression failed before implementation and passed afterward.
The focused launcher race suite, changed Go lint, specification checks, and
public documentation validators passed. See the work order for commands and
results; hosted checks and review are tracked separately.

## Risks

- Treating zero as an unset value would break zero-history follow. Preserve an
  explicit parsed default and test direct handler inputs deliberately.
- Tail applies the count per file, so macOS may show up to twice the selected
  history; documentation must state this.
- `docs/public/cli.md` has a known concurrent change in a different maintenance
  section. Recheck fresh complete PR overlap before publication and preserve
  concurrent edits; do not claim the entire file has no overlap.
