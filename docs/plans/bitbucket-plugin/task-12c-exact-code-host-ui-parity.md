---
id: "12c-exact-code-host-ui-parity"
title: "Use exact shared GitHub/GitLab dashboard and task flow"
status: completed
wave: 3c
depends_on: ["12b-github-parity-page"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md"
---

# Task 12c: Use exact shared GitHub/GitLab dashboard and task flow

## Intent

Replace visual approximation and plugin-invented actions with the actual host-rendered
GitHub/GitLab dashboard contract. Preserve Bitbucket API/domain ownership and native
task review-provider depth.

## Confirmed mismatch

GitHub `PRList` and GitLab `MRList` render inert rows whose title opens the provider
and whose sole action is a compact **Task** preset dropdown. Preset selection opens
`TaskCreateDialog` directly. Bitbucket rendered equal **Task** and **Review** buttons,
then added an intermediate preset modal before server-side task creation. Served and
source hashes matched, so stale assets did not cause the mismatch.

Manual follow-up found two remaining parity regressions: Bitbucket scope pills changed
an out-of-band state parameter without committing the matching visible query, while
GitHub commits every preset filter into its query field; and Bitbucket used hand-drawn
SVG paths instead of GitHub/GitLab's host Tabler glyphs.

A second manual follow-up found that the Task preset rows still all used the generic
checklist fallback, because the plugin could not select the host's exact eye, message,
and tool preset icons. It also found no task-topbar CI status/popover contribution, so
linked Bitbucket pipelines were visible only inside the full review panel.

## Owned paths

- Host:
  - `apps/web/components/integrations/**`
  - `apps/web/components/github/my-github/{pr-list,list-toolbar}.tsx`
  - `apps/web/components/gitlab/my-gitlab/{mr-list,list-toolbar,start-task-menu}.tsx`
  - `apps/web/lib/plugins/{host-api,types}.ts` and focused tests
  - plugin authoring/API docs and this feature's spec/plan/ADR
- `kdlbs/kandev-plugin-bitbucket`:
  - `ui/**`
  - `internal/{domain,cloud,datacenter,plugin}/**`
  - `manifest.yaml`

## Implementation

1. Extract provider-neutral `ChangeRequestList`/row, task preset menu, and list
   toolbar under `components/integrations`; refactor GitHub and GitLab to consume them
   before exposing them with `IntegrationScopeBar` and `TaskRowIndicator` on `host.ui`.
2. Add human display-author and creation-time fields without replacing provider-
   canonical authorization identity. Add a provider-free association action that
   paginates workspace tasks and reads task-scoped Bitbucket link state.
3. Render Bitbucket results with the shared host components. Remove the row Review
   action, dashboard Watch action, and `/bitbucket?review=...` dashboard flow; retain
   `ReviewDetailPanel` only for the registered native task ReviewPanel. Watches remain
   in native integration settings.
4. Use GitHub/GitLab's **Review**, **Address feedback**, and **Fix CI** task presets.
   Selecting one mounts `host.ui.TaskCreateDialog` immediately with matched repository,
   source branch, task title, and prompt; successful creation links the PR and routes
   to the task.
5. Render linked tasks through the same host `TaskRowIndicator`; keep provider title
   external and row container inert.
6. Export host-owned semantic integration icons, use them for Bitbucket scope and row
   states, and keep runtime UI in the host repository rather than a shared UI repo.
7. Make Bitbucket's visible committed query the source of truth for scope state:
   selecting Open/All/Merged/Declined writes and commits the normalized state token;
   committing a query drives the same state sent to Cloud/Data Center.
8. Add semantic preset icon names to the shared menu and make GitHub and plugins resolve
   them through one host mapping; Bitbucket sends eye/message/tool for the three native
   presets.
9. Add the provider-neutral host `IntegrationChangeRequestStatus` control and
   `openTaskReview` callback. Register Bitbucket in `chat-top-bar`, refresh linked PR
   statuses on open and around every 90 seconds, and use the same desktop hover/mobile
   drawer interaction as GitHub.

## Mobile design contract

- Entry and hierarchy match `/github` and `/gitlab`: topbar, temporary filter Sheet,
  compact list toolbar, full-width change-request list.
- Row title opens Bitbucket. **Task** is the only row action; its host DropdownMenu
  receives Kandev's phone bottom-surface treatment and 44px trigger/items.
- Task creation uses the same native responsive `TaskCreateDialog`; review uses the
  host's task Review surface. No extra plugin Drawer or mobile dashboard detail route.
- Document has one vertical scroll owner and no horizontal overflow; shared host
  components own responsive geometry, while plugin state/query logic remains shared.

## TDD and verification

1. RED host tests for missing `host.ui` exports and shared row/task-menu behavior.
2. RED plugin unit/E2E tests asserting one **Task** dropdown, no **Review** button,
   direct native task dialog, linked-task refresh, and matching desktop/mobile flow.
3. RED Cloud/Data Center/workflow tests for display metadata and association paging.
4. Run focused host component/plugin API tests, GitHub/GitLab regressions, plugin unit
   and race tests, typecheck/lint/build, packaged desktop/mobile E2E, then live Cloud.
5. Compare fresh `/github`, `/gitlab`, and `/bitbucket` desktop/mobile screenshots;
   require source, installed, and served asset hashes to match with zero console errors.

## Risks

- New `host.ui` exports require a named host release before plugin publication.
- Task linking happens after host task creation; failures must not claim association
  success and must preserve navigation to the successfully created task.
- Provider display names are presentation-only; canonical account IDs remain the
  authorization identity and never get silently substituted.
- Shared primitives must stay provider-neutral; importing Bitbucket types or API logic
  into the host fails this task.

## Completion

- GitHub, GitLab, and Bitbucket now consume the same host-owned change-request list,
  toolbar, task menu, and linked-task indicator primitives.
- The live Cloud package presents one **Task** menu, opens `TaskCreateDialog` directly,
  keeps review in the linked task, and has no dashboard Review or Watch action.
- Verification passed: 1,010 host test files / 7,712 tests, focused host typecheck/lint/
  production build, 29 plugin UI tests, plugin Go race/vet/package checks, and eight
  live Cloud desktop/mobile Playwright checks.
- The manual follow-up is complete: preset clicks commit `state:*` into the visible
  query, committed queries drive provider requests, and Bitbucket consumes the same
  host-owned semantic Tabler icons as GitHub/GitLab.
- Final browser acceptance verified the exact host preset glyph classes: `IconEye`,
  `IconMessageDots`, and `IconTool`. The native task dialog loaded both live Bitbucket
  branches through the persisted provider bridge with HTTP 200 instead of a GitHub
  fallback/500.
- The host-native task-link dialog issued `pullrequests.link` successfully and all three
  disposable manual, auto-discovered, and watch-created tasks remained associated after
  restart.
- The task-topbar pipeline control passed desktop and native-touch acceptance. Opening it
  refreshed live status, desktop hover rendered the pipeline popover/link, mobile used a
  bounded no-overflow drawer with 44px controls, and both opened native Review. A live
  90-second polling cycle tracked `SUCCESSFUL -> INPROGRESS -> SUCCESSFUL`, changing the
  host glyph `check -> clock -> check`; the fixture was restored to `SUCCESSFUL`.
- Final focused host verification passed 154 tests, web typecheck, backend lint/build,
  plugin 37 UI tests, Go unit/race/vet/build, and five-platform package checksums.
