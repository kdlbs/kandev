---
id: "02-retire-deleted-step-identifiers"
title: "Retire the three step identifiers whose work upstream deleted"
status: done
wave: 2
depends_on:
  - "01-step-registry-and-surfaces"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-005
acceptance_criteria:
  - AC-PLATFORM-STARTUP-PROGRESS-005.3
  - AC-PLATFORM-STARTUP-PROGRESS-005.6
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
---

# Task 02: Retire the three step identifiers whose work upstream deleted

## Summary

Upstream `2eee90d38` ("test: close conversation storage follow-up gates", #3742)
landed on `origin/main` the same day Task 01 shipped and deleted, in full, the
work that three of Task 01's registered steps measure:
`apps/backend/internal/task/repository/sqlite/conversation_journal.go` (945 lines,
including `backfillConversationJournal` and both pairs of corpus/outstanding count
helpers) and `apps/backend/internal/plugins/conversation_journal.go` (721 lines,
including `syncAllCommittedSessionEvents`, whose `provider.go` call site was
removed with no replacement). The successor `conversation_session_revisions` table
is trigger-maintained and treats revision zero as correct for an unchanged session,
so it has no batch backfill to measure, and `cleanupLegacyConversationJournal`
(`base_schema.go:65`) drops the journal tables in one transaction of DDL with no
row loop and no unit to count.

The spec was amended on 2026-09-18 accordingly: AC-PLATFORM-STARTUP-PROGRESS-005.3
now enumerates seven identifiers, and AC-PLATFORM-STARTUP-PROGRESS-005.6 names the
three retired ones. This task brings the branch's code to that contract so it can
rebase onto `origin/main`.

## In scope

- Delete `StepJournalTurnsBackfill`, `StepJournalMessagesBackfill`, and
  `StepPluginsSessionEventsMirror` and their registry entries from
  `internal/startup/step.go`, and add all three identifiers to the append-only
  retired list the `init()` guard reads.
- Delete this branch's own copies of the now-dead call sites and their helpers,
  which the rebase will otherwise reintroduce alongside files upstream deleted:
  `beginCountedJournalBackfillStep` and the count helpers in
  `internal/task/repository/sqlite/conversation_journal.go`, and the mirror's
  `BeginStep`/`Advance`/`EndStep` calls in `internal/plugins/conversation_journal.go`.
  Both files are deleted outright on `origin/main`; take the deletion.
- Update the registry completeness tests' expected identifier set and count
  (10 to 7) in `internal/startup/registry_completeness_test.go`, and extend
  completeness test 5 to assert the three real retirements rather than only its
  throwaway-registry stand-in.
- Delete `conversation_journal_steps_test.go` and the three label keys
  (`startup.step.journal_turns`, `startup.step.journal_messages`,
  `startup.step.plugin_session_events`) from every backend and web locale catalog,
  so AC-PLATFORM-STARTUP-PROGRESS-005.4's label check and `i18n:check` both stay
  clean with no orphan keys.
- Rebase the branch onto `origin/main` once the above is committed.

## Out of scope

- Registering a step for `cleanupLegacyConversationJournal`. It is DDL in one
  transaction with no countable unit; the exclusion is recorded in the system
  design. A later change that gives it a row loop is what the registry checks
  exist to catch.
- Reinstating any deleted upstream mechanism to keep an identifier alive.
- `SeedDone` and the `Outstanding`/`Order` registry fields. They stay, unexercised
  by any registered step, because AC-PLATFORM-STARTUP-PROGRESS-001.8 and 001.21
  still bind the next step that resumes or rides an ordered loop. Their unit tests
  in `internal/startup` are unaffected by this task and must stay green.

## Acceptance

- `Registry()` holds exactly the seven identifiers AC-PLATFORM-STARTUP-PROGRESS-005.3
  lists, asserted in both directions.
- Registering any of the three retired identifiers panics at `init()`.
- No reference to `backfillConversationJournal`, `syncAllCommittedSessionEvents`,
  `conversation_turn_versions`, `conversation_message_versions`, or
  `conversation_session_streams` survives in `internal/startup` or in this branch's
  step instrumentation.
- The branch rebases onto `origin/main` with no modify/delete conflict on either
  `conversation_journal.go`.

## Validation

```bash
cd apps/backend && go test ./internal/startup/... ./internal/task/repository/sqlite/... ./internal/plugins/... -count=1
cd apps/backend && make lint
cd apps/web && pnpm run i18n:check
```

## Files likely touched

- `apps/backend/internal/startup/step.go`, `registry_completeness_test.go`
- `apps/backend/internal/task/repository/sqlite/conversation_journal.go` (deleted upstream), `conversation_journal_steps_test.go`
- `apps/backend/internal/plugins/conversation_journal.go` (deleted upstream)
- `apps/backend/internal/i18n/locales/*.json`
- `apps/web/src/locales/*/startup.json`
