---
id: "04-e2e"
title: "E2E coverage, desktop and phone"
status: done
wave: 4
depends_on: ["03-scope-setting-picker"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.7
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.8
system_design:
  - ../../specs/integrations/system-design/github-repository-picker.md
---

# Task 04: E2E coverage, desktop and phone

## Outcome

Playwright proves the user flows end to end against the mock GitHub provider.

## Scope

- New `apps/web/e2e/tests/automations-github-repository-picker.spec.ts`:
  seed accessible repos via the mock GitHub client; on a new automation add
  "PR: opened", turn off "All repositories", open "Add repository", assert the
  repos are listed without typing, type a bare name and pick it, save, reload,
  and assert the trigger config holds `{ owner, name }`. Phone project variant
  asserts the popover is inside the viewport and the option is tappable.
- Extend `e2e/tests/integrations/github-workspace-settings.spec.ts` scope test
  to add repositories through the picker instead of the text field.
- Update the existing `automations-pr-merged-trigger.spec.ts` orgs-request
  test only if the orgs fetch moves.
- Re-run `create-task-remote-repo.spec.ts` to confirm task creation is
  unchanged.

## Exclusions

- Containers project.

## Requirements and design

- AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1–.8
- [System design](../../specs/integrations/system-design/github-repository-picker.md)

## Acceptance

- New and updated specs pass on desktop and phone projects.
- `make fmt && make typecheck test lint` passes.

## Verification

```bash
cd apps/web && pnpm e2e:run -- tests/automations-github-repository-picker.spec.ts tests/integrations/github-workspace-settings.spec.ts tests/automations-pr-merged-trigger.spec.ts tests/task/create-task-remote-repo.spec.ts
make fmt && make typecheck test lint
```

## Files likely touched

- `apps/web/e2e/tests/automations-github-repository-picker.spec.ts` (new)
- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/automations-pr-merged-trigger.spec.ts` (if needed)
- `apps/web/e2e/tests/mobile-automations-github-repository-picker.spec.ts` (new)
- `apps/web/e2e/helpers/github-repository-picker.ts` (new)

## Dependencies

Task 03.

## Results

- New desktop spec `automations-github-repository-picker.spec.ts` and phone spec `mobile-automations-github-repository-picker.spec.ts` (the `mobile-chrome` project only matches `mobile-*.spec.ts`), sharing `e2e/helpers/github-repository-picker.ts`. The helper sets an active workspace GitHub connection, which the picker requires.
- `github-workspace-settings.spec.ts` scope tests add repositories through the picker.
- Desktop run: 21 passed (new spec, GitHub workspace settings, PR merged trigger).
