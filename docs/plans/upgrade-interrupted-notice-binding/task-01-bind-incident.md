---
id: "01-bind-incident"
title: Bind the authenticated notice source at resolution
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Bind the authenticated notice source at resolution

1. Extend the existing shared absent-notice fixture in apps/backend/internal/task/repository/sqlite/native_resume_binding_test.go with a sole generic unresolved-work block. Read the committed pending marker and pass its parsed identity to VerifyCommittedRecoveryNoticeSource. Confirm the new assertion fails on both engines.
2. In native_resume_binding.go, use the same authenticated incident for the pending marker and its resolved source block. When a matching bound block already exists, retain its binding. When only a generic block exists, persist the unique proven submission and stream on resolution in the same transaction. Check affected rows. Preserve existing behavior for generic retirements without a publishable incident. Exercise ambiguous candidates and rollback. Keep the exact publication predicates in recovery_notice_source.go.
3. Verify that one interrupted submission produces a publishable marker, that independent or ambiguous evidence cannot authorize a submission-specific notice, and that a failed transaction leaves no partial binding. Run from apps/backend against a disposable PostgreSQL DSN in KANDEV_TEST_POSTGRES_DSN. Confirm both shared test entry points execute.

```sh
systemd-run --user --scope -p MemoryMax=8G go test -trimpath -v ./internal/task/repository/sqlite -run 'Test(Postgres)?NativeResume' -count=1
```

Use the v0.103.1 interrupted-session lifecycle scenario to check the PR candidate on PostgreSQL 16.16 and SQLite. The production Release upgrade regression workflow and scripts/upgrade/run.sh enforce main-only execution. Use a PR-compatible integration test entry point for premerge evidence. Record the candidate SHA, baseline, engine version, resume response and observed notice. Distinguish repository-level checks from full release rehearsals.

## Results

The new regression cases failed on both engines before the change. After the change, the full native-resume repository suite passed on SQLite and disposable PostgreSQL 17.11 (264 seconds under MemoryMax=8G). It exercised pending notice verification, ambiguous incidents, rollback and existing native recovery cases. Full release rehearsal evidence remains pending under the main-only workflow rule.

The legacy restart integration test includes multiple retired interrupted submissions in one generation. With no prior recovery snapshot, all authenticated work can retire and the generic block can resolve. The transaction leaves both delivery fields empty and emits no submission-specific marker. A historical terminal snapshot with one current interruption retains an exact binding.

After correcting the legacy-history case, the native resume and historical terminal repository suites passed on SQLite and PostgreSQL 17.11. A separate test verifies that ambiguous history with a saved notice still refuses resolution.

The historical-terminal matcher requires a bound submission ID. A generic companion cannot identify a later interrupted submission on its own. The transaction binds a sole generic block before matching. The historical tests cover both a settled earlier notice and an earlier incident that still lacks terminal settlement.
