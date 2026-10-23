# Button Color Audit

## Scope and limits

Source audit on 2026-10-22, existing `feature/improve-markdown-pre-a36` branch.
Named Button imports from `@kandev/ui/button` or `@kandev/ui` were resolved with
TypeScript AST parsing; tests, E2E, dependencies, and generated output excluded.
Found 286 omitted/static-default sites, including 206 explicit disabled props.
Also found 62 dynamic variants and three spread-bearing default tags. Those
counts are source candidates, not counts of confirmed rendered defects.
The full explicit-disabled inventory is below. Dynamic props, wrappers,
forwarded children, and runtime classes require interpretation during repair.

## Confirmed rendered root cause

Real Markdown Save in Firefox desktop and WebKit phone uses opacity 0.45.
Its label/icon foreground is #EDF1FF; enabled primary background is #4F46E5 in
light and #6468F0 in dark. Whole-control compositing dims both. Desktop shortcuts
add independent #737373 light / #888888 dark foreground. This also weakens the
shortcut when the button is enabled. Phone has no shortcut child.

Read-only diagnostics captured light/dark real buttons and synthetic DOM-only
state comparisons. The neutral proposal uses opacity 1, #F5F5F5 / #2A2A2A
background, and #737373 / #888888 inherited foreground. Its opaque text contrast
is 4.35:1 light and 4.05:1 dark. These measurements describe the prototype, not
an implemented production change or an accessibility conformance verdict.

Evidence: `/tmp/kandev-pr3113-evaluation-4e7mws9y/save-style-diagnostic.json`.
Images in that directory include `save-style-firefox-desktop-dark-synthetic.png`
and real `save-style-firefox-desktop-dark-button.png` / phone equivalents.
The synthetic comparisons have no application handlers. Canvas was verified
through source and a synthetic style reproduction, not its actual live entry.
All 66 fixture hashes were unchanged. Browsers closed. No persistent theme,
database, runtime, source, or main-instance change was made.

## Implemented repair and rendered coverage

The shared default Button now has opaque neutral disabled colors, including
explicit dark disabled tokens. A rendered PR regression caught the equal
specificity between `disabled:bg-muted` and `dark:bg-green-600`; the later dark
rule won. The combined dark-disabled variant resolves that cascade without
changing the enabled feature palette. Granted notification and completed Saved
status supply matching ordinary and dark disabled success tokens.

Real rendered coverage checks desktop/phone Markdown clean, dirty, and pending
Save; notification unsupported/granted states; settings enabled/pending/Saved;
Canvas sidebar setup; and GitHub Approve/Merge pending states. Both themes are
included. GitLab approval, plan Send/Start, and every other inventory entry are
source-audited shared consumers, not individually exercised live. Their action
predicates and enabled classes are unchanged. The work order records browser
results and native-device limits.

## Exact local foreground repairs

- `apps/web/components/task/markdown-file-toolbar.tsx:88`: shortcut.
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx:66`: shortcut.
- `apps/web/components/editors/codemirror/codemirror-editor-toolbar.tsx:182`: shortcut.
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx:110`: spinner
  currently forces primary foreground; inherit neutral disabled foreground too.
- `apps/web/components/canvas/canvas-task-create-launcher.tsx:103`: sidebar
  presentation adds muted text but leaves default primary fill. Use ghost in
  sidebar and retain default in settings.

## Custom-color decisions

All seven controls retain enabled feature colors. Unavailable actions become
neutral. Already-granted notification permission and completed settings Save retain
success colors. Pending/invalid settings Save becomes neutral.

| Source                                                                | Existing treatment         | Proposed disabled treatment                             |
| --------------------------------------------------------------------- | -------------------------- | ------------------------------------------------------- |
| `apps/web/components/github/pr-detail-panel.tsx:233`                  | Green approval             | Neutral                                                 |
| `apps/web/components/github/pr-merge-button.tsx:215`                  | Green merge, dark override | Neutral; verify cascade                                 |
| `apps/web/components/gitlab/mr-detail-panel.tsx:212`                  | Green approval             | Neutral                                                 |
| `apps/web/components/settings/notification-permission-section.tsx:53` | Green granted status       | Success tokens when granted; neutral when unsupported   |
| `apps/web/components/settings/settings-floating-save.tsx:82`          | Success Save / Saved       | Neutral when pending/invalid; success tokens when Saved |
| `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx:97`  | Violet plan-mode send      | Neutral; spinner inherits                               |
| `apps/web/components/task-create-dialog-footer.tsx:184`               | Blue plan-mode start       | Neutral                                                 |

Four custom-color default sites currently lack explicit disabled props:
`apps/web/app/office/inbox/inbox-item-row.tsx:125`,
`apps/web/components/task/mobile/session-mobile-top-bar.tsx:227`, and
`apps/web/components/task/task-center-panel.tsx:505,516`. Their enabled colors
remain unchanged. Raw GitHub/GitLab merge buttons and raw walkthrough controls
use independent styling; they are not evidence of this default-Button defect.

The two sidebar spread tags resolve variants from shared `buttonProps`.
`StorageActionButton` forwards variant/color props and computes disabled state;
its default consumers inherit the correction. Do not count spread tags as
confirmed default controls solely from their opening tags.

## Explicit-disabled default consumer inventory

Line numbers are the audit snapshot, before repair. The disabled expression
may evaluate false at runtime. Related examples include desktop/phone editor
Save, task-description Save, task creation, SSH, integration query, profile,
plugin installation, and form submission controls.

- `apps/web/app/auth/invite-page.tsx:130`
- `apps/web/app/auth/login-page.tsx:116`
- `apps/web/app/auth/setup-wizard.tsx:86`
- `apps/web/app/coordinator/components/send-back-form.tsx:196`
- `apps/web/app/coordinator/proposal-card/edit-form.tsx:444`
- `apps/web/app/coordinator/proposal-card/message-edit-form.tsx:69`
- `apps/web/app/coordinator/proposal-card/proposal-card.tsx:268`
- `apps/web/app/coordinator/proposal-card/reject-form.tsx:77`
- `apps/web/app/office/agents/[id]/components/agent-channels-tab.tsx:237`
- `apps/web/app/office/agents/[id]/components/agent-configuration-tab.tsx:175`
- `apps/web/app/office/agents/[id]/components/agent-permissions-tab.tsx:88`
- `apps/web/app/office/agents/[id]/components/agent-routing-card.tsx:114`
- `apps/web/app/office/agents/[id]/components/agent-skills-tab.tsx:222`
- `apps/web/app/office/agents/components/create-agent-dialog.tsx:296`
- `apps/web/app/office/components/new-task-dialog.tsx:183`
- `apps/web/app/office/components/workspace-pause-controls.tsx:190`
- `apps/web/app/office/inbox/inbox-item-row.tsx:215`
- `apps/web/app/office/projects/[id]/project-header.tsx:92`
- `apps/web/app/office/projects/create-project-dialog.tsx:353`
- `apps/web/app/office/routines/[id]/routine-detail-view.tsx:203`
- `apps/web/app/office/routines/create-routine-dialog.tsx:468`
- `apps/web/app/office/routines/create-routine-dialog.tsx:476`
- `apps/web/app/office/setup/step-import.tsx:58`
- `apps/web/app/office/setup/wizard-footer.tsx:48`
- `apps/web/app/office/setup/wizard-footer.tsx:53`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx:45`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx:280`
- `apps/web/app/office/workspace/costs/create-budget-form.tsx:177`
- `apps/web/app/office/workspace/costs/default-ceiling-card.tsx:45`
- `apps/web/app/office/workspace/routing/page.tsx:222`
- `apps/web/app/office/workspace/settings/components/git-section.tsx:207`
- `apps/web/app/office/workspace/settings/components/settings-content.tsx:190`
- `apps/web/app/office/workspace/settings/components/settings-content.tsx:238`
- `apps/web/app/office/workspace/settings/export/export-preview.tsx:35`
- `apps/web/app/office/workspace/settings/sync/sync-diff-pane.tsx:48`
- `apps/web/app/office/workspace/skills/create-skill-form.tsx:130`
- `apps/web/app/settings/executor/new/page.tsx:308`
- `apps/web/app/settings/workspace/workflow-import-profile-selection.tsx:391`
- `apps/web/app/settings/workspace/workspace-repositories-dialog.tsx:207`
- `apps/web/app/settings/workspace/workspace-repository-set-editor.tsx:59`
- `apps/web/app/settings/workspace/workspace-repository-set-editor.tsx:91`
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx:124`
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx:271`
- `apps/web/app/settings/workspace/workspaces-page-client.tsx:72`
- `apps/web/components/agent/cli-profile-editor.tsx:277`
- `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.tsx:151`
- `apps/web/components/automations/managed-automation-import-dialog.tsx:71`
- `apps/web/components/automations/trigger-configs/plugin-webhook-controls.tsx:25`
- `apps/web/components/azure-devops/azure-devops-filters.tsx:276`
- `apps/web/components/azure-devops/azure-devops-save-view-dialog.tsx:85`
- `apps/web/components/azure-devops/azure-devops-settings.tsx:468`
- `apps/web/components/azure-devops/azure-devops-watch-settings.tsx:525`
- `apps/web/components/azure-devops/azure-devops-work-item-detail.tsx:107`
- `apps/web/components/azure-devops/azure-devops-work-item-detail.tsx:189`
- `apps/web/components/config-chat/config-chat-setup.tsx:135`
- `apps/web/components/config-chat/config-chat-setup.tsx:207`
- `apps/web/components/coordinators/add-standing-order-dialog.tsx:106`
- `apps/web/components/coordinators/coordinator-add-page.tsx:95`
- `apps/web/components/coordinators/sections/goal-section.tsx:51`
- `apps/web/components/coordinators/sections/goal-section.tsx:108`
- `apps/web/components/coordinators/setup/coordinator-setup.tsx:126`
- `apps/web/components/coordinators/setup/coordinator-setup.tsx:136`
- `apps/web/components/create-local-repository-surface.tsx:212`
- `apps/web/components/diff/comment-form.tsx:73`
- `apps/web/components/diff/walkthrough-step-card.tsx:130`
- `apps/web/components/editors/codemirror/codemirror-editor-toolbar.tsx:166`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx:50`
- `apps/web/components/folder-picker.tsx:517`
- `apps/web/components/github/github-app-connection-panel.tsx:91`
- `apps/web/components/github/github-app-create-form.tsx:200`
- `apps/web/components/github/github-app-import-form.tsx:158`
- `apps/web/components/github/github-app-import-form.tsx:195`
- `apps/web/components/github/github-connection-settings-form.tsx:283`
- `apps/web/components/github/github-status.tsx:367`
- `apps/web/components/github/issue-watch-dialog.tsx:508`
- `apps/web/components/github/my-github/save-preset-dialog.tsx:120`
- `apps/web/components/github/pr-ci-automation-prompt-dialog.tsx:113`
- `apps/web/components/github/pr-detail-panel.tsx:233`
- `apps/web/components/github/pr-merge-button.tsx:215`
- `apps/web/components/github/review-watch-dialog.tsx:639`
- `apps/web/components/gitlab/mr-auto-fix-prompt-dialog.tsx:113`
- `apps/web/components/gitlab/mr-detail-panel.tsx:212`
- `apps/web/components/gitlab/mr-discussions-section.tsx:126`
- `apps/web/components/gitlab/my-gitlab/save-preset-dialog.tsx:100`
- `apps/web/components/gitlab/task-mr-link-dialog.tsx:200`
- `apps/web/components/gitlab/watch-dialog.tsx:446`
- `apps/web/components/improve-kandev-dialog.tsx:414`
- `apps/web/components/integrations/integration-copy-config-menu.tsx:106`
- `apps/web/components/integrations/integration-save-query-dialog.tsx:159`
- `apps/web/components/integrations/task-change-request-link-form.tsx:40`
- `apps/web/components/integrations/validated-popover.tsx:178`
- `apps/web/components/jira/jira-issue-watch-dialog.tsx:592`
- `apps/web/components/jira/jira-oauth-fields.tsx:49`
- `apps/web/components/jira/jira-oauth-fields.tsx:95`
- `apps/web/components/jira/my-jira/jql-editor.tsx:50`
- `apps/web/components/jira/my-jira/list-toolbar.tsx:569`
- `apps/web/components/linear/linear-issue-watch-dialog.tsx:314`
- `apps/web/components/onboarding-dialog.tsx:252`
- `apps/web/components/onboarding-dialog.tsx:261`
- `apps/web/components/plugins/workspace-agent-chat-composer.tsx:80`
- `apps/web/components/plugins/workspace-agent-chat-interactions.tsx:53`
- `apps/web/components/plugins/workspace-agent-chat-interactions.tsx:118`
- `apps/web/components/quick-chat/quick-chat-dialog.tsx:198`
- `apps/web/components/quick-chat/quick-chat-setup.tsx:85`
- `apps/web/components/sentry/sentry-issue-dialog.tsx:531`
- `apps/web/components/sentry/sentry-issue-watch-dialog.tsx:452`
- `apps/web/components/settings/account/api-tokens.tsx:130`
- `apps/web/components/settings/account/security-settings.tsx:127`
- `apps/web/components/settings/add-tui-agent-dialog.tsx:303`
- `apps/web/components/settings/agent-runtime-update-control.tsx:356`
- `apps/web/components/settings/canvas-host-desktop-actions.tsx:147`
- `apps/web/components/settings/canvas-host-desktop-actions.tsx:235`
- `apps/web/components/settings/canvas-lifecycle-dialogs.tsx:331`
- `apps/web/components/settings/canvas-lifecycle-dialogs.tsx:504`
- `apps/web/components/settings/canvas-release-review.tsx:274`
- `apps/web/components/settings/canvas-rename-dialog.tsx:137`
- `apps/web/components/settings/canvas-share-dialog.tsx:291`
- `apps/web/components/settings/canvas-share-dialog.tsx:371`
- `apps/web/components/settings/copy-move-dialog-body.tsx:145`
- `apps/web/components/settings/editor-form.tsx:372`
- `apps/web/components/settings/executor-profile-dialog.tsx:89`
- `apps/web/components/settings/install-agent-card.tsx:51`
- `apps/web/components/settings/installed-agent-card.tsx:229`
- `apps/web/components/settings/notification-permission-section.tsx:53`
- `apps/web/components/settings/plugin-executor-profile-dialog.tsx:140`
- `apps/web/components/settings/plugins/canvas-install-dialog.tsx:216`
- `apps/web/components/settings/plugins/canvas-install-dialog.tsx:225`
- `apps/web/components/settings/plugins/canvas-marketplace-detail.tsx:54`
- `apps/web/components/settings/plugins/install-plugin-dialog.tsx:133`
- `apps/web/components/settings/plugins/marketplace-sources-dialog.tsx:184`
- `apps/web/components/settings/plugins/plugin-capability-approval-editor.tsx:114`
- `apps/web/components/settings/plugins/plugin-row.tsx:581`
- `apps/web/components/settings/profile-edit/docker-sections.tsx:209`
- `apps/web/components/settings/profile-edit/inline-secret-select.tsx:150`
- `apps/web/components/settings/prompts-settings.tsx:574`
- `apps/web/components/settings/repository-branch-policies.tsx:100`
- `apps/web/components/settings/repository-branch-policies.tsx:117`
- `apps/web/components/settings/repository-branch-policies.tsx:221`
- `apps/web/components/settings/repository-branch-policies.tsx:243`
- `apps/web/components/settings/secret-form.tsx:73`
- `apps/web/components/settings/secrets-settings.tsx:439`
- `apps/web/components/settings/settings-floating-save.tsx:82`
- `apps/web/components/settings/ssh-connection-card.tsx:385`
- `apps/web/components/settings/system/backups-table.tsx:300`
- `apps/web/components/settings/system/bundle-customizer.tsx:239`
- `apps/web/components/settings/system/create-user-dialog.tsx:173`
- `apps/web/components/settings/system/feature-toggles-settings.tsx:223`
- `apps/web/components/settings/system/invite-dialog.tsx:93`
- `apps/web/components/settings/system/log-viewer.tsx:282`
- `apps/web/components/settings/system/organizations/create-org-card.tsx:43`
- `apps/web/components/settings/system/organizations/org-admin-dialog.tsx:87`
- `apps/web/components/settings/system/restore-dialog.tsx:150`
- `apps/web/components/settings/system/storage/storage-action-button.tsx:17`
- `apps/web/components/settings/units/unit-members-dialog.tsx:121`
- `apps/web/components/settings/units/units-page.tsx:143`
- `apps/web/components/settings/utility-agent-dialog.tsx:169`
- `apps/web/components/settings/workflow-card-dialogs.tsx:119`
- `apps/web/components/settings/workflow-card-dialogs.tsx:237`
- `apps/web/components/settings/workflow-sync-dialog.tsx:493`
- `apps/web/components/settings/workspaces/workspace-team-access-card.tsx:164`
- `apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx:210`
- `apps/web/components/task/change-workflow-form-sections.tsx:521`
- `apps/web/components/task/changes-panel-dialogs.tsx:159`
- `apps/web/components/task/changes-panel-header-actions.tsx:160`
- `apps/web/components/task/chat/background-work/background-work-panel.tsx:347`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx:97`
- `apps/web/components/task/chat/clarification-overlay-header.tsx:169`
- `apps/web/components/task/chat/dynamic-route-recovery.tsx:163`
- `apps/web/components/task/chat/message-renderer.tsx:130`
- `apps/web/components/task/chat/messages/message-actions.tsx:412`
- `apps/web/components/task/chat/messages/message-comment-surface.tsx:213`
- `apps/web/components/task/chat/queued-ghost-edit-view.tsx:60`
- `apps/web/components/task/editor-comment-popover.tsx:103`
- `apps/web/components/task/inspector/preview-feedback-collection.tsx:143`
- `apps/web/components/task/inspector/preview-feedback-controls.tsx:163`
- `apps/web/components/task/layout-preset-selector.tsx:166`
- `apps/web/components/task/markdown-file-toolbar.tsx:70`
- `apps/web/components/task/mobile/mobile-file-actions.tsx:154`
- `apps/web/components/task/new-session-dialog.tsx:477`
- `apps/web/components/task/new-subtask-form-parts.tsx:517`
- `apps/web/components/task/passthrough-toolbar.tsx:439`
- `apps/web/components/task/plan-selection-popover.tsx:256`
- `apps/web/components/task/plan-selection-popover.tsx:535`
- `apps/web/components/task/share/share-dialog.tsx:163`
- `apps/web/components/task/sidebar-filter/view-manager.tsx:213`
- `apps/web/components/task/simple/components/approval-action-bar.tsx:234`
- `apps/web/components/task/simple/components/task-description-editable.tsx:167`
- `apps/web/components/task/simple/task-chat.tsx:320`
- `apps/web/components/task/simple/task-documents.tsx:155`
- `apps/web/components/task/task-external-link-dialog.tsx:312`
- `apps/web/components/task/task-github-issue-dialog.tsx:151`
- `apps/web/components/task/task-navigation-read-feedback.tsx:31`
- `apps/web/components/task/task-page-content.tsx:201`
- `apps/web/components/task/task-plan-revert-confirm-dialog.tsx:54`
- `apps/web/components/task/task-rename-dialog.tsx:57`
- `apps/web/components/task/workflow-move-options.tsx:218`
- `apps/web/components/task/workflow-stepper.tsx:423`
- `apps/web/components/task-create-dialog-footer.tsx:39`
- `apps/web/components/task-create-dialog-footer.tsx:81`
- `apps/web/components/task-create-dialog-footer.tsx:97`
- `apps/web/components/task-create-dialog-footer.tsx:184`
- `apps/web/components/task-create-dialog-repository-options.tsx:226`
- `apps/web/components/task-create-dialog-repository-sets-save.tsx:122`
- `apps/web/components/threads/threads-view-editor-sections.tsx:363`
- `apps/web/components/vcs/vcs-change-request-dialog.tsx:107`
- `apps/web/components/vcs/vcs-dialogs.tsx:223`
