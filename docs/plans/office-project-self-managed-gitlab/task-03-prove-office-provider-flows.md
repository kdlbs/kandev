---
id: "03-prove-office-provider-flows"
title: "Prove provider-neutral Office flows"
status: done
wave: 3
depends_on:
  - "02-resolve-project-providers"
plan: "plan.md"
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
  - REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-TASKS-PROJECT-REPOSITORIES-001.1
  - AC-TASKS-PROJECT-REPOSITORIES-001.3
  - AC-TASKS-PROJECT-REPOSITORIES-001.6
  - AC-TASKS-PROJECT-REPOSITORIES-001.9
  - AC-TASKS-PROJECT-REPOSITORIES-001.10
  - AC-TASKS-PROJECT-REPOSITORIES-001.11
  - AC-TASKS-PROJECT-REPOSITORIES-001.12
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.1
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.7
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.8
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.1
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.8
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.9
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
  - ../../specs/plugins/system-design/repository-url-discovery.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 03: Prove provider-neutral Office flows

## Summary

Prove the existing Office editor-to-task flow for configured GitLab and an installed repository plugin on desktop and phone.
Retain explicit-plugin task creation and document the supported capability and connection requirements.

## In scope

- Add the two success scenarios and provider-disabled refusal described in the plan's E2E matrix.
- Save URLs through the actual project custom-entry picker and verify persistence after reload.
- Configure GitLab in the Office workspace and install the real packaged repository fixture for plugin discovery.
- Submit unassigned tasks without provider hints or browser descriptors. Verify exact returned and persisted repository identity.
- Keep every existing explicit-plugin desktop/phone scenario and run those suites with the Office regressions.
- Clean disposable connections, tasks, repositories, and plugin state after assertion failures as well as success.
- Update the Office how-to and plugin-authoring reference during implementation.

## Out of scope

- Frontend production, locale, layout, navigation, or control changes.
- Live vendor deployments, new clone protocols, external plugin releases, and broad E2E runs.

## Acceptance

1. Desktop and phone save a bare plugin project URL, reload it, and create a task with the authoritative provider identity and exact clone URL.
2. GitLab still succeeds in its own Office workspace. Disabled or mismatched providers show the existing error without a new task or repository.
3. Existing explicit-plugin suites pass. Documentation states active capability and connection requirements without promising live vendor validation.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.
The managed runners build the current backend and Vite assets.
Run desktop and phone commands sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run app/office/projects/repo-entry.test.ts)
(cd apps/web && pnpm exec prettier --check e2e/tests/office/new-task-dialog.spec.ts e2e/tests/office/mobile-new-task-dialog.spec.ts)
(cd apps/web && pnpm exec eslint --config eslint.e2e-sleeps.config.mjs e2e/tests/office/new-task-dialog.spec.ts e2e/tests/office/mobile-new-task-dialog.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/office/new-task-dialog.spec.ts tests/task/plugin-repository-task-create.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-dialog.spec.ts tests/task/mobile-plugin-repository-task-create.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/office-project-self-managed-gitlab
```

Record discovered test counts, actual results, and any execution blocker.
Run the exact [documentation coverage preflight](plan.md#documentation-coverage-preflight) with all three work orders before final handoff.
Promote draft paired specifications only after the complete package passes and the designs match implementation.

## Files likely touched

- `apps/web/e2e/tests/office/new-task-dialog.spec.ts`.
- `apps/web/e2e/tests/office/mobile-new-task-dialog.spec.ts`.
- `apps/web/e2e/tests/office/project-provider-source-helpers.ts` (new, shared scenario helpers).
- `docs/public/office-tasks.md` (how-to).
- `docs/public/plugins-authoring.md` (reference).
- This work order, `plan.md`, and draft paired specifications for final results and lifecycle.

## Dependencies

Task 02 must pass before final E2E verification. Its service regressions supply the behavioral RED gate.

## Risks

- Desktop provides `dialogSeed`, while phone provides `officeSeed`. Configure the correct Office workspace rather than the base seed workspace.
- Plugin installation is shared by the worker. Explicitly restore plugin state and clean the actual Office repository rows.
- API-only assertions cannot prove the project picker works.
- Tests must not start an external clone or replace backend origin verification with browser interception.

## Parallelism

`sequential`

## Inputs

- [Project requirement](../../specs/tasks/requirements/project-repository-context.md) and [surface design](../../specs/tasks/system-design/project-repository-context.md#surface-and-documentation).
- [Discovery requirement](../../specs/plugins/requirements/repository-url-discovery.md) and [design](../../specs/plugins/system-design/repository-url-discovery.md).
- [Explicit selection requirement](../../specs/plugins/requirements/repository-provider-task-creation.md) and [design](../../specs/plugins/system-design/repository-provider-task-creation.md).
- Task 02 results and the plan's E2E matrix.
- `installFixturePlugin`, `apiClient.configureGitLab`, `officeApi.createProject`, `waitForHttp`, and the existing Office/plugin fixture cleanup patterns.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, `/simple-english`, and web guidance.

## Results

Completed. Desktop and phone Office suites now save a self-managed GitLab URL and an installed-plugin URL through the real project custom-entry picker, reload the project, and create an unassigned task through the New Task form. Every success asserts the returned and persisted task attachment. Identity assertions are per provider: the GitLab cases assert provider, host, owner, name, and clone URL; the desktop plugin case also asserts the immutable repository ID and default branch, and the phone plugin case asserts provider, immutable repository ID, and clone URL.

Refusal coverage differs by viewport. Desktop submits a mismatched GitLab origin and a disabled plugin; phone submits a disabled plugin. Each refusal asserts the existing visible error, the dialog staying open, and no created task. The disabled-plugin cases on both viewports also assert that the submission added no plugin repository row, and the phone case compares the exact repository IDs against a pre-submission baseline. The desktop mismatched-origin case does not inspect repository rows. The existing explicit-plugin desktop and phone suites still pass.

Implemented files:

- `apps/web/e2e/tests/office/new-task-dialog.spec.ts`
- `apps/web/e2e/tests/office/mobile-new-task-dialog.spec.ts`
- `apps/web/e2e/tests/office/project-provider-source-helpers.ts` (new)
- `docs/public/office-tasks.md`
- `docs/public/plugins-authoring.md`

Validation passed:

- `pnpm exec vitest run app/office/projects/repo-entry.test.ts`: six tests.
- `pnpm exec prettier --check` and `pnpm exec eslint` on both Office specs and the shared helper.
- `pnpm e2e:run --project chromium --no-build -- tests/office/new-task-dialog.spec.ts`: seven tests.
- `pnpm e2e:run --project chromium --no-build -- tests/task/plugin-repository-task-create.spec.ts`: two tests.
- `pnpm e2e:run --project mobile-chrome --no-build -- tests/office/mobile-new-task-dialog.spec.ts tests/task/mobile-plugin-repository-task-create.spec.ts`: six tests.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests. `node scripts/validate-public-docs.mjs`: 54 pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `git diff --check`.

The browser runs prove admission and persistence. They do not claim a live self-managed clone, live vendor authentication, or first-launch credential evidence. The packaged fixture proves the host contract, not every third-party plugin release.
