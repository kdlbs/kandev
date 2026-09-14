# Canvas UX follow-up investigation

Status: Investigated and planned on 2026-08-30. Implementation is not started.

Implementation plan:
[Plugin-backed canvases UX follow-up](../plugin-backed-canvases-ux-follow-up/plan.md).

This file records the canvas issues found after enabling `features.canvases`.
It uses the current screenshots, source, specifications, tests, and attached ACP
diagnostic bundle as evidence.

## Outcome summary

| ID           | Finding                                  | Result                                                                                          |
| ------------ | ---------------------------------------- | ----------------------------------------------------------------------------------------------- |
| CANVAS-UX-01 | Keep the sidebar section folded          | Partly implemented, but route and persisted-state behavior need correction and browser coverage |
| CANVAS-UX-02 | Add Canvases to workspace cards and tabs | Confirmed missing                                                                               |
| CANVAS-UX-03 | Create a canvas through a normal task    | Confirmed missing and requires a contract update                                                |
| CANVAS-UX-04 | Open a new canvas panel automatically    | Confirmed missing                                                                               |
| CANVAS-UX-05 | Explain lifecycle actions                | Confirmed missing                                                                               |
| CANVAS-UX-06 | Fill the available canvas height         | Confirmed Dockview portal sizing defect. Direct and phone routes are correct                    |
| CANVAS-UX-07 | Follow the live Kandev theme             | Confirmed protocol and scaffold gap                                                             |
| CANVAS-UX-08 | Reduce canvas skill reads                | Confirmed inefficiency. Keep the executor-portable MCP boundary                                 |

## Isolated browser reproduction

The reproduction used the production web and backend builds. It ran in the
isolated Playwright fixture, not the user's live instance.

- The existing desktop canvas acceptance test passed in 6.0 seconds.
- The existing phone canvas acceptance test passed in 11.9 seconds.
- A temporary diagnostic test measured the Dockview, direct-route, and phone
  layouts. The temporary files were removed after the run.
- A task canvas published while its task page was open. No canvas panel appeared
  before the manual panel action.
- A task route with no stored Canvases preference showed the section folded.
- A direct canvas route expanded the section and persisted `canvases: true`.
- The Dockview slot was 640 CSS pixels high. The canvas host used 194 pixels,
  and the iframe used 150 pixels.
- The direct desktop iframe used the complete 636-pixel application area.
- At a 393 by 727 phone viewport, the iframe used the complete 626-pixel
  application area. The document had no horizontal or vertical overflow.
- A real Kandev theme change changed the host theme. It sent no message to the
  iframe, and the iframe's computed colors did not change.

## Contract reconciliation

The current specifications describe two different canvas products.

- The agent-authored design permits a task-level **Create canvas** action. It
  sends a prepared prompt to a task agent.
- The older collaborative-canvas requirements forbid creation in the sidebar.
  They also describe a blank canvas dialog and native blocks.
- The shipped canvas is an agent-authored isolated web application. It has no
  blank block editor.

Before implementation, update the active agent-authored requirements and design.
Supersede the incompatible creation rules in the collaborative-canvas package.
The new contract must say that a canvas creation action starts a normal task.

## CANVAS-UX-01: Keep the sidebar section folded

### Evidence

- `CanvasesSection` passes `defaultExpanded={false}` in
  `apps/web/components/app-sidebar/sections/canvases-section.tsx`.
- The unit test manually sets the saved value to `false`. It does not test a
  first feature-enable transition with no saved Canvases key.
- `AppSidebar` expands a section when its route becomes active. The route map
  includes `/canvases/*`.
- Expansion state persists in local storage.
- The screenshot shows the section open and using vertical space.
- The isolated task route started folded when the saved state had no Canvases
  key. Enabling the feature alone did not expand the section.
- Opening a direct canvas route changed the state to expanded and saved that
  value. This explains the screenshot after direct canvas navigation.

### Required outcome

- A fresh user sees Canvases folded after enabling the feature.
- Opening a direct canvas route does not force the section open.
- A user can still open the section and retain that explicit preference.
- The folded header shows the active canvas count when the count is nonzero.
- The empty setup row appears only after the user expands the section.

### Acceptance checks

- Add a state test with no saved `canvases` key.
- Add a feature-off to feature-on test.
- Add a route test for `/canvases/:id` that keeps the section folded.
- Add a persistence test for an explicit user expansion.

## CANVAS-UX-02: Add Canvases to workspace cards and tabs

### Evidence

- `WORKSPACE_SETTINGS_TABS` does not contain `canvases`.
- `SectionCounts` and `SECTION_STATS` do not contain `canvases`.
- `WorkspaceCanvasesPage` and its settings route already exist.
- The settings tree adds a separate canvas link. This bypasses the shared tab
  catalog and causes the visible navigation mismatch.
- The canvas settings route renders `WorkspaceCanvasesPage` directly. It does
  not use `WorkspaceSettingsShell`, so it cannot show an active Canvases tab.

### Desktop outcome

- Add a feature-gated **Canvases** tab to each workspace settings page.
- Add a canvas count tile to a workspace card when the wide card layout can fit
  six tiles.
- Keep the existing five-tile layout at narrower desktop widths if six tiles
  can make labels unreadable.
- Include the canvas count in the workspace resource total.

### Mobile outcome

- Keep the Canvases tab in the horizontal, scrollable workspace tab strip.
- Do not depend on the workspace-card tile for access on a phone.
- Center the active Canvases tab with the existing tab-strip behavior.
- Preserve one page scroll owner and 44-pixel action targets.

### Acceptance checks

- Add catalog tests for the feature-disabled and feature-enabled states.
- Add responsive component tests for narrow and wide workspace cards.
- Add desktop and phone navigation tests for the Canvases tab.
- Test that archived canvases do not inflate the active count unless the label
  explicitly describes all canvases.

### Implementation note

Make the shared workspace tab catalog feature-aware. Remove the separate
`appendWorkspaceCanvasNodes` path after all tab consumers use the same filtered
catalog. Wrap the canvas page in `WorkspaceSettingsShell` with Canvases active.
Do not fetch canvas counts while the feature is disabled.

## CANVAS-UX-03: Create a canvas through a normal task

### Evidence

- The workspace canvas page only exposes **Refresh**.
- The sidebar header only exposes the settings shortcut.
- `TaskCreateDialog` accepts initial title and description values.
- The dialog supports **No repository**. An empty path creates a task-owned
  scratch workspace.
- Repository-free tasks already select an eligible local executor profile.
- The initial-values contract cannot select repository-free mode.

### Required flow

1. The user selects the `+` action in the desktop sidebar or workspace Canvases
   settings page.
2. Kandev opens the standard task creation dialog.
3. The dialog selects **No repository** and leaves the scratch path empty.
4. The dialog selects a compatible local executor profile.
5. The user selects the workflow and agent profile.
6. The dialog contains a localized canvas task title and prompt.
7. Kandev creates a normal task and opens its task details page.
8. The agent creates and publishes the canvas in that task context.

Suggested default prompt:

> Create a Kandev canvas that lists tasks grouped by repository. Add a button
> that moves each task to its next workflow step. Use the Kandev canvas
> authoring skill and publish the canvas when it is ready.

The prompt must not hardcode one workflow or agent profile. The user owns those
choices. The executor selection must use capability-based local selection, not
a stored profile identifier.

### Desktop outcome

- Add a compact `+` action beside the sidebar Canvases heading.
- Keep the existing management shortcut beside it.
- Give both icon actions accessible names and hover tooltips.
- Add a primary **Create canvas** action to workspace Canvases settings.

### Mobile outcome

- The desktop sidebar is absent on phones.
- Keep **Create canvas** visible in workspace Canvases settings.
- Use the existing full-screen task dialog on phones.
- Do not add a second canvas-only creation form.

### Implementation note

Add a reusable task-dialog preset instead of mutating dialog state from each
entry point. The preset must provide the title, prompt, scratch source mode,
and local-executor preference. Workflow and agent selectors remain editable.

### Acceptance checks

- Test both entry points against the same preset.
- Test scratch workspace creation with no repository and no explicit path.
- Test local executor selection when the last-used profile is Worktree.
- Test that workflow and agent profile choices remain editable.
- Add one desktop and one phone end-to-end creation flow.

## CANVAS-UX-04: Open the first published canvas automatically

### Evidence

- Lifecycle WebSocket events include `canvas_id`, `task_id`, scope, status, and
  active release state.
- The frontend canvas handler only increments an invalidation revision.
- Dockview can add a canvas panel, but it only does so from the manual panel
  menu.
- The agent-authored design says the task client opens the new canvas after the
  first release activates.
- The isolated browser published a canvas while its task page was open. The
  Dockview API had no `canvas:<canvas_id>` panel before the manual action.

### Required outcome

- When the active task receives its first valid `canvas.release.activated`
  event, add and activate `canvas:<canvas_id>` in Dockview.
- Do not open duplicate panels after retries or repeated events.
- Do not steal focus for a different task or workspace.
- A pending-permission first release may open a host status panel so the user
  can review it. It must not show an unusable blank frame.
- On a phone, navigate to the focused full-height canvas route instead of
  mounting Dockview.

### Acceptance checks

- Add reducer or handler tests for active-task matching and deduplication.
- Add a desktop browser test that publishes after the task page is open.
- Add a pending-permission review test without an API approval helper.
- Add a phone test for the focused route.

## CANVAS-UX-05: Explain releases, permissions, and promotion

### Evidence

- `CanvasDesktopActions` renders the buttons without Tooltip components.
- The labels do not explain when a user needs to review permissions or promote.

### Required outcome

- **Releases and permissions** explains that it reviews release history and
  approves new requested access.
- **Promote canvas** explains that it moves the task canvas to workspace scope
  after permission review.
- Disabled actions explain why they are disabled.
- Keep visible labels. Tooltips provide additional help only.
- Put equivalent help in the mobile action drawer. Do not require hover.

### Acceptance checks

- Add focus and pointer tooltip tests on desktop.
- Add a disabled-action explanation test.
- Add visible help text or descriptions to the phone drawer test.

## CANVAS-UX-06: Fill the available canvas height

### Evidence

- The screenshot shows unused space below the embedded application.
- `CanvasPage` and `WebAppFrame` use full-height classes.
- The canvas is rendered through a persistent Dockview portal.
- The portal element uses `display: contents` inside a block-level, full-height
  Dockview slot.
- `PageShell` relies on `flex-1` but has no `h-full` fallback for that block
  parent.
- The 640-pixel Dockview slot therefore gave `PageShell` only its 234-pixel
  content height. The host used 194 pixels, and the iframe used 150 pixels.
- The direct route used a flex parent and gave the iframe 636 pixels.
- The phone route gave the iframe 626 pixels and had no document overflow.
- Current tests only assert CSS class names. They do not compare rendered
  bounding boxes.

The root cause is the missing full-height flex boundary around the portaled
canvas route. The defect is specific to Dockview. Do not change the direct or
phone viewport calculations to fix it.

The narrow fix belongs at the canvas Dockview renderer boundary. Give the
portaled canvas route a full-height flex parent. Do not change the shared
`PageShell` contract for unrelated routes without separate evidence.

### Required outcome

- The iframe fills all space below host controls in a Dockview panel.
- The direct desktop route fills its application area.
- The phone route fills the available dynamic viewport and safe area.
- The host remains the only outer scroll owner.

### Acceptance checks

- Add browser assertions that iframe height matches the host application area
  within two CSS pixels.
- Cover a normal panel, a maximized panel, the direct route, and a phone route.
- Resize the viewport and Dockview group during the test.

## CANVAS-UX-07: Follow the live Kandev theme

### Evidence

- The isolated web-app context does not include a theme or semantic colors.
- `WebAppFrame` does not send theme data to the opaque-origin iframe.
- The canvas scaffold hardcodes dark colors such as `#101318` and `#191e27`.
- `color-scheme: light dark` does not map application colors to Kandev tokens.
- The trusted in-process plugin API has `theme` and `onThemeChange`, but the
  isolated canvas runtime cannot use that API.
- During the browser reproduction, a host theme change sent zero messages to
  the iframe. Its computed background, text color, and color scheme stayed the
  same.

### Required contract

- Expose the resolved `light` or `dark` mode.
- Expose a bounded semantic token set for background, foreground, card, muted,
  border, primary, accent, destructive, and focus ring colors.
- Deliver the initial values before the canvas becomes visible.
- Deliver live updates when the user changes the Kandev theme.
- Do not expose privileged host APIs or weaken the opaque-origin sandbox.
- Update the scaffold and UI reference to use semantic CSS variables with safe
  fallbacks.

The implementation needs a small protocol design. A host-to-frame message is a
possible transport because an opaque frame cannot read host CSS. The design
must define message versioning, source validation, token bounds, and startup
ordering before code changes.

### Acceptance checks

- Add protocol tests for the initial theme payload and updates.
- Add browser tests for dark-to-light and light-to-dark changes.
- Assert computed canvas colors, not only a text label.
- Cover direct desktop, Dockview, and phone hosts.

## CANVAS-UX-08: Reduce canvas skill reads

### ACP evidence

The attached normalized ACP trace contains one canvas creation session.

- The agent made nine `read_canvas_authoring_skill_kandev` calls before it
  called `create_canvas_kandev`.
- It read the main skill and six reference files.
- It guessed two scaffold paths. Both calls failed with
  `skill_path_invalid`.
- The nine calls spanned about 12.4 seconds.
- The create call started about 20.7 seconds after the first skill read.
- The main skill did not provide the exact scaffold inventory.
- The agent later recreated the scaffold structure manually.

### Absolute-path assessment

Do not return the Kandev host's absolute skill path as the general solution.
That path does not exist inside Docker or SSH executors. It can also be outside
the local agent's file sandbox. The current MCP read boundary gives every
executor the same authenticated and allowlisted contract.

An executor-local absolute path requires deployment or mounting into each
executor. That reintroduces synchronization and cleanup work that the current
design intentionally avoids.

### Recommended outcome

- Keep one executor-portable MCP read boundary.
- Make the default read return a compact authoring bundle in one call.
- Include the main workflow, exact file inventory, manifest contract, browser
  protocol summary, theme rules, and minimal scaffold files.
- Keep detailed references available through optional path reads.
- Return the exact available paths in `create_canvas_kandev` and skill-read
  responses.
- Consider creating the minimal scaffold in the assigned source directory when
  `create_canvas_kandev` runs. The agent can then inspect and edit it with its
  native file tools.
- Change the skill text so it does not tell an agent to read the main document
  again after the tool has already returned it.

### Acceptance checks

- Add a golden inventory test that includes every documented path.
- Add a tool contract test for the one-call core bundle.
- Add local, Docker, and SSH authoring tests with the same response shape.
- Add an ACP evaluation that creates the sample canvas with at most one core
  skill-read call and no invalid path calls.

## Ordered implementation todo

- [x] Reconcile the active requirements and system design with the task-based
      creation flow and theme protocol.
- [ ] Add the feature-gated workspace Canvases tab and responsive card count.
- [ ] Make the sidebar folded behavior deterministic without discarding an
      explicit user preference.
- [ ] Add one shared canvas task-creation preset.
- [ ] Add create actions to workspace settings and the desktop sidebar.
- [ ] Auto-open the first task canvas from lifecycle events.
- [ ] Add desktop tooltips and mobile lifecycle-action explanations.
- [ ] Measure and fix full-height layout in real browsers.
- [ ] Add the isolated canvas theme and semantic-token protocol.
- [ ] Update the authoring scaffold and UI guidance to consume theme tokens.
- [ ] Replace repeated skill reads with one core bundle and exact inventory.
- [ ] Run focused unit tests, desktop Playwright, and phone Playwright.
- [ ] Run the full web lint, type check, i18n checks, and canvas backend tests.

## Definition of done

- A user can start canvas creation from workspace settings or the desktop
  sidebar.
- The standard task dialog opens with a scratch workspace and prepared prompt.
- The user still selects the workflow and agent profile.
- The first release appears in the task without a reload or manual panel action.
- Canvas controls explain their effects on desktop and phone.
- The iframe fills its host and follows live Kandev theme changes.
- The agent receives current authoring guidance without repeated or failed
  skill reads.
- All behavior remains gated by `features.canvases`.
