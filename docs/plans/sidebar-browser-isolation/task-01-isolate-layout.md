---
id: "01-isolate-layout"
title: "Isolate sidebar layout between browser tests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-007
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.4
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Isolate sidebar layout between browser tests

## Summary

Reset the shared E2E workspace's navigation layout before opening each test
page. Use the current settings revision and the existing default-layout reset.
Keep layout persistence within a scenario and preserve real task submission.

## Files

- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/tests/settings/sidebar-direct-customization.spec.ts`
- `apps/web/e2e/tests/settings/mobile-sidebar-direct-customization.spec.ts`
- `apps/web/e2e/tests/task/create-task-url-reopen-no-branches.spec.ts`

## Verification and results

The following receipt is preserved from the earlier work-order appendix. The
broader feature record remains unchanged in its original package.

### PR #3598 test-isolation remediation (2026-10-07)

The shared E2E page fixture reset sidebar views but retained the previous test's
workspace layout. The existing collapse-persistence scenario followed by URL
repo task creation reproduced an intercepted New Task click. The page fixture
now resets the layout through `sidebar_layout_state`, using the current saved
revision and the API's default-layout reset. A test still retains its own saved
layout across reloads. The URL spec's one-retry override was removed.

Validation from `apps/web`:

```sh
E2E_PORT_OFFSET=0 pnpm e2e:run --no-build --project chromium tests/settings/sidebar-direct-customization.spec.ts tests/task/create-task-url-reopen-no-branches.spec.ts -- --grep 'collapses every navigation entry|repo added via GitHub URL' --retries=0 --repeat-each=3
E2E_PORT_OFFSET=0 pnpm e2e:run --no-build --project chromium tests/settings/sidebar-direct-customization.spec.ts tests/task/create-task-url-reopen-no-branches.spec.ts tests/task/create-task-branch-policy.spec.ts -- --retries=0
```

Results: six and nine tests passed respectively, without retries. These checks
preserve the collapsed-state persistence assertions and actual task submission.

Rebuilt desktop validation after integrating main's workflow-start-selection
fix passed all nine tests in sidebar direct customization, URL repo reopen, and
file-tree drag-and-drop. The matching mobile sidebar customization file passed
one test using `--project mobile-chrome --retries=0`. Focused ESLint, web
typecheck, catalog validation, and documentation coverage passed (72 work
orders). Native Windows/macOS and targeted durable-delivery PostgreSQL/live
harness release gates remain open; hosted replacement-head CI is pending.
