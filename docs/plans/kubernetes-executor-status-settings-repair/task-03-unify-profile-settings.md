---
id: "03-unify-profile-settings"
title: "Unify Kubernetes profile settings"
status: complete
wave: 3
depends_on: ["02-task-chrome-status-controls"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 03: Unify Kubernetes profile settings

## Acceptance

- A Kubernetes profile root contains the shared connection editor,
  diagnostics, active sessions, and profile fields in one hierarchy; members
  see the same hierarchy read-only.
- Connection plus workload edits participate in one save/reset flow with one
  active-session confirmation and accurate partial-failure dirty state.
- Configured executor tree/legacy links resolve to a profile root; only a
  zero-profile orphan retains the standalone recovery editor.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/settings/kubernetes-profile-cluster-section.test.tsx components/settings/kubernetes-save-confirmation.test.ts components/settings/profile-edit/use-executor-profile-save-contributor.test.tsx components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/settings/legacy-executor-settings-route.test.tsx src/settings-routes.test.tsx && pnpm --filter @kandev/web lint && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet && cd ../.. && git diff --check
```

## Files likely touched

- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/app/settings/executors/k8s/[executorId]/page.tsx`
- `apps/web/components/settings/kubernetes-profile-cluster-section.tsx`
- `apps/web/components/settings/kubernetes-profile-cluster-section.test.tsx`
- `apps/web/components/settings/kubernetes-save-confirmation.ts`
- `apps/web/components/settings/kubernetes-save-confirmation.test.ts`
- `apps/web/components/settings/profile-edit/use-executor-profile-save-contributor.ts`
- a focused combined Kubernetes page-save hook/test if extraction is needed
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`
- `apps/web/components/settings/legacy-executor-settings-route.tsx`
- `apps/web/components/settings/legacy-executor-settings-route.test.tsx`
- `apps/web/lib/settings/executor-settings-routes.ts`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.tsx`
- affected locale catalogs under `apps/web/src/locales/`

## Dependencies

Task 02.

## Parallelism

`sequential`. It changes the same route helper and locale/settings contracts as
Task 02.

## Inputs

- Spec: one-page hierarchy, shared-resource copy, combined save/partial failure,
  member permissions, and legacy/orphan routing scenarios.
- Plan: One profile hierarchy and Navigation sections.
- Existing patterns: `useKubernetesExecutorResource`,
  `KubernetesConnectionCard`, `useSettingsSaveContributor`,
  `useKubernetesSessionImpact`, and `saveWithKubernetesSessionConfirmation`.

## Output contract

Report RED/GREEN evidence, save ordering and failure semantics, mobile/read-only
behavior, files changed, blockers/risks, and synchronize this task plus
`plan.md`.

## Results

Implemented one Kubernetes profile hierarchy with the shared connection editor,
diagnostics, active sessions, workload, workspace, credentials, environment,
scripts, and MCP policy. Administrators save connection and profile drafts
through one contributor and one active-session confirmation. Connection saves
run first and mark their baseline immediately, so a later profile failure leaves
only the profile dirty. Members see the same hierarchy read-only.

Configured Kubernetes executor rows are disclosure-only and point to profiles.
Legacy and standalone configured-executor URLs redirect to the first profile;
only an executor with zero profiles keeps the standalone recovery editor.

RED evidence:

- The cluster section rendered a summary/link instead of inline connection
  fields, combined edits fell through to workload-only confirmation, and no
  coordinated page contributor existed.
- Configured executor tree and legacy/standalone routes still navigated to the
  separate connection page.

GREEN evidence:

- Focused settings/routing suite: 8 files, 88 tests passed.
- `pnpm --filter @kandev/web lint`: 0 warnings/errors.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: all catalogs complete; pseudo locale synchronized.
- `pnpm run i18n:ratchet`: 21 added and 36 modified files clean; allowlist
  intact.
- `git diff --check`: clean.
