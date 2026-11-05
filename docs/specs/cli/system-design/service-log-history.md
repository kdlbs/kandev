---
status: current
system: cli
requirements:
  - REQ-CLI-SERVICE-LOG-HISTORY-001
---

# Service log history system design

## Purpose and boundaries

The native CLI owns parsing and passes the selected count to the existing Linux
and macOS log commands. Service manager selection and home resolution remain
under their existing contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-CLI-SERVICE-LOG-HISTORY-001` | Data and contracts, control flow, failure and recovery |

## Components and responsibilities

`parseServiceArgs` and `parseServiceFlag` in `internal/launcher/service.go`
validate log-only options. `runLinuxService` and `buildJournalArgs` pass the
count to `journalctl`; `runLaunchdService` passes it to `tail` for both existing
log paths. `executeServiceCommand` is the existing injectable execution seam.

## Data and contracts

Initialize the count to 200 at parsing, independently of zero as an explicit
value. Validate a nonempty ASCII digit sequence before bounded integer
conversion, then pass a canonical decimal value as one argument. Support the
three specified spellings; apply repeated valid flags in argument order. Do not
interpret an option-looking token as a count. Keep direct constructed test
arguments aligned with the parser's explicit default rather than making zero a
default sentinel.

## Control flow

The public `Run` service route parses the full argument list before dispatch.
Linux keeps the existing user-unit versus system-unit arguments and follow or
no-pager behavior, with `-n` followed by the selected count. macOS keeps the
existing `-f` behavior and selected home resolution, passing `-n` and the count
once to `tail` with both files; tail applies that count to each file. No shell
command string or extra process is needed.

## Failure and recovery

Invalid, absent, or out-of-range values and use on other actions return the
existing usage-error path before any manager invocation. Existing command
execution failures and exit handling are unchanged. Operator help identifies
the supported forms and range.

## Persistence, security and observability

The selection is per invocation, with no persistence, configuration, new
privilege, telemetry, or log-content handling. Existing service-manager access
and process output behavior apply.

## Related contracts

The native service command and startup contract are described in
[Native Kandev CLI requirements](../requirements/native-kandev-cli.md).
