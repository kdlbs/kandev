---
id: "02-direct-mentions"
title: "Selected direct-message references"
status: done
wave: 2
depends_on:
  - "01-agent-registration"
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

# Task 02: Selected direct-message references
## Summary

Deliver the selected-reference path from draft choice through durable direct/initial-message acceptance to provider lists and per-reference feedback.
Include crash recovery, explicit retry/relink, provenance, and desktop/phone access in this slice.

## In scope

- Add explicit reference intent and normalization/fingerprint handling; retain safe old-client and recalled-draft defaults.
- Carry selection IDs through draft, request, stored reference, and result DTOs. Preserve IDs on retries and viewport changes.
- Bind accepted selection IDs to task/reference/origin. Reject conflicting reuse and return the original intent on exact replay.
- Insert registration intents in direct/initial-brief transactions, including direct messages deferred to the queue.
- Add bounded post-commit processing, restart/read recovery, retry/relink endpoint, result reads and scoped events.
- Add chip mode controls and submitted states; integrate provider events/revisions with task badge/list and origin details.
- Add localization and desktop/mobile/keyboard tests. Extend task and Quick Chat shared composer paths where the existing picker is available.
- Link the new registration contract from the existing composer and initial-brief designs without duplicating their criteria.

## Out of scope

Ordinary queue insertion/edit-save producers, new mention-picker entry points, natural-language parsing, repository attachment, and new provider support.
Until Task 03, ordinary queue submission must visibly use context-only behavior; never display a linking promise on an unsupported path.
Direct messages that the server defers after acceptance retain this work order's registration behavior.

## Acceptance

1. Selected GitHub/GitLab references link only after initial/direct acceptance; context-only, literal text, and failed/cancelled admission never create intents.
2. Pending/success/failure/unlinked states survive retry/reload and origin deletion; task lists use committed revisions and stale events cannot restore a removed row.
3. UI-01, UI-02, UI-03 and UI-05 provide the same localized controls on desktop/phone, with keyboard focus and screen-reader feedback.

## ASCII UI preview

UI-01 / UI-02, desktop; [full preview](plan.md#ascii-ui-preview):

```text
Fix [repo-a #123 | Link on send v]     [Send]
    ( ) Context only
Sent: [repo-a #123] Linked PR #123     [PRs 1]
      [repo-b #124] Not linked [Retry] [Details]
```

UI-03 / UI-05, phone:

```text
[repo-a #123 v]
+------------------------------+
| (o) Link on send             |
| ( ) Context only      [Done] |
+------------------------------+
Original message unavailable
[Open PR] [Unlink]
```

Mode remains visible without hover. Phone drawer actions have 44-pixel minimum targets and safe-area clearance.
Maps to this work order's 001.1, 004.4, and 006.* criteria. Reuse existing tokens and primitives.

## Verification

Run once before frontend validation if dependencies are absent: `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/entityrefs ./internal/mentions ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'Registration|Reference|InitialTaskBrief|WSAddMessage' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race -tags fts5 ./internal/task/repository/sqlite -run 'Postgres.*Registration|Registration.*Postgres' -count=1 -v)
(cd apps/web && pnpm exec vitest run components/task/chat/reference-registration.test.tsx components/task/chat/tiptap-entity-reference-extension.test.ts components/task/chat/messages/entity-reference-chip.test.tsx lib/entity-references/message-references.test.ts lib/ws/handlers/github.test.ts lib/ws/handlers/gitlab.test.ts lib/state/slices/github/github-slice.test.ts lib/state/slices/gitlab/gitlab-slice.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/pr-mention-linking.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/chat/mobile-pr-mention-linking.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

New test paths are created by this work order. Include SQLite/PostgreSQL acceptance rollback, restart, and unlink race cases.
Add `TestRegistrationSelectionIdentity` for token binding, changed-payload reuse, and replay after unlink.
Frontend tests cover draft serialization, fresh selection, context-to-link toggles, recall defaults, and desktop/phone transitions.
Rendered checks must compare the control hierarchy with UI-01/02/03/05, including pseudo-locale and breakpoint draft preservation.

## Files likely touched

- `apps/backend/pkg/api/v1/` entity-reference DTO and `internal/entityrefs/reference.go` below `apps/backend`.
- `apps/backend/internal/task/handlers/message_handlers.go`, `message_handlers_initial_task_brief.go`, and new registration tests.
- `apps/backend/internal/task/service/`, `repository/sqlite/message*.go`, registration-intent repository, and retention tests.
- `apps/backend/internal/orchestrator/message_meta.go`, shared registration wiring and post-commit recovery.
- `apps/backend/internal/events/`, task routes/result DTOs, gateway event projection.
- `apps/web/lib/types/entity-reference.ts`, provider/backend DTOs, `lib/entity-references/`.
- `apps/web/components/task/chat/use-entity-reference-composer.ts`, `messages/entity-reference-chip.tsx`, `tiptap-entity-reference-extension.tsx`, message rendering, and new status controls.
- `apps/web/hooks/message-request.ts`, `use-message-handler.ts`, provider hooks, task list/details and source navigation.
- `apps/web/lib/ws/handlers/{github,gitlab}.ts`, provider store slices and new registration event handler.
- Locale catalogs and new desktop/mobile PR-mention E2E files listed above.

## Dependencies

Task 01. Consume its existing canonical resolver, transactions, policy and tombstone contract.

## Risks

A provider error must not reject an accepted message. Late success must not overwrite unlink or workspace-generation changes.
Keep acceptance-time initial-brief selection and title ownership intact. Do not add association behavior to task-description edits.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-registration.md) and [design](../../specs/integrations/system-design/task-change-registration.md).
- [UI preview](plan.md#ascii-ui-preview), [matrix](validation-matrix.md), and existing entity-reference composer tests.
- Root/backend/web/chat guidance and mobile-parity skill.

## Results

Task 02 is complete. Selected direct and initial-message references now persist admission intents, resolve after acceptance, expose per-reference state and retry/relink, and retain origin and provider-list feedback on desktop and phone.

The following targeted checks passed, including remediation of the five static review findings:

- Backend registration/reference tests across entity references, message handlers, task service, SQLite persistence, processor authorization/reconciliation, and GitHub/GitLab registration paths.
- PostgreSQL 16 registration, admission-fence, and concurrent message-metadata writer tests with `-race` against a disposable database.
- 209 focused frontend tests across eleven files, plus `pnpm run typecheck`.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- Desktop Chromium E2E passed for direct-send and initial-brief admission; mobile Chrome E2E passed for the registration drawer and accepted message. Both managed runs rebuilt the backend and Vite test assets.
- Specification catalog/lint, public-doc validators, and `git diff --check`; full results are in [validation evidence](validation-evidence.md).

Regression coverage confirms that pending work reauthorizes its original actor before provider access and mutation, startup reconciliation repairs a missed unlink event, association revisions reject stale browser updates/deletes through hydration, PostgreSQL serializes per-message metadata read/modify/write, and unsupported queued or clarification admission stays context-only even when a direct-send draft races into a queue.

Queue editing and plugin registration remain in Tasks 03 and 04.
