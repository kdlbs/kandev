# TYPO-011: Align workspace and provider form typography

- Priority: P1
- Status: In progress
- Scope: Workspace tabs, integrations, provider forms, repositories, secrets, and automations
- Related: TYPO-002, TYPO-004, TYPO-005, TYPO-006

## Finding

Workspace settings have shared section chrome, but provider forms and nested workspace controls implement their own label, helper, value, and code typography. This creates a second settings language inside the Integrations and workspace areas.

## Evidence

- `apps/web/components/settings/workspaces/workspace-section-header.tsx:39-43` establishes a shared workspace section heading and description.
- `apps/web/components/github/github-repo-scope-section.tsx:150-192` uses raw `text-sm font-medium` labels for provider scope fields.
- `apps/web/components/github/github-pat-form.tsx:31-44` combines a default `Label`, `text-xs` helper copy, and a `font-mono` input.
- `apps/web/components/gitlab/gitlab-settings.tsx:271-303` uses `text-xs` helper copy and `font-mono text-sm` token fields.
- `apps/web/components/jira/jira-settings.tsx:118-262` and `apps/web/components/linear/linear-settings.tsx:78-113` use default labels with local `text-xs` inline helper spans.
- `apps/web/components/azure-devops/azure-devops-settings.tsx:363-423` uses shared labels for controls but a separate `text-xs leading-relaxed` help surface and `text-sm` error copy.
- `apps/web/app/settings/workspace/workspace-repositories-dialog.tsx:57-75` mixes `text-sm` row copy with `text-xs` path metadata, while `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx:64-111` mixes `font-mono text-xs`, `text-sm`, and `text-xs` metadata.

## User impact

Users moving from a workspace section to a provider form encounter different label and helper scales for equivalent controls. Provider technical values can also dominate or shrink relative to the shared workspace hierarchy.

## Proposed direction

Expose the settings field and helper roles to provider forms. Keep provider-specific status, code, and credentials as semantic technical variants. Migrate one provider end to end first, then apply the same primitives to the remaining providers and workspace dialogs.

## Verification

- Compare Workspace, GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps settings pages.
- Test connected, disconnected, loading, validation-error, and empty states.
- Check provider forms on Pixel 5, including token fields and long help text.

## Progress

- 2026-08-16: Opened from the settings source audit.
