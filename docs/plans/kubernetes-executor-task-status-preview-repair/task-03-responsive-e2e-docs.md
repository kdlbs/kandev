---
id: "03-responsive-e2e-docs"
title: "Verify responsive flows and update docs"
status: complete
wave: 3
depends_on: ["02-polish-responsive-preview"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 03: Verify responsive flows and update docs

## Acceptance

- Managed desktop Playwright proves a Kanban/sidebar Kubernetes glyph reaches
  its authoritative tone before hover, then opens the structured exact-Pod
  summary and refreshes causally.
- Managed mobile Chrome proves tapping the compact status glyph opens the same
  facts in a Drawer without selecting the task, horizontal overflow, unsafe
  fixed geometry, or a sub-44 px effective target.
- Public executor/Kubernetes docs describe eager task-chrome status and the
  fine-pointer summary/coarse-pointer Drawer without changing navigation.
- Focused frontend, i18n, E2E safety, docs validation, and diff gates pass; all
  managed processes and temporary directories are removed without touching the
  main or isolated seed instances.

## Verification

```bash
cd apps/web && pnpm exec playwright test e2e/tests/settings/kubernetes-task-environment.spec.ts --project=chromium --workers=1 --retries=0 && pnpm exec playwright test e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts --project=mobile-chrome --workers=1 --retries=0 && pnpm run e2e:sleep-ratchet && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet
cd ../../ && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/kubernetes-task-environment-helpers.ts`
- affected page objects/selectors only when existing semantic locators are insufficient
- `docs/public/k8s.md`
- `docs/public/executors.md`
- this task file and `plan.md`

## Dependencies

Task 02.

## Parallelism

`sequential`. It validates the final resource and responsive presentation
contracts on settled selectors.

## Inputs

- Spec: eager Kanban, structured fine-pointer summary, and coarse-pointer Drawer
  scenarios.
- Plan: E2E Tests and Public Documentation.
- Existing patterns: managed Kubernetes task-environment specs, exact route
  interception, `KanbanPage.taskCard`, `SessionPage.sidebar`, E2E sleep ratchet,
  and public-doc validation scripts.

## Output contract

Record exact discovered/passed counts, first-attempt/no-retry results, process
and temporary-directory teardown evidence, docs validation, files changed,
blockers/risks, and synchronize this task plus `plan.md` to complete only after
all acceptance gates are green.

## Results

- Managed desktop Chromium discovered 1 test and passed 1/1 first attempt in
  8.2 seconds with one worker and `--retries=0`. It proves Kanban and sidebar
  tones hydrate before hover and the structured exact-Pod summary opens.
- Managed mobile Chrome discovered 1 test and passed 1/1 first attempt in 8.4
  seconds with one worker and `--retries=0` against the same fresh artifacts.
  It proves eager status, the effective 44 px trigger, structured Drawer facts,
  and that opening and closing the status disclosure does not select its task
  row.
- `docs/public/k8s.md` and `docs/public/executors.md` now document eager task
  status and the fine-pointer/touch presentations. Public-doc validation passed
  61 tests and validated 41 pages.
- The E2E sleep ratchet, frontend typecheck/lint, i18n checks, focused Prettier,
  and `git diff --check` pass. Managed E2E teardown left no E2E-owned runner,
  backend, frontend, Playwright, or temporary fixture process. The pre-existing
  isolated seed backend/frontend remained running and untouched; the main
  instance was also untouched. No blocker remains.
