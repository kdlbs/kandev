---
spec: docs/specs/agents/requirements/runtime-updates.md
created: 2026-07-27
status: complete
---

# Implementation Plan: Agent Runtime Update Dialog

## Overview

PR #1950 placed retained update state, versions, output, and results directly
inside installed-agent cards. It also configured PR previews with
`KANDEV_MOCK_AGENT=only`, which intentionally removes every real agent from the
registry and therefore leaves the install catalogue empty. This repair restores
the preview catalogue, adds a read-only update-preview contract, and moves the
approval and ephemeral streaming lifecycle into a responsive dialog.

## Confirmed root causes

- `apps/backend/cmd/preview/sprite_ops.go` exports
  `KANDEV_MOCK_AGENT=only`. `registry.Provide` treats `only` as an isolated E2E
  mode and registers only `mock-agent`; `ListAvailableAgents` can only return
  registry entries, so Settings receives no real install candidates.
- `AgentRuntimeUpdateControl` is rendered inside `InstalledAgentCard`, so all
  versions, output, and terminal state expand the card.
- `useAgentRuntimeUpdates` calls `listAgentUpdateJobs` on mount and upserts
  retained backend jobs into the global Settings store, explicitly restoring
  output and terminal state after a page restart.
- The only preparation path is `POST /agent-update/:agentName`; target-version
  resolution happens after enqueue, so the UI cannot show the next version and
  exact command before mutation.

## Backend

### Preview environment catalogue

Change `buildExtractScript` in
`apps/backend/cmd/preview/sprite_ops.go` to use
`KANDEV_MOCK_AGENT=true`. That mode retains the enabled mock agent while loading
the built-in real-agent catalogue, allowing unavailable agents with install
scripts to appear in Settings. Update the focused script assertion in
`apps/backend/cmd/preview/sprite_ops_test.go`; keep the registry mode test as
supporting evidence that `true` loads defaults and `only` does not.

### Read-only update preview

Add `AgentUpdatePreviewDTO` in
`apps/backend/internal/agent/settings/dto/dto.go` with agent, package, current
version, target version, argv, and display command fields.

Add `Controller.PreviewAgentUpdate(ctx, name)` in
`apps/backend/internal/agent/settings/controller/agent_update.go`. It validates
the same built-in `ManagedNPMRuntimeAgent` boundary as enqueue, reads the current
capability version, resolves the upstream target through `RuntimeUpdater`, and
serializes `CacheUpdateCommand()` with the existing `buildCommandString`
helper. Extend that helper to preserve empty argv entries as `""`, so the
displayed recipe remains an exact representation of the direct argv command.
It never claims maintenance state, creates a job, or executes the update command.

Register `GET /api/v1/agent-update/:agentName/preview` and a focused handler in
`apps/backend/internal/agent/settings/handlers/handlers.go`. Reuse the existing
update error classification for not-found, unsupported, unavailable, and
version-resolution failures.

## Frontend

### API and page-local update lifecycle

Add `AgentUpdatePreview` and `previewAgentUpdate` to
`apps/web/lib/api/domains/agent-update-api.ts`.

Refactor `useAgentRuntimeUpdates` so it does not list or hydrate retained update
jobs on mount. Starting an update returns or binds the accepted job identity for
the currently open dialog; WebSocket snapshots and chunks may continue using
the existing bounded Settings store, but the dialog displays only the job
started or recovered during its current page-local interaction.

### Installed-agent card and responsive update surface

Replace the inline `AgentRuntimeUpdateControl` composition with a compact
icon-only trigger in `apps/web/components/settings/installed-agent-card.tsx`.
The icon remains visually small while its standalone hitbox is at least 44px
and has an agent-specific accessible label and tooltip. Unmanaged agents omit
the trigger.

Replace `agent-runtime-update-control.tsx` with a responsive update surface:

- Desktop uses a centered `Dialog`.
- Phone uses an inset bottom `Drawer`, following
  `components/kanban/mobile-menu-sheet.tsx`, because this is a temporary,
  single-purpose approval flow rather than a primary route.
- Header and approval/dismiss actions remain fixed; one `min-h-0 flex-1
  overflow-y-auto` body owns scrolling and clears the bottom safe area.
- Opening fetches the read-only preview and shows current/target versions,
  host-only scope, automatic capability refresh, unchanged active sessions,
  and the exact wrapping command.
- Approval is disabled until preview succeeds and is the only action that
  sends the update POST.
- After approval, the same surface shows phase, bounded stdout/stderr, terminal
  result, and retry. Closing or restarting the page discards the UI selection
  and presentation state.
- No update information is rendered in the card.

### Public documentation

Update `docs/public/agents-and-profiles.md` to describe the icon-triggered
preview and approval flow, the exact-command disclosure, dialog-only streaming,
and the intentionally ephemeral browser presentation. The preview launcher
environment change is internal and does not need public environment-variable
documentation.

## Tests

- **Preview catalogue mode:** change `TestBuildExtractScript` first and confirm
  it fails while the script still exports `only`, then make it pass with
  `true`. File: `apps/backend/cmd/preview/sprite_ops_test.go`.
- **Read-only preview:** controller tests prove current/target/command output,
  empty-argument display, unmanaged rejection, registry failure, and absence
  of update execution.
  File:
  `apps/backend/internal/agent/settings/controller/agent_update_test.go`.
- **Exact display command:** extend the existing table test for
  `buildCommandString` with an empty argument. File:
  `apps/backend/internal/agent/settings/controller/controller_test.go`.
- **Preview handler:** handler tests prove the GET response and errors while
  preserving POST enqueue behavior. File:
  `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`.
- **Ephemeral state helper behavior:** retain focused Settings slice/WS tests
  for bounded same-job output merging, and remove the retained-job hydration
  expectation from the hook path. Files:
  `apps/web/lib/state/slices/settings/settings-slice.test.ts` and
  `apps/web/lib/ws/handlers/agents.test.ts`.
- **Public docs:** run `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`.

## E2E Tests

- **Desktop approval:** opening the card icon shows versions, explanation, and
  exact command while POST count remains zero; approving starts one job and
  streams output/result inside the dialog.
- **Restart reset:** after progress or a terminal result, restarting the page
  leaves only the icon on the card; a newly opened dialog contains a fresh
  preview and no prior output/result.
- **Mobile parity:** the Pixel 5 flow taps the 44px icon, uses the bottom
  drawer, reads the preview before approval, streams a long output without
  document horizontal overflow, and reaches the fixed final actions.

Files:

- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation. No subagents are
authorized.

Wave 1:

- [x] [Task 01: Restore the preview install catalogue](task-01-preview-catalogue.md)
- [x] [Task 02: Add the read-only update preview API](task-02-update-preview-api.md)

Wave 2:

- [x] [Task 03: Move updates into an ephemeral responsive dialog](task-03-update-dialog-ui.md)

Wave 3:

- [x] [Task 04: Verify desktop and mobile update flows](task-04-update-dialog-e2e.md)

Tasks 01 and 02 touch disjoint files, but remain sequential unless the user
explicitly authorizes subagents after the model checkpoint.
