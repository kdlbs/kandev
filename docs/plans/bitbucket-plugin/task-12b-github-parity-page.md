---
id: "12b-github-parity-page"
title: "Rebuild Bitbucket page with GitHub list parity"
status: completed
wave: 3b
depends_on: ["12-plugin-ui-native-registrations"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 12b: Rebuild Bitbucket page with GitHub list parity

## Intent

Replace the live-evaluation desktop split workbench with the first-party `/github`
page's list-first hierarchy while retaining plugin-owned review depth, watches, Cloud
and Data Center capability handling, and native mobile parity.

## Confirmed cause

`ui/src/bundle.ts` renders `.bb-desktop-panes`, permanently allocating a narrow queue
and an empty review pane. `apps/web/app/github/github-page-client.tsx` instead composes
`PresetsScopeBar`, `ListToolbar`, one padded full-width `PRList`, and an optional
pagination footer. Source, installed, and served plugin asset hashes matched during
reproduction, so stale files did not cause the reported layout.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree:
  - `ui/src/bundle.ts`
  - `ui/src/view-models.ts`
  - `ui/plugin.css`
  - `ui/bundle.js`
  - `ui/test/registrations.test.ts`
  - `ui/test/view-models.test.ts`
  - `ui/e2e/desktop-packaged-plugin.spec.ts`
  - `ui/e2e/mobile-packaged-plugin.spec.ts`

## Implementation

1. Add tested review-query URL helpers so `/bitbucket?review=<canonical-key>` is a
   reloadable focused-detail state and Back returns to `/bitbucket`.
2. Replace `.bb-desktop-panes` with a full-height list layout matching GitHub's order:
   desktop scope pills, compact title/count + repository/query + refresh/watch toolbar,
   then one padded, bordered, divided result list.
3. Match GitHub row density and anatomy: provider-colored PR state icon, linked title,
   repository/number and readable author metadata, restrained status, compact Task and
   Review affordances. Hide provider-opaque account identifiers from display.
4. Reuse `ReviewDetailPanel` as focused page content and native task ReviewPanel; do
   not duplicate review action logic.
5. Keep phone composition one-dimensional: compact list toolbar, filter Drawer,
   direct focused detail, 44px controls, safe-area padding, and one scroll owner.

## Mobile design contract

- Desktop outcome: scan full-width results and launch a task, open Bitbucket, or open
  focused review without a permanent detail pane.
- Mobile entry: `/bitbucket`; closest exemplar is `/github` list/filter hierarchy plus
  `apps/web/components/kanban-with-preview.tsx` direct detail navigation.
- Hierarchy: topbar → compact toolbar → PR list → focused review. Primary row action is
  Review; filters/actions are temporary Drawers.
- Scroll: list or focused review owns vertical scroll, never both; viewport-bound
  content uses `100dvh`, overscroll containment, and bottom safe-area clearance.
- Shared query/filter/review state and view-model helpers serve both compositions.

## RED tests

- `ui/test/view-models.test.ts`: review URL encoding/decoding and opaque-author display.
- `ui/test/registrations.test.ts`: GitHub-parity markers exist and permanent split-pane
  markers do not.
- Desktop Playwright: full-width list exists, no review pane exists initially, Review
  changes URL and focuses detail, Back restores list.
- Mobile Playwright: same URL/back outcome, filter/action Drawers remain contained,
  touch targets remain at least 44px, and document has no horizontal overflow.

## Verification

```sh
npm test
npm run typecheck
npm run lint
npm run e2e
test -z "$(gofmt -l .)"
git diff --check
```

Completed verification: 27 Vitest checks plus typecheck/build passed; seven packaged
desktop/mobile Playwright scenarios passed against the live Cloud-connected instance.
Fresh desktop and mobile documents rendered without browser console errors.

Then copy only `ui/bundle.js` and `ui/plugin.css` into the running disposable plugin
installation, hard-reload a fresh document, and capture `/github` and `/bitbucket` at
identical desktop/mobile viewport sizes. Do not replace the live plugin server binary.

## Risks

- Bitbucket queue actions currently expose a bounded result set without total/cursor;
  match GitHub's list hierarchy but do not fabricate pagination totals.
- Review query keys must remain encoded and must not become a host-specific route.
- Cloud can return canonical opaque account IDs; presentation must omit those rather
  than exposing unreadable identifiers or changing authorization identity semantics.
