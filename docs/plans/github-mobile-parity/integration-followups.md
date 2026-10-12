# Integration mobile-usability follow-ups

Assessed 2026-09-11 from the native integration UI, live [plugin registry](https://github.com/kdlbs/kandev/blob/main/plugin-registry/plugins.yaml), and each plugin's default-branch source. The user requested one persistent subtask per needed repository, each with its own workspace.

## Created planning tasks

All four were created under parent `a7d164bc-194f-4b79-941a-60a6f1280b47` with `workspace_mode: new_workspace`, `start_agent: false`, and inherited agent/executor profiles. Task state can subsequently change when the user starts planning. Their deliverable is requirements, system design, an implementation plan, and provider-specific work orders, followed by a review checkpoint. Creation does not authorize implementation, publishing, or additional agents.

### Later implementation authorization

On 2026-09-11, the user explicitly asked the parent to share the accepted GitHub changes and have these tasks begin implementation (parent conversation message `be8d0b0e-d707-447e-b727-3712fe5f7a22`). The native, Bitbucket and Slack tasks had completed design packages; their existing sessions received the implementation handoff with source paths, final toolbar/page-drawer behavior, test evidence, provider-semantic limits and the stopped-instance warning. This later instruction supersedes the original planning-only phase for those packages, not their technical dependency gates or repository ownership.

Bitbucket started its independent saved-query recovery order, and Slack's existing session resumed. Native received the direct user-message identifier to verify implementation authority and was asked to begin independent Sentry/generated-settings work where its dependency gates allow. Shared work still requiring the unmerged parent patch must not be reported as unblocked.

YouTrack had no session or plan despite the user's expectation that all four were started. Messaging returned `NOT_FOUND`; moving it to the configured In Progress start step still left zero sessions. Its implementation/context brief is prepared, but no additional session was silently created. Its missing planning package also remains a prerequisite rather than assumed reviewed work.

| Repository and verified base | Existing surfaces in scope | Kandev task |
| --- | --- | --- |
| [kdlbs/kandev](https://github.com/kdlbs/kandev), `main` | GitLab, Jira, Linear, Azure DevOps, Sentry; genuinely needed host/SDK primitives | `b6f16caa-4ada-45ee-8c96-b4550f69ac8f` |
| [kdlbs/kandev-plugin-bitbucket](https://github.com/kdlbs/kandev-plugin-bitbucket), `main` | PR dashboard, saved queries, review/task actions and watches | `71b6794f-2399-4920-b49f-e6aae2223c13` |
| [ahmedbally/kandev-plugin-youtrack](https://github.com/ahmedbally/kandev-plugin-youtrack), `main` | Issues, saved views, filters, task actions and watcher dialog | `ea7a86cd-4102-4e8c-894e-d04576232aa8` |
| [kdlbs/kandev-plugin-slack](https://github.com/kdlbs/kandev-plugin-slack), `master` | Existing connection/settings/activity panel, not a new dashboard | `c2e6a1ec-86d5-450e-a3d7-0fbc56a209c4` |

The native providers share one repository task but require separate work orders. Plugin code stays in its actual repository. YouTrack is community-owned; later delivery needs a verified contribution route and explicit authorization rather than assuming push access.

## Native evidence and planning scope

- **GitLab:** `apps/web/app/gitlab/gitlab-page-client.tsx` hides desktop preset controls below `md` and provides a separate hamburger/left Sheet. Assess named Views discovery, saved-query recovery, and compact truthful pagination. It consumes the shared toolbar updated in the parent task.
- **Jira:** `apps/web/components/jira/my-jira/list-toolbar.tsx` combines search, views, save, count, sorting, refresh, and JQL in a wrapping desktop toolbar. Its results footer also needs hierarchy and touch review. Jira's `/search/jql` pagination is token-based without a total; arbitrary page jumps and synthetic counts are not valid adaptations.
- **Linear:** `apps/web/app/linear/linear-page-client.tsx` uses fixed-width team/assignee controls and a desktop-shaped next/previous footer. Assess filters, long issue labels, task actions, and cursor/loaded-page feedback. Do not silently add saved views where none exist.
- **Azure DevOps:** `apps/web/app/azure-devops/azure-devops-page-client.tsx` uses a left filter Sheet while mode/scope controls remain inline. Assess work-item and PR mode controls, filters, status and linked tasks. Preserve its existing phone board's focused-column navigation.
- **Sentry:** There is no top-level `/sentry` dashboard, as documented in `docs/public/integrations.md`. Assess existing task issue-selection/current-task links, `components/sentry/sentry-issue-watch-table.tsx`, watch forms, and multi-instance settings. A new dashboard requires a separate approved proposal.

## Registry evidence and planning scope

**Bitbucket:** [ui/src/bitbucket-page.ts](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/main/ui/src/bitbucket-page.ts) consumes host toolbar and cursor pagination; [ui/src/dashboard-scope.ts](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/main/ui/src/dashboard-scope.ts) puts desktop scope controls inside a left Sheet. Plan an intentional mobile choice surface, discoverable existing saved-query actions, balanced metadata, matching touch geometry, and packaged mobile/desktop tests. Retain discovered-cursor navigation rather than copying GitHub's known-total page picker.

**YouTrack:** [ui/bundle.js](https://github.com/ahmedbally/kandev-plugin-youtrack/blob/main/ui/bundle.js) contains narrow toolbar overrides, a fixed-width saved-view Popover, hover-only deletion, input clearing at save time, wide issue-list padding, and a custom watcher overlay. Plan touch-accessible view actions and retry-safe saves, filter/sort hierarchy, readable issue/task actions, and safe-area-aware dialogs. Retain Load more semantics.

**Slack:** [ui/bundle.js](https://github.com/kdlbs/kandev-plugin-slack/blob/master/ui/bundle.js) registers a plugin-settings status card, not an issue dashboard. Its unwrapped connection/team/user header and activity timestamps/actions are analogous density problems. Limit planning to those existing surfaces and the credential-form layout; do not invent saved views or pagination. Tests must mock status/activity and never send real messages or scans.

The live registry's other entries are tools or analytics: session-cost, provider-usage, Kandy, task-manager, GitHub Status, tags, notes, voice, and DeepSeek balance. They do not expose the provider work dashboards requested here, so no speculative tasks were created for them.

## Shared constraints and dependency order

1. Each task audits its current default branch and captures mobile evidence in a disposable mocked instance. No access to the developer's main :9998 data, provider credentials, or live-provider mutations is needed.
2. Plans preserve provider capabilities and pagination semantics while addressing 44px touch targets, desktop density, long/translated labels, empty/loading/error/retry states, focus/Back, internal scrolling, safe areas, and 320px/767px/768px geometry.
3. The parent's GitHub/shared-toolbar changes are not yet merged. Native host/SDK changes required by plugins belong to the native task, with explicit exports, compatibility tests and docs. Plugin work depends on those changes; no private host imports or duplicated host implementations.
4. Initial plans return for review before implementation. The later user instruction above permits the completed packages to proceed; missing packages and host dependencies remain explicit gates. Packaged plugin tests and release/minimum-host compatibility are not permission to publish or install anything in a personal instance.
