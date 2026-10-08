---
status: done
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/notification-settings-atomicity.md
legacy_specs: []
---

# Implementation Plan: Notification Settings Atomicity

## Overview and confirmed cause

Restore one atomic provider configuration save. At investigated and refreshed main
`08e4ffdb99caf40b0df5baa67b29cf4313188f15`, service update writes provider fields
before event validation and subscriptions. Creation also writes the row before
subscriptions. The store transaction currently protects only subscription
replacement. Supplied isolated SQLite evidence reproduced durable provider changes
on invalid events and subscription-write errors.

## Scope and approach

- Validate complete proposed input before writes.
- Add atomic repository create/update operations sharing transaction-aware provider
  and subscription SQL helpers; preserve ownership and optional events.
- Route service create/update and default-provider bootstrap through these saves.
- Recreate real SQLite regressions and update three notification repository test
  doubles for the new interface. Keep test additions in separate files where
  existing test-file size limits require it.
- Record the contract in notification requirements and the paired design.
  Pre-commit rejection or confirmed abort preserves the prior configuration; a lost
  PostgreSQL COMMIT acknowledgement has unknown outcome and requires reload or
  reconciliation. The entire configuration commits together; no retry redesign.

Exclude UI changes, schema migrations, multi-provider batch atomicity, delivery
behavior, new authorization rules, and unrelated refactors. This is a routine
transaction repair; the requirement, design, and tests retain the rationale without
a separate ADR.

## Tests and acceptance mapping

| Criteria | Permanent evidence |
| --- | --- |
| `AC-PLATFORM-NOTIFICATIONS-001.9` | `service/atomicity_test.go`: `TestNotificationRejectedUpdatePreservesConfiguration`, `TestNotificationFailedCreateLeavesNoProvider`; real SQLite, invalid events/config, trigger-induced insertion failure |
| `AC-PLATFORM-NOTIFICATIONS-001.9` | `store/sqlite_atomicity_test.go`: create/update rollback and successful complete save, default-provider retry coverage in service |
| `AC-PLATFORM-NOTIFICATIONS-001.6`, `AC-PLATFORM-NOTIFICATIONS-001.10` | Existing controller create event tests; controller update omission/empty tests, service valid/omitted/empty save tests, store/service foreign/missing ownership tests |

The real controller/service/store boundary provides end-to-end backend evidence
for the failed save; no rendered UI changes or artificial browser test are needed.

## Work orders

- [x] [Task 01: Save notification provider configuration atomically](task-01-atomic-provider-save.md)

One sequential work order. No implementation workers or additional persistent
tasks/sessions are authorized. The parent coordinates the required design handoff.

## Verification results

Implemented and passed the work order's targeted race tests, SQL guard,
store-conformance race suite, specification validators, public-doc validators
(62 tests, 47 pages), documentation coverage preflight, and whitespace check.
Permanent PostgreSQL behavioral coverage is present but skipped locally because
`KANDEV_TEST_POSTGRES_DSN` was unavailable. The invalid-event and subscription-write
regressions reproduced the original defect before production changes. See Task 01
for exact commands and detailed results. Publication/CI/review/merge remain pending.

## Risks

- A nil versus empty event selection must retain its meaning across layers.
- Ownership must gate subscription replacement inside the same transaction.
- Keep rebinding and writer/reader pools compatible with both supported dialects.
- Test-only triggers must be confined to temporary databases.
- Refresh main before publication/merge because other independent repairs run.
