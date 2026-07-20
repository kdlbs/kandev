---
id: "04-manual-editor-migration"
title: "Existing manual editor migration"
status: done
wave: 2
depends_on: ["01-save-coordinator"]
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 04: Existing Manual Editor Migration

## Acceptance

- Existing route-level Save controls register with the shared action and duplicate header/card/footer Save buttons are removed.
- Multi-card repository and nested profile forms keep independent dirty/error state while one Save covers all route contributors.
- SSH login-shell selection joins the executor-profile draft and no longer invokes profile persistence before the shared Save.
- New-resource routes and dialog/sheet Create, Save, and destructive actions retain their explicit local actions; only automation edit mode registers with the floating action.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings app/settings/agents app/settings/executors app/settings/workspace
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/settings/settings-page-template.tsx`
- `apps/web/components/settings/unsaved-indicator.tsx`
- `apps/web/components/settings/editors-settings.tsx`
- `apps/web/components/settings/notifications-settings.tsx`
- `apps/web/components/settings/prompts-settings.tsx`
- `apps/web/components/settings/secrets-settings.tsx`
- `apps/web/app/settings/workspace/workspace-edit-client.tsx`
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/components/settings/repository-card.tsx`
- `apps/web/app/settings/agents/[agentId]/page.tsx`
- `apps/web/app/settings/agents/[agentId]/agent-setup-parts.tsx`
- `apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx`
- `apps/web/app/settings/executor/[id]/page.tsx`
- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/components/settings/ssh-agent-readiness-card.tsx`
- `apps/web/app/settings/executor/[id]/profile/[profileId]/page.tsx`
- `apps/web/components/automations/automation-editor.tsx`

## Dependencies

Task 01.

## Inputs

- Spec: settings-wide coverage and overlay exceptions.
- Existing `SettingsPageTemplate`, `UnsavedSaveButton`, repository dirty helpers, and profile save helpers.

## Output Contract

Report migrated routes and intentional overlay exceptions, tests run, selector changes for E2E, files touched, blockers, and update task/plan status.

## Result

- Migrated workspace, repository, agent/profile, executor/profile, MCP policy, editor, notification, and automation edit forms to route contributors.
- Folded SSH login-shell and automation trigger edits into their owning drafts; trigger writes now begin only after Save and partial retries do not recreate completed triggers.
- Retained explicit Create actions for new agents, profiles, executors, and automations, plus overlay-local and destructive commands.
- Verified with 113 focused settings and automation tests, web typecheck, and scoped ESLint.
