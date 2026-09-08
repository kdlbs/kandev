---
id: "04-responsive-e2e-docs"
title: "Verify responsive flows and update docs"
status: complete
wave: 4
depends_on:
  - "01-refresh-status-credentials"
  - "02-task-chrome-status-controls"
  - "03-unify-profile-settings"
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 04: Verify responsive flows and update docs

## Acceptance

- Desktop and mobile E2E cover recovered sidebar/Kanban status, compact task
  controls, profile-root navigation, and the one-page admin/member settings
  hierarchy without horizontal overflow.
- Legacy configured-executor routes redirect to a profile while orphan recovery
  remains reachable.
- Public Kubernetes/executor docs describe the final hierarchy and shared
  connection blast radius; every managed E2E process is torn down.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web build:e2e && cd web && pnpm e2e:raw --project=chromium --workers=1 --retries=0 e2e/tests/settings/kubernetes-task-environment.spec.ts e2e/tests/settings/kubernetes-executor.spec.ts && pnpm e2e:raw --project=mobile-chrome --workers=1 --retries=0 e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts e2e/tests/settings/mobile-kubernetes-executor.spec.ts && pnpm run e2e:sleep-ratchet && cd ../.. && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check
```

## Files likely touched

- `apps/web/e2e/helpers/session-capabilities.ts` or a focused Kubernetes status rewrite helper
- `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/kubernetes-executor.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`
- `docs/public/k8s.md`
- `docs/public/executors.md`
- `docs/plans/kubernetes-executor-status-settings-repair/plan.md`
- all task files in this plan for final result synchronization

## Dependencies

Tasks 01, 02, and 03.

## Parallelism

`sequential`. It verifies the integrated backend/frontend contract and updates
shared E2E artifacts already modified by the first UX repair.

## Inputs

- Spec: all new responsive/status/settings scenarios.
- Plan: E2E Tests and Public Documentation sections.
- Existing patterns: managed Playwright fixture, `routeWebSocket`,
  `assertNoDocumentHorizontalOverflow`, and current Kubernetes E2E seed helpers.

## Output contract

Report exact discovery/pass counts, artifact paths, process/fixture teardown,
docs validation, files changed, blockers/risks, and mark all tasks plus
`plan.md` complete only after every command passes.

## Results

Implemented the integrated responsive coverage and public documentation for the
repaired status and settings hierarchy. Desktop coverage proves that Kanban and
sidebar Kubernetes status icons surface an exact unauthorized Pod read, recover
to the exact healthy Pod after credentials become usable, and use the healthy
tone instead of retaining the stale error. The task disclosure keeps the Pod
summary while exposing only compact Refresh and Profile settings controls; its
Refresh action joins the active status request and its gear links to the exact
profile root.

Desktop and mobile settings coverage now treats the profile root as the single
configured Kubernetes hierarchy: shared connection fields, diagnostics, active
sessions, workload, workspace, credentials, scripts, and policy all remain on
one page. It verifies unsaved connection plus workload diagnostics, direct
post-create profile navigation, configured legacy-route redirects, member
read-only parity, touch targets, contained YAML, and zero-profile orphan
recovery.

Documentation in `docs/public/k8s.md` and `docs/public/executors.md` describes
the same hierarchy, the shared connection blast radius, compact task controls,
and the exceptional orphan recovery route.

Final verification:

- Focused frontend suite: 14 files, 97 tests passed.
- Desktop managed Playwright: 4 discovered, 4 passed with one worker and zero
  retries (24.5s).
- Mobile Chrome managed Playwright: 4 discovered, 4 passed with one worker and
  zero retries (19.8s).
- `pnpm run typecheck`: passed.
- Focused ESLint and Prettier checks for the final route/E2E corrections:
  passed.
- `pnpm run e2e:sleep-ratchet`: 16 added and 9 modified E2E files clean.
- Public docs: 61 validator tests passed and 41 published pages validated.
- `git diff --check`: clean.

The managed runner removed every process and temporary directory created by
these runs. The only workspace backend left running is the intentionally
preserved isolated seed on port 48646; the main Kandev instance on port 9998 was
not touched. Older `/tmp/kandev-e2e-*` directories from August 21-24 were
outside these runs and were preserved.
