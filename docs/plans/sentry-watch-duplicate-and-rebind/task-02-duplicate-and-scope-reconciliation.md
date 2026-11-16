---
id: "02-duplicate-and-scope-reconciliation"
title: "Duplicate action and instance-scoped field reconciliation"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.1
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.2
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.3
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.4
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001.5
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.1
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.2
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.3
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.4
  - AC-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002.5
system_design:
  - ../../specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md
---

# Task 02: Duplicate Action and Instance-Scoped Field Reconciliation

## Summary

Add a Duplicate row action that opens a prefilled create dialog. When the
instance changes in the dialog, keep the org and projects the new instance
offers and every other field. This delivers the region-copy use case on the
existing create API.

## ASCII UI preview

Full previews: [plan](plan.md#ascii-ui-preview), views UI-01, UI-02, UI-03.

```text
desktop: | US | org:acme project:web | ... | [Pause] [Check] [Reset] [Dup] [Del] |
phone:   |     [Pause] [Check] [Reset] [Dup] [Del] |   (44 px targets)

Duplicate Sentry watcher
Sentry instance [ EU v ]
Organization    [ acme v ]   kept (offered by EU)
Projects        [ web x ]    api dropped (not offered by EU)
Environment, levels, status, lookback, query, workflow ... Enabled: kept
                                   [ Cancel ] [ Create ]  (disabled while verifying)
```

## In scope

- `WatchActions`: `onDuplicate` prop and an `IconCopy` icon button between
  Reset and `WatcherDeleteAction`, `data-testid="sentry-watch-duplicate-<id>"`,
  localized `aria-label` and tooltip, propagation stopped. Wire through
  `DesktopSentryIssueWatchTable`, `MobileWatchCard`, and the table props.
  Use at least 44 px targets on phone or coarse pointers and 28 px on
  fine-pointer desktop; do not key touch sizing solely on pointer precision.
- `SentryIssueWatchersSection`: `duplicating` snapshot and `openDuplicate`;
  snapshot the displayed `enabledDrafts.items` row, including any unsaved
  Enabled toggle, without committing/discarding the source draft.
  `openCreate` and `openEdit` clear it.
- `SentryIssueWatchDialog`: `duplicateOf` prop. The opening key includes
  the mode and source ID; use a lossless duplicate initializer (including the
  exact stored prompt, even empty, and `enabled`). Leave the existing edit
  initializer's prompt fallback unchanged; defaults apply only to fresh create.
  Test empty-prompt draft prefill, readiness blocking until user correction,
  and a later create without writing the source. Title
  `sentry:duplicateSentryWatcher`; existing create path.
- Lift project loading from `FilterFields` into the dialog. `useWatchOrgs`
  and the project hook expose `{ items, status }`, keyed by workspace and
  instance only, and discard superseded responses. Org narrowing stays
  client-side; changing org must not refetch projects.
- `reconcileInstanceScope` and `scopePending` in `sentry-issue-watch-form.ts`;
  instance changes keep org/projects pending verification; `isWatchFormReady`
  blocks Save while pending. When reconciliation clears an invalid org in
  create/duplicate mode, apply the sole-org default in that same transition
  and drop all carried projects. Do not depend on `useWatchOrgs` rerunning.
- Sole-instance auto-selection uses that same transition, not a direct
  instance-only `setForm`. Test legacy duplicate empty->sole-instance with
  deferred and failed browse requests: Save and selectors stay disabled,
  invalid carried org/projects clear with the sole-org rule, valid selections
  survive, and source plus unrelated fields remain unchanged.
- Disable org and project selectors throughout pending scope verification,
  including keyboard interaction. Preserve carried values as read-only
  display; do not let injected synthetic options resolve pending. Once lists
  settle, reconcile/clear, then enable verified choices. Test attempts to toggle
  carried projects or org while new lookups are deferred, and failure of one
  lookup while the other is still pending; Save must remain blocked.
- Delete the old eager-clearing test ("removes the previous instance's org
  and project choices before the new lookup resolves") in
  `sentry-issue-watch-dialog.test.tsx`; replace with behavioral coverage of
  kept valid selections, removed invalid ones, pending Save, and late responses.
- Tests for a copied unsaved Enabled toggle and legacy invalid filters
  (multi-status and unsupported lookback): failed Create toasts and preserves
  the draft and source, rather than silently converting the filter.
- New `sentry` catalog keys (`duplicate`, `duplicateThisSentryWatcher`,
  `duplicateSentryWatcher`) in all seven locales (`pnpm run i18n:zh-hant` for
  zh-hk/zh-tw); append touched files to `i18nGuardFiles` if not present.
- Public docs: one sentence on Duplicate in the Sentry watch section of
  `docs/public/integrations.md`.
- Unit tests and the two E2E scenarios below.

## Out of scope

- Enabling the instance picker in edit mode, the warning, and update payload
  changes (Task 03).
- Backend changes.

## Acceptance

- Duplicate sits between Reset and Delete in desktop and phone rows; it opens
  the duplicate dialog with every source field (including instance and
  Enabled as displayed, including unsaved toggles) and never opens edit. Valid
  Create posts a new watcher without saving the source draft; Cancel writes
  nothing; an unbound source requires an instance; rejected legacy filters
  leave a correctable draft open. Empty stored prompts stay empty until edited.
- Switching instances keeps valid org/projects, clears invalid or
  unverifiable ones, applies the create/duplicate sole-org default, keeps all
  other fields, and disables Create and scoped selectors until verification
  completes. Toggling synthetic carried values cannot bypass verification.
- Desktop and phone E2E complete Create after switching instances, verify the
  new watcher and unchanged source, and check the phone Duplicate hit target
  and viewport containment.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/sentry/sentry-issue-watch-table.test.tsx components/sentry/sentry-issue-watch-dialog.test.tsx components/sentry/sentry-issue-watch-form.test.ts components/sentry/sentry-issue-watchers-section.test.tsx components/sentry/sentry-watch-dialog-lifetime.test.tsx)
(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts -- --grep "duplicates a watcher onto another instance")
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts --grep "duplicates a watcher from the phone list")
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/sentry/sentry-issue-watch-table.tsx`
- `apps/web/components/sentry/sentry-issue-watchers-section.tsx`
- `apps/web/components/sentry/sentry-issue-watch-dialog.tsx`
- `apps/web/components/sentry/sentry-issue-watch-filter-fields.tsx`
- `apps/web/components/sentry/sentry-issue-watch-form.ts`
- `apps/web/components/sentry/*.test.ts(x)` listed above
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/sentry.json`
- `apps/web/e2e/pages/sentry-settings-page.ts`
- `apps/web/e2e/tests/integrations/sentry-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts`
- `docs/public/integrations.md`

## Dependencies

None. Shares `sentry-issue-watch-dialog.tsx` with Task 03, which runs after.

## Risks

- The opening key must distinguish duplicate from edit of the same watch, or
  a duplicate could reuse an edit owner and save via PATCH.
- Lifting project loading must keep the existing lifetime tests
  (`REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001`) green.
- `useSentryProjects` is also the project source for `FilterFields`; the
  lifted hook must keep org narrowing.
- Dialog and filter-field files are near TS lint line limits; extract hooks
  into a sibling module if needed.

## Inputs

- `docs/specs/integrations/requirements/sentry-watch-duplicate-and-rebind.md`
- `docs/specs/integrations/system-design/sentry-watch-duplicate-and-rebind.md`
- `docs/specs/integrations/system-design/sentry-watch-dialog-lifetime.md`
- `apps/web/AGENTS.md`, `docs/i18n.md`

## Results

Pending.
