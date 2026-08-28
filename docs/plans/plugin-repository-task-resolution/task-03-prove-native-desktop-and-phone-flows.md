---
id: "03-prove-native-desktop-and-phone-flows"
title: "Prove native desktop and phone flows"
status: done
wave: 3
depends_on:
  - "02-preflight-task-repository-selections"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.1
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.5
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.8
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.9
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 03: Prove Native Desktop and Phone Flows

## Summary

Use the real packaged fixture to prove that the native task dialog creates a
task from an unregistered plugin repository and fails cleanly when inspection
is unavailable. Run the same user value on desktop and phone.

## In scope

- Add a desktop Playwright test that installs the fixture, opens the native task
  dialog, selects the plugin repository and a non-default branch, creates the
  task, and verifies the persisted repository identity and branch.
- Prove the selected repository did not exist before submission and exists
  after successful creation.
- Add a failure case for an inactive or unavailable inspect action and verify
  the dialog stays open, shows a safe error, and creates no task.
- Add a `mobile-*.spec.ts` test for the same first-use success path through the
  phone dialog.
- Verify phone repository and branch controls are reachable, the create action
  is touch-sized, and the dialog has no horizontal overflow.
- Use semantic roles, labels, and existing plugin fixture helpers. Do not add
  fixed sleeps or repeated blind retries.

## Out of scope

- A packaged external Bitbucket plugin dependency.
- Screenshots as the primary assertion.
- Re-testing every generic task-dialog field.
- New mobile-only product behavior.

## Acceptance

- Desktop creation from the fixture's first-use repository succeeds and opens
  a task attached to the canonical plugin repository and selected branch.
- A provider inspection failure leaves no task and keeps the user's form
  available with a bounded error.
- The phone flow provides the same selection and submit capability without
  overflow or undersized primary actions.
- Tests pass without retries in the focused desktop and mobile projects.

## Verification

Write the desktop first-use test before enabling the server resolver and
confirm that it reproduces the unsupported provider failure. Then run:

```bash
# From apps/web:
rtk pnpm e2e:raw --project=chromium e2e/tests/task/plugin-repository-task-create.spec.ts --retries=0
rtk pnpm e2e:raw --project=mobile-chrome e2e/tests/task/mobile-plugin-repository-task-create.spec.ts --retries=0
```

Run the fixture packaging command required by the existing E2E helper before
the focused specs when the helper does not build it automatically.

## Files likely touched

- `apps/web/e2e/tests/task/plugin-repository-task-create.spec.ts`
- `apps/web/e2e/tests/task/mobile-plugin-repository-task-create.spec.ts`
- `apps/web/e2e/helpers/plugin-fixture.ts`
- `apps/web/e2e/README.md` only if the fixture setup contract changes

## Dependencies

- Task 02 supplies the complete server-resolved task-create flow.

## Risks

- Reusing a persisted fixture repository can turn the test into the fast path.
  Assert absence before submit and isolate workspace state.
- Clicking a desktop-hidden control in the phone project gives false parity.
  Use the phone dialog's visible native controls.
- An optional external plugin package makes CI coverage disappear. Use the
  repository-owned fixture package.

## Parallelism

`sequential`

## Inputs

- The fixture action contract from Task 01.
- The task preflight contract from Task 02.
- Existing task-dialog and plugin fixture Playwright helpers.
- Mobile UI language and E2E cleanup guidance.

## Results

Added desktop and phone Playwright coverage using the packaged fixture. The
success cases prove first-use repository persistence, authoritative provider
identity, and selected branch handling. The desktop failure case proves the
dialog remains open with a bounded provider-unavailable message and no task or
repository write. The phone case also proves reachable controls, 44px submit
targets, and no horizontal overflow.

Verification passed without retries:

```text
pnpm e2e:raw --project=chromium e2e/tests/task/plugin-repository-task-create.spec.ts --retries=0
pnpm e2e:raw --project=mobile-chrome e2e/tests/task/mobile-plugin-repository-task-create.spec.ts --retries=0
```
