---
id: "08-native-repository-provider"
title: "Native repository provider in task creation"
status: completed
wave: 2
depends_on: ["01-design-package", "04-frontend-plugin-registry"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 08: Native repository provider in task creation

## Intent

Move native remote repository selection, branch listing, and pull-request inspection
through repository-provider registry adapters without new host Bitbucket parsing.

## Owned paths

- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories.test.tsx`
- `apps/web/hooks/domains/github/use-branches-by-url.ts`
- `apps/web/hooks/domains/github/use-branches-by-url.test.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-state.test.ts`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-helpers.test.ts`
- `apps/web/components/task-create-dialog-remote-repo-provider-tabs.tsx`
- `apps/web/components/task-create-dialog-remote-repo-provider-tabs.test.tsx`
- `apps/web/components/task-create-dialog-remote-repo-identity.ts`
- `apps/web/components/task-create-dialog-remote-repo-identity.test.ts`

## Dependencies

Tasks 01 and 04.

## Acceptance

1. Registry adapters drive built-in and plugin repository discovery, branch selection,
   and URL inspection; unknown self-managed URLs no longer default to GitLab.
2. Provider-neutral state uses `remoteUrl` while preserving `githubUrl` caller
   compatibility during migration.
3. Plugin-inspected URLs produce a complete descriptor for backend validation, including
   Cloud/DC custom host and exact clone URL behavior.

## Verification

```sh
cd apps/web && pnpm test -- hooks/domains/integrations/use-remote-repositories.test.tsx hooks/domains/github/use-branches-by-url.test.ts hooks/domains/github/use-pr-info-by-url.test.ts components/task-create-dialog-helpers.test.ts components/task-create-dialog-remote-repo-provider-tabs.test.tsx components/task-create-dialog-remote-repo-identity.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Risks

Provider hint alone is untrusted. Preserve legacy callers while ensuring a plugin
provider descriptor—not a host Bitbucket parser—crosses the backend boundary.
