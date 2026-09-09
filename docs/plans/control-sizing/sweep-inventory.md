# Control sizing sweep inventory

## Status and method

Snapshot: 2026-09-10. Production implementation is complete.

A read-only TypeScript AST scan examined 1526 TSX source files under
`apps/web/components`, `apps/web/app`, and `apps/packages/ui/src`.
It found 681 candidate attributes across 315 files.
The scan inspected button-like tags, Inputs, SelectTriggers, Comboboxes,
CommandInputs, and dialog actions for explicit height classes and large sizes.

These are candidates, not 681 confirmed bugs.
Responsive branches, content-sized rows, and correct touch-only controls also match.
The scan does not resolve named class constants or inherited geometry.
It does not establish which components mount in each viewport.

Every listed candidate has an inspected control and one final disposition. A checked box records source review and, where the control is covered by a representative flow, rendered evidence. Follow-up helper, CSS, size-prop, and padding-only findings are listed in the additional implementation section.

## Confirmed starting points

| Source | Evidence | Intended correction |
| --- | --- | --- |
| `task/chat/session-stopped-banner.tsx` | Three actions use unconditional min-h-11 | Standard desktop height plus scoped touch minimum |
| `task/chat/dynamic-route-recovery.tsx` | Retry/skip/cancel/stop share unconditional min-h-11 | Standard desktop actions |
| `settings/layouts/layout-profile-list.tsx` | Action wrapper uses sm:min-h-8 | Standard desktop actions |
| `settings/layouts/layout-settings.tsx` | Input uses sm:min-h-9 and actions use sm:min-h-8 | Aligned standard fields/actions |
| `settings/workspaces/workspace-settings-shell.tsx` | Heading picker overrides h-9 | Standard trigger height |
| `settings/workflow-section-actions.tsx` | Ordinary toolbar actions use size=sm | Standard ordinary actions |
| `settings/repository-secret-bindings.tsx` | Inputs/selects/actions use unconditional 44px classes | Aligned desktop group and touch targets |
| `settings/system/storage/storage-action-button.tsx` | Button and disabled wrapper impose min-h-11 | Shared standard dimensions |
| `settings/system/storage/storage-policy-fields.tsx` | settingsControlClassName receives h-11 | Remove caller override |
| `settings/system/storage/storage-adoption-field.tsx` | Input uses h-11 | Standard field |
| `settings/settings-control.ts` | Only a desktop minimum constrains callers | Shared complete sizing contract |
| `task/workflow-step-disclosure.tsx` | Move here already uses h-7 and coarse h-11 | Preserve action sizing and inspect row spacing |

Paths in this table are relative to `apps/web/components`.
The table includes helper files outside the AST candidate list.

## Complementary search

```bash
rg -n '(min-h-|max-h-|h-|size-)(8|9|10|11|12|\[)|size="(sm|lg)"|height\s*:' \
  apps/web/components apps/web/app apps/packages/ui/src \
  -g '*.tsx' -g '*.ts' -g '*.css' -g '!*.test.*'
rg -n 'settingsControlClassName|settingsActionClassName|touchButtonClass|buttonVariants|triggerClassName' \
  apps/web/components apps/web/app apps/packages/ui/src
```

Inspect padding-only raw buttons, wrappers, flex stretching, and data-size selectors separately.
Repeat these searches after each migration family.
Do not mechanically replace every result.

## Disposition key

- **changed in implementation:** the candidate was migrated to the shared standard or compact contract.
- **already conforming:** the candidate already receives the correct role from a shared primitive.
- **touch-only:** the candidate is intentionally a touch composition.
- **content-sized:** the candidate is a content surface such as a row, card, menu, chip, or picker option.
- **documented exception:** the candidate is specialized chrome covered by the system design exceptions.

## Shared primitives

- [x] `apps/packages/ui/src/sidebar.tsx` — documented exception; specialized chrome retains independent geometry.

## Task surfaces

- [x] `apps/web/components/task-create-dialog-advanced-settings.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-create-dialog-dependencies.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-create-dialog-footer.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task-create-dialog-launch-preview-control.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task-create-dialog-pill.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-create-dialog-priority-select.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task-create-dialog-remote-repo-chip.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task-create-dialog-repo-chip-parts.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task-create-dialog-repository-sets-control.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-create-dialog-source-mode.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-create-dialog-workspace-repo-chips.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/add-workspace-sources/add-folder-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/branch-picker-list.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/changes-panel-dialogs.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/anchored-last-prompt-bar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/clarification-overlay-header.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/clarification-panel-section.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/clarification-status-banner.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/context-items/context-chip.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/dynamic-route-recovery.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/chat/entity-reference-menu.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/mcp-explorer/mcp-explorer-header.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/mcp-explorer/mcp-indicator.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/mcp-explorer/mcp-server-list.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/mcp-explorer/mcp-tool-detail.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/mcp-explorer/mcp-tool-list.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/message-list-shared.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/messages/action-message-actions.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/chat/messages/action-message-recovery.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/chat/messages/agent-status.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/chat/messages/message-comment-surface.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/messages/prompt-mention-components.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/messages/tool-subagent-message.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/popup-menu.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/queued-ghost-list.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/queued-ghost-message.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/queued-ghost-panel-header.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/queued-ghost-row-actions.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/chat/reset-context-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/scroll-to-last-prompt-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/chat/session-stopped-banner.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/commit-detail-panel.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/ensure-session-error.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/executor-environment-info.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/executor-settings-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/file-browser-parts.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/file-browser-toolbar.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/file-upload-conflict-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/file-viewer-header.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/html-preview-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/markdown-preview-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/mobile-pill-button.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/mobile-terminal-keybar-helpers.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/mobile-terminal-row.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/plugin-panel-picker.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/session-mobile-top-bar.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/task/mode-selector.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/passthrough-toolbar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/port-forward-dialog-actions.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/port-forward-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/preview-plan-panel.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/prompt-history-panel-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/prompt-history-panel-row.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/remote-cloud-tooltip.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/remote-contribution-header-actions.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/remote-contribution-resolution-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/review-item-selector.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/session-dialog-shared.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/sessions-dropdown.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/sidebar-filter/automatic-color-repository-picker.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/sidebar-filter/automatic-color-rule-card.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/sidebar-filter/automatic-color-rule-fields.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/sidebar-filter/automatic-color-settings.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/sidebar-filter/sidebar-filter-bar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/sidebar-filter/sidebar-settings-disclosure.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/sidebar-filter/sidebar-view-chips.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/sidebar-filter/sort-picker-primitive.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/sidebar-filter/task-row-settings.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/sidebar-filter/typed-filter-clause-editor.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/sidebar-filter/view-manager.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/simple/OfficeSimplePane.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/simple/components/labels-picker.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/simple/components/managed-runtime-npm-run-error.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/simple/components/multi-select-popover.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/simple/components/run-error-entry.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/simple/components/task-launch-error-entry.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/task-page-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/task-pr-picker-dialog.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/task/task-unarchive-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task/workflow-step-disclosure.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/workspace-switcher.tsx` — already conforming; retained after source and responsive-geometry review.

## Settings surfaces

- [x] `apps/web/app/settings/agents/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executor/[id]/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executor/[id]/profile/[profileId]/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/k8s/[executorId]/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/new/[type]/kubernetes-create-page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/new/[type]/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/new/[type]/ssh-create-page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/executors/ssh/[executorId]/page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/workspace/workspace-repositories-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/app-sidebar/sections/settings/settings-search.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/account/security-settings.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/settings/agent-profile-delete-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/agent-profile-page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/agent-profile-picker.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/settings/agent-runtime-update-control.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/agents/agent-profiles-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/canvas-host-components.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/canvas-lifecycle-dialogs.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/cli-flags-field.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/copy-move-dialog-body.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/dynamic-agent-candidate-list.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/dynamic-agent-policy-editor.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/dynamic-agents-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/executor-profiles-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/installed-agent-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/keyboard-shortcuts-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/layouts/layout-editor-toolbar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/layouts/layout-profile-delete-confirmation.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/layouts/layout-profile-list.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/layouts/layout-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/mcp-strategy-select.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/settings/model-config-resolution-status.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/model-fallback-settings-shell.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/settings/notification-events-table.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/settings/notifications-settings-external-providers.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/plugins/marketplace-sources-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/plugins/plugin-detail.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/plugins/plugin-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-advanced-options.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/settings/profile-edit/env-vars-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-edit/inline-secret-select.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-edit/portable-config-bundles.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-edit/profile-connection-settings-action.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-enabled-help.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/profile-status-panels.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/prompt-row-actions.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/repository-branch-policies.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/repository-branch-policy-fields.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/repository-secret-bindings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/runtime-version-picker.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/secrets-delete-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/secrets-list-item-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/settings-floating-save.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/sleep-inhibition-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/bundle-customizer.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/feature-toggles-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/log-viewer.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/message-queue-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/storage/storage-action-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/storage/storage-adoption-field.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/storage/storage-confirmation-dialogs.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/storage/storage-policy-fields.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/storage/storage-setting-help.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/settings/units/unit-members-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-cycle-diagnostic.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-pipeline-editor-panels.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-pipeline-editor-wip-controls.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-pipeline-editor.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-session-config-carry-warning.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-session-config-editor.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-session-config-rule-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-step-agent-profile-selector.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/settings/workspace-canvases-page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workspaces/workspace-placement-card.tsx` — changed in implementation; verified by source review and focused checks.

## Office surfaces

- [x] `apps/web/app/office/agents/[id]/components/agent-configuration-tab.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/inbox/inbox-page-client.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/projects/[id]/project-header.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/projects/create-project-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/projects/project-repository-picker.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/routines/routine-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/setup/close-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/setup/step-agent.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/activity/activity-feed.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/costs/create-budget-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/org/org-zoom-controls.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/routing/components/role-tier-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/settings/export/export-file-tree.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/workspace/skills/skill-list.tsx` — changed in implementation; verified by source review and focused checks.

## Other application surfaces

- [x] `apps/web/app/gitlab/gitlab-page-client.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/app/tasks/tasks-list-controls.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/tasks/tasks-list-view.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/tasks/tasks-pagination.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/app-sidebar/app-sidebar-section.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/app-status-bar/app-status-surface-provider.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/app-status-bar/backend-reload-required-alert.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/automations/automation-delete-confirm-dialog.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/automations/automations-export-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/automations/automations-list-page.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/automations/schedule-selector.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-board.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-default-queries.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-quick-actions.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-watch-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/azure-devops/azure-devops-work-item-detail.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/branch-refresh-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/canvas/canvas-task-create-launcher.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/combobox.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/command-panel-footer.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/command-panel-scope-switcher.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/config-chat/config-chat-panel.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/config-chat/config-chat-setup.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/confirmation/action-confirm-popover.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/create-local-repository-surface.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/editors/codemirror/codemirror-code-editor.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/editors/lsp-status-button.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/editors/tiptap/plan-bubble-menu.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/enhance-prompt-button.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/folder-picker.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/github/action-presets-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/default-queries-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-access-help.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-connection-panel.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-create-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-import-fields.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-import-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-import-guide.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-app-policy-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-cli-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-connection-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-connection-settings-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-pat-form.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-permissions-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-repo-scope-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/github-status.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/multi-pr-ci-popover.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/github/my-github/presets-sidebar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/github/pr-ci-automation-rows.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/github/pr-merge-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/pr-reviews-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/action-presets-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/mr-detail-panel.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/mr-discussions-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/mr-reviewer-control.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/my-gitlab/list-toolbar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/my-gitlab/presets-sidebar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/gitlab/subscription-toggle.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/watch-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/gitlab/watch-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/change-request-ci-anatomy.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/integrations/change-request-detail-comments.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/integrations/change-request-detail-copy-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/change-request-detail-header.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/change-request-detail.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/integration-change-request-status-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/integrations/integration-change-request-status-mobile.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/integrations/integration-change-request-status-trigger.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/integrations/integration-list-toolbar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/integration-save-query-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/integration-start-task-menu.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/saved-query-default-button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/validated-popover.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/jira/jira-action-bar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/jira/jira-oauth-fields.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/jira/my-jira/list-toolbar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/jira/task-presets-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/kanban-card-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/kanban/columns-menu.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/kanban/graph2-task-pipeline.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/kanban/kanban-header.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/kanban/mobile-column-tabs.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/kanban/mobile-menu-task-list-options.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/kanban/mobile-menu-utility-actions.tsx` — touch-only; preserved with its existing touch composition.
- [x] `apps/web/components/linear/linear-settings.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/model-config-selector-content.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/navigation/app-nav-sections.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/navigation/app-nav-trigger.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/navigation/destination-rows.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/quick-chat/quick-chat-dialog.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/quick-chat/quick-chat-setup.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/quick-chat/quick-chat-tab-item.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/quick-chat/quick-chat-tab-strip.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/quick-chat/quick-tab-add-menu.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/quick-chat/quick-terminal-tab-item.tsx` — documented exception; specialized chrome retains independent geometry.
- [x] `apps/web/components/repository-discovery-root-controls.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/review/review-dialog-pr-state.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/review/review-diff-header.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/review/review-diff-toolbar.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/review/walkthrough-overlay.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/runs/automation-detail-page.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/runs/run-filters.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/sentry/sentry-instance-card.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/sentry/sentry-issue-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/sentry/sentry-issue-watch-multiselect.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/session/prepare-progress.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/shared/resizable-markdown-table.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/system-health/health-indicator.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task-autopilot-toggle.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task-edit-dialog-dependencies.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/task-preview-panel.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/theme-toggle.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/threads/thread-session-switcher.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/threads/threads-view-controls.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/threads/threads-view-editor-actions.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/threads/threads-view-editor-sections.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/threads/threads-view-task-picker.tsx` — content-sized; preserved because the surface is not an ordinary control.
- [x] `apps/web/components/ui/data-table-pagination.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/watches/watcher-delete-action.tsx` — already conforming; retained after source and responsive-geometry review.
- [x] `apps/web/components/workflow-selector-row.tsx` — changed in implementation; verified by source review and focused checks.

## Additional implementation files

Follow-up helper, size-prop, and padding-only searches identified these implementation files outside the original AST rows. Each changed row is included here so the final sweep remains auditable.

- [x] `apps/packages/ui/src/button.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/packages/ui/src/control-sizing.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/packages/ui/src/input-group.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/packages/ui/src/input.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/packages/ui/src/select.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/agents/agents-page-client.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/components/new-task-bottom-bar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/components/new-task-participant-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/components/new-task-selector-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/components/new-task-stages.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/components/routing/route-panel.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/office/setup/step-tier-profiles.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/workspace/workspace-repositories-client.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/workspace/workspace-repository-set-editor.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/workspace/workspace-repository-sets-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/settings/workspace/workspaces-page-client.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/app/tasks/columns.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/my-github/issue-list.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/my-github/list-toolbar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/github/my-github/save-preset-dialog.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/auth-error-message.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/integrations/integration-cursor-pagination.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/jira/my-jira/ticket-row.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/settings-control.ts` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/settings-typography.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/system/feature-toggle-card.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-section-actions.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workflow-sync-section.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/settings/workspaces/workspace-settings-shell.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/remediation-link.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/sidebar-filter/filter-multi-select.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/sidebar-filter/group-picker.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/simple/task-documents.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/task/task-top-bar.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/components/workspaces/workspace-picker-content.tsx` — changed in implementation; verified by source review and focused checks.
- [x] `apps/web/lib/utils/selector-options.ts` — changed in implementation; verified by source review and focused checks.

## Completion evidence

Implementation completed on 2026-09-10.

- Shared primitives now own the 28px standard, 24px compact, and responsive coarse-pointer touch classes.
- Ordinary task, settings, integration, Office, navigation, and shared-wrapper controls were migrated; retained rows identify content-sized, touch-only, or documented specialized geometry.
- Red/green regressions covered shared controls, completed-session actions, task creation, Layouts, repository secrets, workflow navigation, settings typography, and representative application controls.
- Final focused Vitest and managed E2E results are recorded in the work orders and plan.
- The final source sweep found no unexplained ordinary control with an unconditional desktop 44px height; remaining explicit heights are classified above or are embedded/editor/layout geometry.
