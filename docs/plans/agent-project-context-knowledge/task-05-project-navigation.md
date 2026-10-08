---
id: "05-project-navigation"
title: "Compact project navigation and delivery evidence"
status: complete
wave: 5
depends_on:
  - "04-project-creation"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-004.4
  - AC-PROJECTS-AGENT-PROJECTS-005.1
  - AC-PROJECTS-AGENT-PROJECTS-005.2
  - AC-PROJECTS-AGENT-PROJECTS-005.5
  - AC-PROJECTS-AGENT-PROJECTS-005.6
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 05: Compact Project Navigation and Delivery Evidence

## Summary

Place Projects below Integrations in the default ordinary sidebar. Save space
when empty and hide the archived entry while retaining lifecycle APIs.

## In scope

- Own default ordinary navigation composition in
  `apps/web/components/app-sidebar/app-sidebar.tsx`, ordinary Projects projection
  in `sections/projects-section.tsx`, and necessary UI state fallback/tests.
- Order Integrations, Projects, then Tasks in the default ordinary branch.
  Preserve saved custom layout ordering and Office Projects behavior.
- Use data-dependent section defaults when no saved choice exists. Remove an
  unconditional global Projects default that masks the per-section fallback;
  preserve stored booleans and explicitly default Office Projects as before.
- Empty/unloaded projects start collapsed; plus remains usable when closed.
  Preserve a user's explicit expansion across refresh and project removal.
- Hide ordinary archived button/rows and remove sidebar-only archived loading.
  Keep project menus, archive/delete confirmation, context retention, restore
  service/API, and worker/state refresh behavior.
- Reconcile lifecycle E2E by restoring through the API; do not delete archive,
  restore, cascade failure, or context-preservation assertions.
- Add default-order, empty create, stored-choice, custom-layout, and Office
  regression tests. Include phone navigation and return focus.
- Update public navigation guidance and capture final desktop/phone evidence.

## Out of scope

- New archived-project access, global layout migration, or Office redesign.
- Backend archive/restore contract changes, feature-flag changes, commit/push.

## Acceptance

The approved `UI-05` preview in [plan.md](plan.md#ascii-ui-preview) matches the
default ordinary sidebar. No empty placeholder or archive row consumes space
until explicitly expanded. Existing stored preferences remain authoritative,
creation works from the closed header, and project/worker lifecycle still works.

## Verification

Use focused unit/component tests for changed default/preference/layout logic.
Run changed-source eslint and typecheck, then desktop/mobile Projects E2E once
the final navigation changes are ready. Preserve existing Office tests; run the
focused Office sidebar/navigation regressions if affected shared logic changes.

```bash
pnpm --dir apps/web exec vitest run components/app-sidebar lib/state/slices/ui lib/local-storage-app-sidebar.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/projects/agent-projects.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/projects/mobile-agent-projects.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a named test path differs, run the existing equivalent and record it. Use
one managed shard and keep browser suites sequential. Report fresh-build paths,
desktop/phone screenshots, control dimensions, focus, scroll ownership, and any
mocked-provider limitation. The coordinator reviews code/rendering and promotes
spec/plan statuses after all checks and corrections pass.

## Results

Implemented the default ordinary sidebar order, data-dependent Projects
expansion, and hidden archived navigation. Empty or not-yet-loaded Projects
starts closed while its add action stays visible; saved expansion booleans remain
authoritative, and Office retains its explicit expanded default. Saved custom
layouts and Office section order remain unchanged. Sidebar-only archived
project loading and restore rows were removed; archive, delete, context, and
restore API behavior remain covered. Public navigation guidance now states the
default order and collapsed empty state, and explains that archived projects are
hidden from the sidebar while the API retains restore support.

Passed verification:

- `pnpm vitest run components/app-sidebar/app-sidebar-section.test.tsx
  components/app-sidebar/app-sidebar.test.tsx
  components/app-sidebar/sections/projects-section.test.tsx
  lib/state/slices/ui/ui-slice.test.ts lib/local-storage.test.ts`: 5 files,
  113 tests passed. The wider sidebar/UI-state run also passed 49 files and
  530 tests before the test-grouping lint extraction.
- Task05-focused changed-file ESLint passed with `--max-warnings 0`; the final
  all-changed frontend audit covered 40 TS/TSX paths and passed. Typecheck,
  `i18n:check`, and `i18n:ratchet` passed. Prettier formatted all changed web
  TS/TSX and locale JSON files.
- Public documentation checks passed: 62 validator tests and all 47 published
  pages. The final `git diff --check` passed.
- Managed desktop Projects E2E passed 7/7 with
  `pnpm e2e:run --host --shards 1 --project chromium
  tests/projects/agent-projects.spec.ts` (session 70641).
- Managed mobile Projects E2E passed 2/2 with
  `pnpm e2e:run --host --shards 1 --project mobile-chrome
  tests/projects/mobile-agent-projects.spec.ts` (session 55984).
- A capture-only tablet rerun passed 1/1 with the coordinator-help drawer
  hidden and finite portal animations settled before the form screenshot
  (session 86716).
- An initial desktop run exposed an incorrect assertion that expected the empty
  placeholder to remain visible when the section correctly collapsed after the
  last project was archived. The test now checks the collapsed state and then
  explicitly expands the header before checking the empty placeholder. The
  separate coordinator profile test passed 1/1; the first full-run cleanup had
  an unexplained backend exit after the earlier lifecycle assertion aborted,
  and the final full desktop run passed without that failure.

Desktop captures are preserved at
`/tmp/kandev-projects-refinements-evidence/task05-desktop-final/` (default form,
Advanced settings, repository picker, context advice, and 393×600 narrow
fine-pointer form). Its sidebar screenshot retains the Tasks loading skeleton;
the coordinator's settled dark-theme sidebar check supplies the final
navigation visual review. Clean 820×900 tablet form/picker/help captures are
preserved at `/tmp/kandev-projects-refinements-evidence/task05-tablet-final/`.
Phone captures are preserved at
`/tmp/kandev-projects-refinements-evidence/task05-mobile-final/` (393×851
form/Advanced/help/sidebar/context and delete confirmation). The
project create action measures 24×24 CSS pixels on fine desktop and 44×44 on
touch; phone form fields, selectors, help actions, and footer use 44px targets.
The mobile help test verifies Escape dismissal restores focus and the form has
no horizontal overflow.
