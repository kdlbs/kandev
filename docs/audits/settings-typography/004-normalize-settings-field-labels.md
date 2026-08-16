# TYPO-004: Normalize settings field labels and label emphasis

- Priority: P1
- Status: In progress
- Scope: Form labels, switch labels, and inline setting labels
- Related: TYPO-001, TYPO-006, TYPO-007, TYPO-011

## Finding

Settings labels use several different implementations: the shared `Label`, raw HTML `label`, labels with muted color, labels with `text-sm`, and labels that inherit the component default. The semantic role is the same, but the visual result changes by page and by feature.

## Evidence

- `apps/packages/ui/src/label.tsx:13` makes the base `Label` `text-xs/relaxed font-medium`.
- `apps/web/components/settings/account/security-settings.tsx:61-73` and `apps/web/components/settings/account/api-tokens.tsx:115` use raw labels with `text-xs text-muted-foreground`.
- `apps/web/components/settings/plugins/plugin-config-form.tsx:67` promotes a label to `text-sm`.
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx:161` uses `text-sm` for a checkbox label, while most profile labels inherit the smaller base role.
- `apps/web/components/settings/system/storage/storage-policy-fields.tsx:53` uses `text-sm` for a setting title and `apps/web/components/settings/system/storage/storage-policy-fields.tsx:87` uses `text-xs text-muted-foreground` for a field label.
- `apps/web/components/settings/profile-form-fields.tsx:138-145`, `apps/web/components/settings/profile-form-fields.tsx:191`, and `apps/web/components/settings/profile-form-fields.tsx:237` switch label treatment between compact and default variants.
- `apps/web/components/settings/profile-edit/inline-secret-select.tsx:47-53` hard-codes a field label as `text-xs text-muted-foreground`, while neighboring executor fields use the default foreground `Label` in `profile-details-card.tsx:29-37`, `mcp-policy-card.tsx:82-91`, and `git-identity-fields.tsx:129-148`.
- Provider and workspace forms mostly use the shared 12px `Label`, while `apps/web/components/github/github-repo-scope-section.tsx:150,174,192` uses raw `text-sm font-medium` labels. Comparable GitHub, GitLab, Jira, Linear, Azure DevOps, and repository fields therefore differ by 2px on mobile-visible forms.
- Workflow fields rely on the shared label at `apps/web/components/settings/workflow-card.tsx:251,261,309` and `workflow-description-field.tsx:27`, while comparable step controls use explicit 14px labels at `workflow-pipeline-editor-panels.tsx:213,269,499` and `workflow-pipeline-editor-step-actions.tsx:318,490-492,529-532`.
- Account token/security forms and system user/invite dialogs use manual `text-xs text-muted-foreground` labels at `apps/web/components/settings/account/api-tokens.tsx:114-117`, `security-settings.tsx:60-75`, `system/create-user-dialog.tsx:51-90`, and `system/invite-dialog.tsx:54-69`. These labels lose the shared foreground, weight, and line-height contract.

## User impact

Some labels read as primary controls, while others read as secondary metadata. Color and size changes are not consistently tied to whether a label names a control, a switch, or a technical value.

## Proposed direction

Introduce a settings field-label role with a documented compact variant. Use it for both `Label` and raw HTML labels, or migrate raw labels to the shared primitive where semantics allow. Define a separate 14px interactive/mobile label variant only for touch-target controls such as step actions, and apply that rule consistently. Keep muted text for helper copy, not for the primary label, unless the setting is intentionally presented as metadata. Migrate inline secret selection and similar fields away from muted helper styling.

## Verification

- Compare labels beside Input, Select, Switch, Checkbox, and Textarea controls.
- Check disabled, error, and long-label states.
- Check all account, workspace, agent, executor, provider, and system forms at the mobile breakpoint.
- Compare provider labels, repository scope labels, workflow fields, and workflow step actions at phone and desktop widths.
- Compare account and system-dialog labels against their shared `Label` counterparts, keeping token URLs and invite values mono as technical values rather than labels.

## Progress

- 2026-08-16: Opened from the settings source audit.
