# Implementation

Status: complete for local testing.

1. Persist primary chief in Office repository; expose user-only channel setup and
   primary-choice endpoints. Reuse workflowEnsurerAdapter for existing workspaces.
2. Add explicit execution_profile_id to routing.AgentOverrides; resolve and validate
   complete profile using existing execution-profile store and health checks.
3. Add workspace settings Agents tab, connection setup and management cards using
   office API, scoped data hooks and existing executor/profile catalogs.
4. Add persistent Chief entry to app-sidebar-primary-nav and mobile Office navigation;
   conversation links preserve workspace and provide return to agent/settings.
5. Verify Go repository/routing/handler tests, TypeScript, lint/i18n, and browser
   setup/navigation across reload and workspace switching. Rebuild isolated preview.

User explicitly authorized implementation after reviewing the navigation proposal.
No production installation or existing account configuration will be replaced.

## Final behavior

Kanban onboarding runs before Office setup. Workspace settings > Agents attaches
personas to existing execution profiles and executor profiles. One explicit chief
is persisted per workspace. Delegation guidance is stored in agent settings and a
bounded directory is supplied to coordinators. Existing workspaces enable Office
workflows without another workspace, default CEO or routine. Board/Office links
preserve explicit workspace selection; chief navigation and task return links close
the loop. Profile and guidance edits use the shared settings save coordinator.

## Verification

- Office package race suite passed, including repository, routing, scheduler,
  service and model tests. User-only setup endpoints reject agent callers.
- Pinned-profile tests cover exact selection, foreign/deleted profiles, no account
  fallback after failure, and accurate configured-profile previews.
- Browser suite: 4 passed for routines, repeated conversations and child results,
  workspace setup/profile execution/context persistence, and Kanban-first setup.
- Final board navigation regression: 2 passed against the final rebuilt assets,
  including returning from the board to the same chief conversation.
- Frontend typecheck, targeted lint, i18n and public documentation checks passed.
- The historical preview used separate state and preserved existing configuration.
  Its machine-specific URL and paths are omitted from publication.

Provider integration tests use mock agents. Real subscriptions and model delegation
judgment require user testing. Delegation guidance is advisory; workspace runtime
permissions and selected profile/executor configuration control access. Legacy
explicit new-Office-workspace setup remains available, after Kanban onboarding.
