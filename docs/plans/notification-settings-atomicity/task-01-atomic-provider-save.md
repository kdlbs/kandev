---
id: "01-atomic-provider-save"
title: "Save notification provider configuration atomically"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.6
  - AC-PLATFORM-NOTIFICATIONS-001.9
  - AC-PLATFORM-NOTIFICATIONS-001.10
system_design:
  - ../../specs/platform/system-design/notification-settings-atomicity.md
---

# Task 01: Save Notification Provider Configuration Atomically

## Summary

Prove the rejected-save defect using the real SQLite store, then make provider
fields and subscriptions commit together. Preserve defaults, event omission,
explicit empty events, and tenant ownership through the actual backend boundaries.

## In scope

- Pre-write input validation; atomic create/update repository operations and shared
  SQL helpers; default Local/System provider creation.
- Permanent invalid-input and injected subscription-failure regressions for create
  and update, plus successful saves and omission/empty/ownership checks.
- Update notification repository test doubles for the atomic interface.
- Minimal public notification documentation explaining failed-save preservation
  in `docs/public/developer-tools.md` (how-to section).

## Out of scope

Schema changes, UI changes, delivery behavior, cross-request batch atomicity,
new authentication boundaries, unrelated refactors, and additional workers/tasks.

## Acceptance

1. Rejected update preserves provider fields, timestamps, and all subscription
   rows; failed create leaves neither provider nor subscriptions. The new
   regressions fail on main for the confirmed cause before implementation.
2. Successful create/update atomically persist the selected configuration, omitted
   updates preserve subscriptions, and explicit empty updates clear them. Default
   creation can retry after a subscription failure without skipping an orphan row.
3. Foreign/missing update fails before subscription mutation; targeted tests and
   persistence/documentation checks pass, with exact results recorded here.

## Verification

```bash
(cd apps/backend && go test -race ./internal/notifications/service ./internal/notifications/store ./internal/notifications/controller)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the new failing regressions alone before the production correction, recording
the expected mismatch. PostgreSQL behavior coverage is environment-gated on
`KANDEV_TEST_POSTGRES_DSN`; report availability explicitly. Do not overlap broad
suites; preserve every running command handle until completion.

## Files likely touched

- `apps/backend/internal/notifications/service/service.go`
- `apps/backend/internal/notifications/service/atomicity_test.go`
- `apps/backend/internal/notifications/service/service_test.go`
- `apps/backend/internal/notifications/service/ownership_test.go`
- `apps/backend/internal/notifications/store/interface.go`
- `apps/backend/internal/notifications/store/sqlite.go`
- `apps/backend/internal/notifications/store/sqlite_atomicity_test.go`
- `apps/backend/internal/notifications/store/sqlite_provider_save.go`
- `apps/backend/internal/notifications/controller/controller_test.go`
- `apps/backend/internal/notifications/controller/atomicity_test.go`
- `docs/public/developer-tools.md`

## Dependencies and parallelism

None. Sequential in this isolated workspace; preserve other work and refresh main.

## Inputs

- [Notification requirements](../../specs/platform/requirements/notifications.md).
- [Atomicity design](../../specs/platform/system-design/notification-settings-atomicity.md).
- Supplied investigation evidence: `invalid_events` and `subscription_write`
  changed the durable provider while returning errors on isolated SQLite.
- Existing controller event defaulting and service/store ownership tests.

## Risks

Nil/empty selection semantics, transaction ownership predicates, dialect rebinding,
and test fixture isolation. No unresolved material product choice.

## Results

Implemented atomic create/update and default-provider bootstrap with shared
transaction SQL helpers. All input validation precedes service writes. Permanent
SQLite regressions failed before correction on invalid-event update, subscription
write create/update, and Local/System bootstrap; the saved fields/timestamp changed
or orphan provider rows remained. The corrected regressions pass.

- `go test -race ./internal/notifications/service ./internal/notifications/store ./internal/notifications/controller`: passed all three packages after final test refactor.
- `go run ./cmd/sqlguard ./internal`: passed.
- `go test -race ./internal/persistence/storeconformance -count=1`: passed.
- `go test -v -run TestPostgresRepositoryAtomicProviderSave ./internal/notifications/store`: passed with the PostgreSQL test skipped. `KANDEV_TEST_POSTGRES_DSN` was unavailable; permanent isolated PostgreSQL coverage is included for rollback, ownership, successful saves, omission, and empty selection.
- `python3 scripts/list-docs.py validate`: passed (339 decisions, 1282 specifications at design checkpoint).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 passed.
- `node scripts/validate-public-docs.mjs`: 47 pages validated.
- `git diff --check`: passed.
- Repository documentation coverage preflight: covered, no errors.

Public docs updated in the existing Apprise notification how-to section. No UI
files changed, so browser screenshots and mobile layout checks are not required.
Node/pnpm resolved through the installed mise runtime; frozen workspace install
completed for normal commit hooks.

Local implementation is done. PR publication and exact-head CI/review/merge
remain externally pending. Commit/push and open a focused PR using local guidance;
remediate CI and all review threads, confirm exact-head checks and trusted semantic
reviews, and use normal merge/queue under existing authorization. Report the PR and
merged SHA to parent `14825981-b175-411d-999a-31ddc2aa5fc3`; do not declare completion
before merge or a concrete external blocker.
