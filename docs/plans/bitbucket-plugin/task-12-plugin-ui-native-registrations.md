---
id: "12-plugin-ui-native-registrations"
title: "Plugin UI and native registrations"
status: completed
wave: 3
depends_on: ["04-frontend-plugin-registry", "05-dynamic-composer-reference-sources", "08-native-repository-provider", "09-native-link-review-surfaces", "10-cloud-dc-domain-auth"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 12: Plugin UI and native registrations

## Intent

Build plugin-owned settings, queue, review, and mobile UI; register it with native
repository, task-action, review-provider, and reference-source contracts.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree: TypeScript UI bundle, route/nav,
  connection/settings components, queue/review panels, native registration module,
  responsive styles, view-model/component tests, and plugin Playwright tests.

## Dependencies

Tasks 04, 05, 08, 09, and 10.

## Acceptance

1. Native **Settings > Integrations > Bitbucket** provides connection/authentication,
   health, and saved-watch configuration; `/bitbucket` provides browse/search, queue
   filters, review details, status presentation, and task launch/link/unlink/create-PR
   through authenticated plugin actions.
2. Plugin registers provider `bitbucket`, Link actions, native review panel, and
   `bitbucket`/`pull_request` composer source; its review component works in host
   desktop and mobile surfaces.
3. Mobile uses focused list/detail route state, one scroll owner, `100dvh`, safe areas,
   >=44px controls, and Drawers for filters/actions rather than compressed panes.

## Verification

```sh
pnpm lint
pnpm typecheck
pnpm test
pnpm e2e
```

## Risks

Capability flags must remove or explain unavailable Cloud/DC controls. Do not duplicate
host UI or add a Bitbucket-specific frontend branch; plugin unload must tolerate
remounting and remove its registrations.
