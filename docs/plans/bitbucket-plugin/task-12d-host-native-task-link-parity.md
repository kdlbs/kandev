---
id: "12d-host-native-task-link-parity"
title: "Use the host-native task change-request link dialog"
status: completed
wave: 3d
depends_on: ["12c-exact-code-host-ui-parity"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md"
---

# Task 12d: Use the host-native task change-request link dialog

## Confirmed root cause

`TaskActionRegistration.run()` supplied verified task context but no native linking
surface. The Bitbucket plugin therefore used `host.openModal()` and hand-built a form
whose width, description placement, footer, validation, success handling, mobile
presentation, and icon differed from `TaskGitHubPRDialog`. Existing tests asserted only
action invocation and modal presence. The real plugin also registered “Link Bitbucket
Pull Request” even though the fixture and built-in submenu items name only their target.

## Implementation

1. Extract GitHub's one-input link form into a provider-neutral host component with
   native inline errors, Cancel/Save footer, submitting state, toast, and close behavior.
2. Add an additive `host.openTaskLinkDialog()` contract that mounts that exact component
   in host-owned dialog geometry; refactor GitHub to consume the shared form first.
3. Propagate registered plugin icon names into both task-card and task-switcher Link
   menus instead of substituting a generic link glyph.
4. Register Bitbucket as **Bitbucket Pull Request**, submit its parsed reference through
   the authenticated `pullrequests.link` action, and remove its copied link form.
5. Update the in-tree fixture and desktop/mobile E2E to prove native anatomy, validation,
   action invocation, close-on-success, toast, icon, and menu copy.

## Mobile contract

- Entry remains the visible task actions menu and its existing **Link** submenu.
- Nearest exemplar is `TaskGitHubPRDialog`; Bitbucket uses the same dialog instead of a
  plugin-only bottom drawer so behavior and focus return remain identical.
- One dialog owns scrolling; input and footer actions remain touch reachable without
  document horizontal overflow.

## Validation

```bash
cd apps && pnpm --filter @kandev/web exec vitest run \
  components/integrations/task-change-request-link-form.test.tsx \
  components/task/task-github-pr-dialog.test.tsx \
  components/plugins/plugin-modal-host.test.tsx \
  components/kanban-card-menu-items.test.tsx \
  lib/plugins/host-api.test.ts
cd ../kdlbs-kandev-plugin-bitbucket && npm test
cd ../kandev-2/apps/web && pnpm e2e:run e2e/tests/plugins/bitbucket-plugin-contract.spec.ts
```

## Acceptance

- Both Link menus show a Bitbucket brand icon and **Bitbucket Pull Request**.
- GitHub and Bitbucket render the same form anatomy and interaction states.
- Invalid input stays open with inline error; success links once, closes, and toasts.
- Desktop and mobile E2E complete the same user outcome with no overflow.
- Full current-worktree review records any unrelated blockers before merge readiness.
