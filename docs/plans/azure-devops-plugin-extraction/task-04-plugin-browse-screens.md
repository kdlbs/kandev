---
id: "04-plugin-browse-screens"
title: "Browse screens"
status: in_progress
wave: 3
depends_on: 
  - "02-plugin-connection"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.8
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.9
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.10
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.11
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.12
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.13
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 04: Browse screens

## Summary

Port the Azure DevOps page: work-item search, Boards, pull requests, scope selectors and filters, work-item detail with comments and state and assignee changes, saved views, presets, quick actions, and per-user browse preferences, on shared host integration components.

## In scope

- Browse actions per the design table with cursor pagination.
- `registerRoute("/azure-devops")`, `registerNavItem`.
- Settings sections for default queries and quick actions.
- Phone composition from the built-in mobile screens.

## Out of scope

- Task launch and links (task 05).

## Acceptance

- Every flow in the built-in `azure-devops.spec.ts`, `mobile-azure-devops.spec.ts`, and `azure-saved-views.spec.ts` has a passing plugin E2E equivalent.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && go test -race ./... -count=1)
(cd ../JnManso-kandev-plugin-azure-devops-main && npm test && npm run e2e)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/ui/src/browse/`
- `../JnManso-kandev-plugin-azure-devops-main/internal/plugin/actions_browse.go`

## Dependencies

Task 02-plugin-connection

## Risks

- Action payloads are capped at 1 MiB; page boards and results.

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-05: implemented on plugin branch `feat/bootstrap` in commits `fa81fd4` (actions), `1be2eeb` (translations), `cc7daf8` (page), and `678a66f` (settings sections).
  - Backend: browse actions ported from the built-in services.
    - Action keys use underscores, as Kandev requires lowercase keys: `projects.repositories`, `teams.list`, `boards.list`, `boards.get`, `boards.update_item`, `work_items.search`, `work_items.get`, `work_items.comments`, `work_items.update`, `pull_requests.list` (opaque cursor, `@me` filters), `pull_requests.get`, `pull_requests.feedback`, `identity.get`, `views.get`, `views.save`, `workspace_settings.get`, and `workspace_settings.patch`.
    - List payloads drop the provider field map and descriptions to stay below 1 MiB.
    - Saved views and preset overrides live in workspace state, written under the workspace lock.
  - UI: the built-in page and components ported almost verbatim.
    - A host-backed automatic JSX runtime (`plugin-jsx`) renders through `host.jsx`.
    - `host-ui.ts` proxies every host UI component.
    - Tabler icons are generated as SVG components.
    - The built-in `azuredevops` strings in all locales are reused as `ui/src/locales/*.json`, with `verbatim.json` and a test that every literal `t()` key exists.
    - Per-user browse preferences use `host.storage` (`user_state` capability).
    - `ui/plugin.css` (`ui.styles`) adds the utilities missing from the host stylesheet, found with `scripts/check-host-css.py`.
    - The route `/azure-devops`, a nav item in the integrations section, and the default queries and quick actions sections in settings.
  - Mock mode: `KANDEV_AZURE_DEVOPS_PLUGIN_MOCK=true` serves the built-in mock client seeded with the built-in E2E organization, plus `auth.MockEntra`. It is used for the smoke test below and task 07.
  - Deviations:
    - Work item descriptions and comments render as text: the server strips description HTML, and comments are converted with `DOMParser`. Markdown rendering would need a React library inside the bundle.
    - The nav item is always registered, because `registerNavItem` has no visibility predicate; the page shows a not-connected notice.
    - Task launch and linked tasks are task 05; the quick-action buttons stay hidden until then.
- Verification:
  - `go test -race ./...` passes.
  - Vitest 66 passes, including the ported board-view and presets tests.
  - `verify-package` and `npm audit` pass.
  - Smoke in mock mode on a disposable Kandev from the removal branch:
    - The board renders with columns. A card opens its detail (description without the script, effort field, discussion). Assign to me works. Drag to Active works.
    - Work-item search and pull-request search work, and feedback shows reviewers and policies.
    - The default queries edit, save, and appear in the scope bar.
    - A saved view saves, and its delete confirmation and delete work.
    - The board choice persists across a reload. The phone board uses the column navigator.
- Open: E2E equivalents of the built-in browse and saved-view specs, run as packaged-host Playwright specs with mock mode (task 07). Owner validation on `contoso`.
- 2026-11-10: manual testing found the quick actions section stuck disabled while the browser requested the workspace settings about 115 times a second.
  - Cause: the plugin's `useT()` returned a new function on every render, and the section's load effect depends on it, so every render cancelled the request in flight.
  - Fix: `useT()` and `usePluginTranslation()` are memoized on the host translation function. Unit tests replay renders with React's identity rules. A packaged E2E case failed on the old code (section disabled) and passes now, with at most four settings loads.
- 2026-11-10: owner testing on `contoso` found that teams and projects stopped at Azure DevOps' 100-record page (Contoso has more teams, so the Offers board was missing), the scope pickers had no search, work-item search was hidden in the WIQL field, **All** pull-request status returned only active pull requests, a project change kept the previous project's repository, and real pull requests had an empty web address. Fixed in plugin commits `79b7026`, `3e7a5b4`, and `a42dcca`: `$top/$skip` paging sorted by name, Kandev's searchable `Combobox` with a contains filter, a search box that builds WIQL for a number or title words, `status=all` sent as is, `repositoryForProject`, and web addresses built from organization, project, and repository names. Tests: `rest_client_paging_test.go`, `rest_client_pr_web_url_test.go`, `mock_client_status_test.go`, and `search-query.test.ts` failed before the changes and pass after them; the mock now follows Azure DevOps for status and the two searches, and three new packaged-host E2E tests cover search, the team picker, and **All**. `make test` (110 Vitest tests, Go tests) and `make e2e` (17 passed) pass.
- 2026-11-10 (second owner test round on `contoso`): picking the selected project again cleared it, because the host combobox reports `""` for a re-picked option; the Board then showed no project selector and searches failed with "a project is required". `SearchablePicker` now keeps the value (`pickedValue`), and the empty Board still renders the project selector. A title search with several words required them as one phrase; it now ANDs one `CONTAINS` per word, and the mock matches every `CONTAINS`. The search box's Enter key ran a search the disabled Search button would not; both now use `canSearch`, and a failed search clears the previous results and count. Unit tests (`search-query.test.ts`, `TestMockClientWIQLSearchesByIDAndTitle`) failed before the changes and pass after them. Packaged E2E adds "keeps the board project when it is picked again" and "opens the board on the first project when the connection has no default project", and the title-word test searches words out of order. `make check-format vet test` (116 Vitest tests) and `make e2e` (19/19) pass.
- 2026-11-11 (third owner test round on `contoso`): the Board showed empty Team and Board pickers with no loading indicator for about 3.3 s, then needed another 2.4 s. It waited for the project's hundreds of teams (ten 100-record pages read one after another, and requested twice) before loading the remembered board.
  - Fix: the Board starts on the team and board remembered for the project (the preference now records the project and their names) and loads it while the team list loads, keeping them only if the list still has them (`board-discovery.ts`). One team listing is shared per workspace and project. A loading indicator shows from the start. The server reads the first listing page alone and later pages four at a time.
  - Tests: `board-discovery.test.ts` (9 cases); `TestRESTClientListsTeamPagesConcurrently` failed before the change (one request at a time) and passes now, with `TestRESTClientListsAnExactMultipleOfThePageSize`. The packaged E2E cases "opens the remembered team and board before the team list arrives" (team list delayed 4 s) and "shows the board loading state from the start" failed on the old code and pass now.
  - Verification: `make check-format vet test` passes (Go with `-race` for `internal/...`, 125 Vitest tests, typecheck); `make e2e` passes 21/21.
- 2026-11-11: owner review: the in-app Azure DevOps icon was drawn in brand blue while Kandev's navigation icons are monochrome. The UI icon now fills with `currentColor`; the marketplace `assets/icon.svg` is unchanged and `assets/NOTICE.md` says so. `azure-icon.test.ts` failed before the change and passes after it.
- 2026-11-11: owner testing found that a Board card had no direct task action; the quick actions were only in the card's detail. Board cards now carry the same task actions menu as work-item results, shared with sprint cards (`task-actions.tsx`); its events stay inside it, so it never opens or drags the card. The packaged E2E case "offers task actions on a board card without opening it" covers it; implemented with task 10.
- 2026-11-11: owner testing on `contoso` found that the Offers **Tasks** board failed with "the Azure DevOps item was not found or is not accessible" while still showing the previous board's Feature cards.
  - Cause: `boards.list` offered every visible backlog level, including the task level, which has no Kanban board, so `work/boards/Microsoft.TaskCategory` answered 404. The Board kept the last snapshot when a read failed.
  - Fix: `ListBoards` skips levels whose `type` is `task`; the Board clears the snapshot when the board changes (AC 004.11).
  - Tests: `TestRESTClientBoardReads` now serves a visible task level and failed before the change; the packaged E2E case "drops the previous board's cards when a board fails to load" failed on the old code (card still visible) and passes now.
  - Verification: `make check-format vet test` passes; `make e2e` passes 29/29.
- 2026-11-11: owner testing on `contoso` found **Pull requests** empty for Contoso / Contoso.Offers / Active while Azure DevOps listed three active pull requests by the owner.
  - Cause: the search sent `reviewer: "@me"`, kept from the **Review requested** shortcut. No shortcut looked selected (the selection had become custom), and the reviewer field sat closed under **Advanced**. Switching modes with the mode tabs also left the scope bar on the other kind's shortcuts.
  - Fix (plugin commit `fa72255`): the scope bar follows the browse mode and marks the shortcut matching the current filters (`matchingPresetId`); **Advanced** shows a count of its set pull-request filters (`advancedFilterCount`) (AC 004.12).
  - Tests: `presets.test.ts` adds 3 cases (failed before: missing helpers). The packaged E2E case "shows a pull-request filter hidden under Advanced" failed on the old code (no shortcut marked) and passes now. `make check-format vet test` and `make e2e` (37/37) pass. The `pull_requests.list` action on `contoso` returns the three pull requests (#601, #602, #603) without the reviewer filter.
- 2026-11-11: owner review: the pull-request **Creator ID** and **Reviewer ID** fields asked for identity IDs (as the built-in integration did), which users do not know.
  - Change (plugin commit `275bcec`, AC 004.13): the action `pull_requests.people` lists the creators and reviewers of the repository's latest 300 pull requests (`DistinctPeople`, sorted by name). **Created by** and **Reviewer** are name pickers with **Anyone**, **Me** (`@me`), and those people (`personPickerOptions`); a saved value the list does not name stays selectable. The mock now filters pull requests by creator and reviewer like Azure DevOps.
  - Tests: `pull_request_people_test.go`, `mock_client_people_test.go`, `browse_people_test.go`, and `pull-request-people.test.ts` failed before the change (missing action and helpers). Packaged E2E "filters pull requests by people picked by name" and the updated "shows a pull-request filter hidden under Advanced". `make check-format vet test`, `go test -race ./internal/...`, and `make e2e` (38/38) pass.
  - Follow-up (plugin commit after `275bcec`, "pick pull-request watch people by name and name Me"): the pull-request watch editor uses the same pickers (people of the watch's repository, or of the project without one); **Me** shows the signed-in identity's name (`identity.get`) and the user's own entry is not listed again. `pull_requests.people` accepts a project without a repository. Tests: `pull-request-people.test.ts` (4 cases), `TestRESTClientListsThePeopleOfAProjectsPullRequests`, the project-wide case in `browse_people_test.go`, and the packaged E2E cases "names the signed-in user under Me instead of listing them twice" and "picks pull-request watch people by name". `make e2e` passes 40/40.
  - Owner review on `contoso`: **Me** showed the identity's provider display name ("ADA PROVIDER") without an email. Plugin commit `d001b12` labels it "Me - <name>" with the email underneath, using the user's own pull-request entry (here "ADA LOVELACE") when the pull requests name them, else the identity. `pull-request-people.test.ts` (5 cases) failed before the change; `make e2e` passes 40/40.
  - Test-phase check on `contoso` (headless, owner session, 2026-11-11): board selector without Tasks; Epics #2001 `Feature: 1/4`, feature #2003 `User Story: 17/27` with 27 stories, story #2004 `Task: 3/3` expanding to its tasks; task line actions and detail; expand all survives a reload; Sprints Sprint 23 with New, Active, Closed and 7 cards; the scope bar marks **Created by me** for that shortcut's filters; **Advanced** count; `Me - ADA LOVELACE` with email; reviewers by name; **Created by me** with **All** lists 25 pull requests. The watch editor without a repository showed "Me - ADA PROVIDER": the identity used `providerDisplayName`. Plugin commit `b49b85f` prefers `customDisplayName` (`TestRESTClientGetCurrentIdentityPrefersTheCustomDisplayName` failed before).
  - Open: the workspace default queries editor in settings still takes identity IDs for its pull-request creator and reviewer.

