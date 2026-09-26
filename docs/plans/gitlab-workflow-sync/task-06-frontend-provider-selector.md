---
id: "06-frontend-provider-selector"
title: "Workflow sync provider selector"
status: done
wave: 4
depends_on: ["05-wire-gitlab-provider"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-workflow-sync.md"
---

# Task 06: Workflow Sync Provider Selector

## Acceptance

1. The workflow sync dialog lets the user choose GitHub or GitLab, mirroring the
   existing `REMOTE_REPOSITORY_PROVIDERS` tab pattern. The repository URL field,
   placeholder, and labels follow the selection, and the config round-trips
   `provider` and `project_path` through the typed API client.
2. A GitLab project URL parses into `project_path` — including nested subgroups,
   self-managed hosts, and the `/-/tree/<branch>/<path>` form — and rebuilds
   back into a URL for display. GitHub parsing via
   `@/lib/utils/github-repo-url` is unchanged.
3. Changing the provider on an existing config warns that the new source becomes
   authoritative and workflows absent from it will be removed (plan `## Risks`).
   All new copy goes through `t()` / `<Trans>` with no hardcoded literals.

## Verification

```bash
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm vitest run lib/utils lib/api hooks/domains/settings
```
```bash
cd apps && pnpm --filter @kandev/web run i18n:check && pnpm --filter @kandev/web run i18n:ratchet
```

## Files Likely Touched

- `apps/web/lib/types/workflow-sync.ts` — `provider`, `project_path`.
- `apps/web/lib/utils/gitlab-repo-url.ts` *(new)* and its `.test.ts`.
- `apps/web/hooks/domains/settings/use-workflow-sync.ts` — provider state and
  provider-routed parse/build.
- `apps/web/components/settings/workflow-sync-dialog.tsx` — selector,
  provider-aware placeholder and title.
- `apps/web/locales/**` — new keys.

## Inputs

- Spec `## Scenarios` 1, 3, 4, 5.
- `apps/web/hooks/domains/integrations/use-remote-repositories.ts:11-13` —
  `REMOTE_REPOSITORY_PROVIDERS`, the provider-selection pattern to mirror.
- `apps/web/components/task-create-dialog-remote-repo-provider-tabs.tsx` —
  existing provider tabs UI.
- `workflow-sync-dialog.tsx:44,152` — the GitHub-hardcoded placeholder and title
  to make conditional.

## Risks

- The i18n ratchet fails the commit on any hardcoded literal in a changed line;
  a SCREAMING_CASE config table of labels passes lint silently but is still a
  violation — review those by eye (root `CLAUDE.md` § Internationalization).
- The dialog currently assumes GitHub URL shapes throughout the hook; missing a
  call site leaves a GitLab config that saves but cannot be re-opened for edit.

## Output Contract

A user can configure, edit, and force-sync a GitLab workflow-sync config from
settings, with GitHub behavior unchanged and no untranslated copy.
