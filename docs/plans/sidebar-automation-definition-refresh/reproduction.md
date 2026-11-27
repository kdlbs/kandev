# Sidebar automation definition refresh: reproduction

Date: 2026-10-23. Classification: UI data freshness.
Related context: https://k.soitgoes.cc/t/0e1c6a70-22c2-4b5c-8ca2-404e3688efbf

## Source baseline

Local HEAD: `595df36ed5c1cf02fd90cb972b623ad6edc53df8`.
Authoritative main from `git ls-remote origin refs/heads/main`:
`c15867d270e3cddcae5334262ba5b31a6ef9ecce`.
The object was available locally. Diffing the automation runs directory,
builtin section, catalog hook, mobile section, layout hook, and owning specs
against that commit showed no differences.

`git fetch origin main` failed because an unrelated local branch reference was
broken: `refs/heads/feature/lead-pr-4371-to-merg-eeb`. No reference was repaired.
The relevant current-main comparison used the authoritative SHA above.

No prior task plan, dirty files, or previous-session messages existed at entry.
The older deployed commit in the request was not used as the reproduction base.

## Temporary component proof

Copy `automations-section.test.tsx` beside itself as a temporary test file.
Use its existing transport mocks and real `AutomationsSection`, definition hook,
summary hook, and interval hook. Parameterize three cases: create, rename, delete.

1. Enable advancing fake timers. Mount the expanded section with one automation.
2. Await the initial row and retain the section DOM element.
3. Change only the mocked API result: add a second automation, rename the first,
   or return an empty array.
4. Advance `LIVE_REFRESH_INTERVAL_MS * 2` inside `act`.
5. Assert the same section element and at least three summary requests.
6. Assert the added row, renamed text, or removed row, respectively.

Executed from the repository root:

```bash
pnpm --dir apps/web exec vitest run components/app-sidebar/sections/automations-refresh-repro.test.tsx -t 'temporary external definition reproduction'
```

Result: exit 1, three expected assertion failures, eighteen unrelated tests
skipped. Creation remained absent. Rename still displayed `Nightly drift`.
Deletion left the original row present. Initial-row, mounted-element, and summary
request assertions passed before each failure. This was a logic failure, not a
setup or test-discovery failure. The temporary file is removed after diagnosis.

## Isolated browser proof

Launch `scripts/dev-isolated --web` using a new empty database and mock providers.
The owned backend used port 48429, web 47429, and agentctl 49429.
Database: `/tmp/kandev-iso-48429-S7AEUn/home/.kandev/data/kandev.db`.
Backend log: `/tmp/kandev-iso-48429-S7AEUn/backend.log`.

The initial diagnostic bundle reported `partial`: backend included, no frontend
browser responded. It preceded successful browser startup and is not proof of
frontend health. The successful browser reported zero console errors.

The workspace-pinned `playwright-cli` used installed Chromium 1228, headless.
Default Chrome was absent. Chromium required the host Nix library paths in its
process environment. These were temporary launch settings, not repository edits.

1. Open `http://localhost:47429/` at 1440 by 1000 pixels. Skip local onboarding.
2. Confirm the saved builtin Automations node has `visible: true`. Expand it.
3. Capture `automation.list` and `automation.summaries` received WebSocket frames.
   Retain the section DOM element and page URL before external creation.
4. From a separate Node WebSocket client, send a normal `automation.create`
   request to `ws://localhost:48429/ws`. Use the isolated workspace, a unique
   name, `enabled: false`, and `repository_mode: none`. Do not configure triggers.
5. Observe subsequent definition and summary responses while keeping the page
   and section mounted. Assert the created row should appear.

The external request returned automation ID
`d24cbcc6-714c-4c03-9e7f-d40917cb59c2`, name `External sidebar proof`.
At 42,671 ms after the section marker:

- The original section element was still mounted and the URL was unchanged.
- The section had zero automation rows and showed `Set up an automation`.
- A received `automation.list` response contained the created ID.
- Nine `automation.summaries` frames had arrived.

The expected row was absent despite successful transport. Reloading afterwards
made `External sidebar proof` appear. The reload was a positive control after
the failure, not part of the failing reproduction.

This confirms the source-observed defect independently of the earlier user's
hidden section. It does not claim the earlier missing section had this cause.

## Mobile control

Resize the same isolated browser to 393 by 851 pixels. Open the navigation drawer
and expand Automations. Create `Mobile external proof` through the separate
WebSocket client, again disabled with no triggers. Keep the drawer mounted.

The created row appeared. At the recorded observation, 24,857 ms after the drawer
marker, the original section remained mounted and document overflow was false.
That observation proves eventual refresh on the existing phone path. It is not
a measurement of the exact first-appearance time or a ten-second SLA test.

Source explains the difference: phone saved-layout rows consume the catalog.
Desktop builtin rows keep their own hook instance. `useHasSavedSidebarLayout`
returns true for any selected workspace, so the no-children mobile fallback is
not the active workspace route. Its similar source pattern is not a separately
confirmed UI defect and is excluded from this patch.

## Root cause and limits

`useWorkspaceAutomations` stores results per hook instance. The catalog's live
and foreground refreshes only update its instance. `AutomationsSection` passes
only its summaries refresh to `useLiveRefresh`, leaving its names list unchanged.

Component proof covers create, rename, and delete. Browser proof covers external
creation. Deferred workspace/order cases, hidden-section regression, and the
permanent RED/GREEN suite remain work-order requirements, not completed evidence.

## Cleanup

The owned browser and isolated instance were stopped after recording evidence.
The temporary test, client/config, screenshots, logs, database, and diagnostic
bundle were removed.
The implementation package retains these procedures and results. No user's
instance, automation, Telegram topic, or homelab service was modified.
