---
id: "03-edit-instance-change-ui"
title: "Edit-mode instance change UI"
status: pending
wave: 2
depends_on:
  - "01-backend-instance-rebinding"
  - "02-duplicate-and-scope-reconciliation"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.1
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.2
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.3
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.4
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.6
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.7
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.8
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.9
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.10
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.11
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.12
system_design:
  - ../../specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md
---

# Task 03: Edit-Mode Instance Change UI

## Summary

Let the edit dialog change a watcher's instance. Show the history-clearing
warning before Update, and send `sentryInstanceId` only when it changed.

## ASCII UI preview

Full preview: [plan](plan.md#ascii-ui-preview), view UI-04.

```text
Edit Sentry watcher
Sentry instance [ US v ]   (was EU)
  Which Sentry instance this watcher polls.
  +------------------------------------------------------------+
  | (!) Changing the instance clears this watcher's import      |
  |     history. Existing tasks are kept; matching issues on    |
  |     the new instance are imported on the next poll.         |
  +------------------------------------------------------------+
...                                     [ Cancel ] [ Update ]
```

UI-05 excerpt (same composition on desktop and phone):

```text
Reset Sentry watcher
Only tasks in the current import history are deleted, including archived
tasks. Tasks from before an instance change stay.
                                             [ Cancel ] [ Reset ]
```

This scope note stays visible on preview success, zero, loading, and failure
(`AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003.7`).

## In scope

- Remove `disabled={!!watch}` from `InstancePicker`; reconciliation from
  Task 02 applies in edit mode.
- Warning `Alert` (`@kandev/ui/alert`) under the picker while `watch` is set
  and the form instance differs from `watch.sentryInstanceId`;
  `data-testid="sentry-watch-instance-change-warning"`. A legacy unbound edit
  shows the same warning after explicit instance selection; every such bind
  clears history.
- `UpdateSentryIssueWatchRequest.sentryInstanceId?: string`; the dialog adds
  it to the update payload only when non-empty and different from the saved
  instance, captured with the rest of the payload before awaiting.
- Add read-only `SentryIssueWatch.scopeRevision` and guard publication in
  `useSentryIssueWatches`: older-scope PATCH/list responses cannot replace a
  newer-scope row; missing revision evidence requires authoritative refresh.
  Preserve workspace and dialog-opening ownership. Add a real hook/section
  regression: hold an accepted A PATCH response, publish accepted B rebind,
  then deliver A. Reopen Edit and Duplicate; both must retain B's binding and
  filter baseline. Cover a delayed list refresh and Reset revision refresh too.
- Add `UpdateSentryIssueWatchRequest.expectedScopeRevision?: number`. Capture
  the saved `scopeRevision` once at opening and send it on every edit update,
  including unchanged-instance/legacy edits and revision 0. Missing evidence
  blocks submit pending authoritative data, not an invented default. Do not
  refresh the opening precondition from passive hydration or after a conflict;
  it is not editable form state. Create/Duplicate send no update precondition.
- Regression through the real hook/section/dialog: accept A->B rebind but hold
  its response; Cancel and reopen the still-visible A row, edit prompt; deliver
  B, then Save the old opening. Verify its stale precondition causes 409,
  preserves its draft, and leaves B's filter/history untouched. Also cover ABA
  and Reset before the new request's server pre-read at the service/API layer.
  Keep same-scope passive hydration behavior and truthful completion toasts.
  Update affected lifetime fixture/request contracts without retaining tests
  that merely pin incidental payload forwarding.
- Copy: new `sentry:instanceChangeWarning`; `sentry:sentryInstanceHelp`
  becomes "Which Sentry instance this watcher polls." in all seven locales.
  Update the immutability comments in `lib/types/sentry.ts`
  (`SentryIssueWatch`, create request) and `FormState`, and the
  `buildWatchPayload` comment.
- Sentry-specific Reset copy via optional `ResetWatchDialog.scopeDescription`
  and `previewFailureDescription` props. Add `sentry:resetWatchScope` (only
  current-history tasks, including archived ones, are deleted; pre-rebind
  tasks remain even when nested below a current-history task) and
  `sentry:resetWatchPreviewFailed` (current-history scope, never "every task").
  Render the scope note on success, zero-count, loading, and error states.
  Surviving direct children are detached, not cascade-deleted; document that
  hierarchy outcome. Other integrations retain their existing defaults.
- Delete the legacy edit test that asserts the picker is disabled ("permits
  mutable updates to a legacy unbound watch while its instance remains
  immutable"); replace with explicit bind warning/payload and unchanged legacy
  edit coverage. Add Reset preview success/failure and 409 draft-retention
  tests in the owning section/dialog suites.
- Public docs: replace both obsolete statements in
  `docs/public/integrations.md`: "the selected instance is immutable after
  creation" and "Because the instance binding is immutable, delete those
  watches first and recreate them against another instance". Document rebind,
  always-clear legacy binding, tasks kept, non-cascading Reset scope and child
  detachment, and conflicts/retry. Document `scopeRevision` response evidence
  and optional PATCH `expectedScopeRevision`: updated edit clients always
  send their opening value; older API callers without it have server-pre-read
  protection only. Agent settings rebinding remains unsupported.
- Desktop and phone edit/rebind E2E scenarios, both extended through Reset:
  delete the EU task but keep the earlier US task, consistent with confirmation.
  New phone test `moves a watcher from the phone list` in
  `mobile-sentry-watch-dialog-ownership.spec.ts` uses native taps, changes the
  picker, reaches/reads the warning and footer within the viewport, verifies
  PATCH carries the new instance and the row updates, then completes Reset
  and verifies task retention/deletion. Do not reuse the unchanged-edit helper
  that asserts absence of `sentryInstanceId`. Check containment, internal
  scroll ownership, and at least 44 px primary actions.

## Out of scope

- Backend (Task 01), Duplicate and reconciliation (Task 02).
- A confirmation modal; the inline warning is the pre-save notice.

## Acceptance

- The edit dialog's instance picker is enabled. The warning shows only
  while the selection differs from the saved instance and disappears when
  it is restored; explicit legacy binding also shows it. Reset describes only
  current history, including when its preview fails.
- Changed-instance updates send `sentryInstanceId`; unchanged-instance updates
  omit it. All edit updates send the immutable opening's
  `expectedScopeRevision`. A stale opening conflicts even before the server's
  pre-read, keeps the draft open, and cannot retarget its filter to the new
  binding. Same-scope lifetime behavior remains intact.
- Desktop and phone E2E: after moving a watcher from US to EU, triggering
  imports EU's `WEB-1` despite US's recorded collision; the row shows EU.
  Reset deletes EU's recorded task while retaining US's task; warning, picker,
  and footer remain reachable and contained on phone.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/sentry/sentry-issue-watch-dialog.test.tsx components/sentry/sentry-issue-watch-form.test.ts components/sentry/sentry-watch-dialog-lifetime.test.tsx components/sentry/sentry-issue-watchers-section.test.tsx components/watches/reset-watch-dialog.test.tsx hooks/domains/sentry/use-sentry-issue-watches.test.tsx)
(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts -- --grep "moves a watcher to another instance")
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts --grep "moves a watcher from the phone list")
(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
make fmt
make typecheck test lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/components/sentry/sentry-issue-watch-dialog.tsx`
- `apps/web/components/sentry/sentry-issue-watch-form.ts`
- `apps/web/components/sentry/sentry-issue-watch-dialog.test.tsx`
- `apps/web/components/sentry/sentry-issue-watchers-section.tsx`
- `apps/web/components/sentry/sentry-issue-watchers-section.test.tsx`
- `apps/web/components/watches/reset-watch-dialog.tsx`
- `apps/web/components/watches/reset-watch-dialog.test.tsx`
- `apps/web/lib/types/sentry.ts`
- `apps/web/hooks/domains/sentry/use-sentry-issue-watches.ts`
- `apps/web/hooks/domains/sentry/use-sentry-issue-watches.test.tsx` (new response-order suite)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/sentry.json`
- `apps/web/e2e/helpers/api-client.ts` (seed issues / list tasks if missing)
- `apps/web/e2e/tests/integrations/sentry-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts`
- `docs/public/integrations.md`
- Package docs at completion: requirement to `active`, design to `current`,
  plan to `implemented`, work-order statuses.

## Dependencies

- Task 01 (PATCH rebinding, history guards, conflict errors, and Reset scope).
- Task 02 (reconciliation and lifted loading in the dialog).

## Risks

- Sending `sentryInstanceId` on unchanged updates would break the lifetime
  tests that assert its absence and could rebind legacy watches unexpectedly.
- The warning must compare against the opening's saved instance, not a
  hydrated list snapshot, to honor same-identity hydration rules.
- Translations for the warning must avoid the em dash
  (`check-no-em-dash-ui`).

## Inputs

- `docs/specs/integrations/requirements/sentry-watch-duplicate-and-rebind.md`
- `docs/specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md`
- `docs/decisions/2026-10-24-sentry-watch-instance-rebinding.md`
- `apps/web/AGENTS.md`, `docs/i18n.md`

## Results

Pending.
