---
id: "03-queued-mentions"
title: "Queued references and edit admission"
status: done
wave: 3
depends_on:
  - "02-direct-mentions"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-001
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-006
acceptance_criteria:
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.5
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-001.6
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.4
system_design:
  - ../../specs/integrations/system-design/task-change-registration.md
---

# Task 03: Queued references and edit admission
## Summary

Extend selected-reference registration to ordinary queue insertion, automatic folds, and committed user edits.
Reuse the direct-message infrastructure and make every queue transformation preserve intent identity without generating links from replay.

## In scope

- Write intents inside queue row/receipt and lease-guarded edit transactions.
- Include intent in fingerprints; preserve source IDs through merge, clear, removal, reservation, dispatch and Send now.
- Define Save deltas from committed reference and selection IDs. Reselection and context-only-to-link generate new IDs, even with identical final text.
- Preserve IDs on unchanged edits, retry, fold, dispatch, and viewport changes. Retain omitted/empty reference-array update semantics.
- Enable Link on send for queue submission and Link on save in the editor; expose status and retained-link disclosure.
- Test question barriers, cancelled/stale session incarnations, edit leases, queue recovery, and multiple-session events.
- Record queue validation results and reconcile companion contract links. Leave shared specs draft until Task 04 also passes.

## Out of scope

New queue scheduler, changing Auto-run/Auto-merge policy, implicit unlink on removal, agent wake, and unrelated queue redesign.

## Acceptance

1. Queue acceptance and successful edit Save create exactly the intended registration receipts; cancelled, stale, or rejected operations create none.
   After unlink, remove-and-reselect followed by Save permits relink. Unchanged Save and replay remain suppressed.
2. Fold/dispatch/replay never restore a detached link or duplicate intent; removal preserves accepted association intent and existing linked siblings.
3. UI-04 and shared direct-message states remain truthful on desktop/phone and preserve queue controls, barriers, focus, and localization.

## ASCII UI preview

UI-04, [full preview](plan.md#ui-04-queue-editor-and-accepted-link-disclosure):

```text
Queued: [repo-a #123] Linked PR #123 [Edit] [Remove]
Edit:   [repo-b #124 | Link on save v]
Links already added stay linked until you unlink them.
                                  [Cancel] [Save]
```

Phone wraps chips within the existing queue edit surface and retains touch-accessible Cancel/Save.
Reselection after unlink shows Link again on save in the desktop control and phone drawer. Cancel discards that new selection ID.
Maps to 001.2-001.5, 005.3 and 006.4. Cancel changes no association; Save does not change Auto-run.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator ./internal/task/handlers ./internal/task/repository/sqlite -run 'Registration|Queue|InitialTaskBrief|EntityReference' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race -tags fts5 ./internal/orchestrator/messagequeue ./internal/task/repository/sqlite -run 'Postgres.*Registration|Registration.*Postgres' -count=1 -v)
(cd apps/web && pnpm exec vitest run hooks/domains/session/queue-registration.test.ts components/task/chat/reference-registration.test.tsx lib/entity-references/message-references.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/pr-mention-linking.spec.ts tests/chat/entity-reference-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/chat/mobile-pr-mention-linking.spec.ts tests/chat/mobile-entity-reference-composer.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new queue-registration tests must cover the matrix's queue crash/replay and edit races in both database modes.
Add `TestRegistrationQueueReselection` with unchanged Save, remove-and-reselect, link/context/link, Cancel, and replay after a second unlink.
Assert exactly one fresh intent for each accepted new selection. Use the same final text/reference values for unchanged and reselection cases.
Run those user flows on desktop and phone, including reload and breakpoint changes while the edit remains unsaved.
Record rendered parity against UI-04, plus actual provider count and unchanged automation/comparison state.
Re-run only affected Task 01/02 checks if shared code changes. Record every required command result before changing status.

## Files likely touched

- `apps/backend/internal/orchestrator/handlers/queue_handlers.go` and new `queue_registration_test.go`.
- `apps/backend/internal/orchestrator/messagequeue/` admission receipts, edit leases, repository transactions, merge/send-now metadata, and tests.
- `apps/backend/internal/orchestrator/event_handlers_agent.go`, `queue_send_now.go`, and metadata carry-through tests.
- `apps/backend/internal/task/repository/sqlite/` shared intent transaction helpers.
- `apps/web/hooks/domains/session/use-queue.ts`, queue API/request fingerprint helpers, and new queue-registration tests.
- Queue editor/row components, shared reference controls, draft serialization, and locale catalogs.
- Existing and new desktop/mobile E2E files named in Verification.
- Queue admission/edit and entity-reference contract cross-links; package Results/status fields after validation.

## Dependencies

Task 02, including its durable result/event/read and retry/relink infrastructure.

## Risks

Queue admission identity differs from the surviving row identity after folds.
A saved edit must compare against the committed revision, not the original admission snapshot.
A replayed receipt must neither create a phantom row nor revive old association intent.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-registration.md), [design](../../specs/integrations/system-design/task-change-registration.md), and [matrix](validation-matrix.md).
- Existing queue admission receipts, edit lease/revision tests, and entity-reference composer E2E fixtures.
- Root/backend/web/chat guidance and mobile-parity skill.

## Results

Task 03 is complete. Queue admission and lease-guarded edit Save now persist registration intents atomically with their queue receipt or edit revision. Selection IDs and admission origins survive replay, automatic folds, dispatch, Send now, and status projection; Cancel and removal do not create or erase accepted intent. Queue routes that cannot persist registration remain context-only.

The following targeted checks passed:

- Backend queue and task registration suites passed for SQLite and PostgreSQL, including admission replay, producer rollback, unchanged Save, reselection, fold, dispatch transfer, clear, Send now, and queue status projection.
- Race-enabled PostgreSQL registration tests passed for the queue, task repository, and backend application packages. Multi-connection message metadata serialization also passed.
- `make build` and `make lint` passed in `apps/backend`.
- 175 focused frontend tests passed across ten files; `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- The production Vite build passed. Chromium E2E passed 9/9 and mobile Chrome passed 4/4 against the rebuilt bundle, including queued reselection, unchanged prompt text, and edit recovery through reload and breakpoint changes.
- Public documentation, specification catalog/lint, and whitespace checks passed; full implementation evidence is recorded in [validation evidence](validation-evidence.md).

PR review fixup also passed the post-rebase checks recorded in [validation evidence](validation-evidence.md). The branch rebased onto current `main`. The managed desktop and mobile queue session-switch tests both pass with restored edit drafts. The PostgreSQL DSN was unavailable for this pass; the earlier disposable PostgreSQL 16 evidence remains recorded above.

Task 04 and shared requirement/design promotion remain pending.
