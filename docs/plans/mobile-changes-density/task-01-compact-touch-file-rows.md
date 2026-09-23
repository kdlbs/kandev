---
id: "01-compact-touch-file-rows"
title: "Compact touch file rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-002
acceptance_criteria:
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.1
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.2
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.3
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.4
system_design:
  - ../../specs/ui/system-design/changes-file-row-containment.md
---

# Task 01: Compact touch file rows

## Summary

Give filenames the primary width and group touch actions in one menu. Preserve
desktop rows and the existing repository/layer-aware handlers.

## In scope

Touch rows, menu, pending feedback, browser regression tests, screenshots, docs.

## Out of scope

Toolbar redesign, Git transport, provider rows, saved preferences, new APIs.

## Acceptance

- Phone rows follow UI-01 and meet the geometry/action criteria.
- Stage, Unstage, Edit, Discard cancellation, and diff opening work by touch.
- Desktop inline actions and tree behavior remain valid.

## ASCII UI preview

UI-01 from the [full preview](plan.md#ascii-ui-preview):

```text
[file] status-surface-metrics.test.ts  [...]
       apps/web/components/task +12 -3
Menu: full path / Stage file / Edit / Discard changes
```

UI-02 desktop retains `[+] folder/name.ts  +12 -3 M` and hover actions.
AC-002.1 through AC-002.4 require the filename hierarchy, single touch menu,
44px targets, action parity, and unchanged desktop density.

## Verification

Run from the repository root after `pnpm install --frozen-lockfile` in `apps`:

```bash
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-changes-panel.spec.ts)
(cd apps/web
cat > e2e/playwright.local-desktop.config.ts <<'EOF'
import config from "./playwright.config";
export default {
  ...config,
  projects: config.projects?.map((project) => project.name === "chromium"
    ? { ...project, testIgnore: [/\/mobile-[^/]*\.spec\.ts$/, ...(project.testIgnore as RegExp[]).slice(1)] }
    : project),
};
EOF
pnpm e2e:run --host --no-build --project chromium tests/git/git-changes-panel.spec.ts -- --config e2e/playwright.local-desktop.config.ts --grep 'keeps compact desktop|keeps stage and unstage|shows same path|shows modified files in unstaged'
desktop_result=$?
rm e2e/playwright.local-desktop.config.ts
exit "$desktop_result")
(cd apps/web && pnpm exec vitest run components/task/changes-panel-file-row.test.tsx components/task/changes-panel-tree.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/changes-panel-file-row.tsx components/task/changes-panel-touch-file-row.tsx)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/mobile-changes-density
```

## Files likely touched

- `apps/web/components/task/changes-panel-file-row.tsx`
- `apps/web/components/task/changes-panel-file-row.test.tsx`
- `apps/web/components/task/changes-panel-touch-file-row.tsx`
- `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `docs/public/sessions-and-review.md`

## Dependencies

None.

## Risks

Preserve trigger focus around portaled menus and discard confirmation.

## Parallelism

`sequential`

## Inputs

The linked file-row requirement/design, existing GitHelper fixtures, shared
DropdownMenu primitives, and mobile UI language contextual-action pattern.

## Results

- RED: the phone filename-width assertion failed at 46.7% against a required minimum of 65%.
- GREEN: managed production build and mobile run: **9 passed**, including flat/tree geometry, 22-level tree containment, stage/unstage, discard cancellation, Edit, and layer/PR diff routing.
- Desktop: **4 passed**, including the new 767px/768px transition and existing stage/unstage pending feedback and mixed-layer scenarios.
- The default desktop discovery excluded this worktree because its absolute path contains `mobile-`. The temporary config above scopes that exclusion to the spec basename; it changes no test behavior and was removed after the run.
- File-row and tree Vitest suites: **24 passed**. Updated obsolete coarse-pointer assertions and covered the pending menu and fine-pointer phone presentation.
- Review regression RED: a 22-level tree left zero filename width. GREEN: touch indentation is capped at 24px; the same browser test preserves more than half the row width for the basename and keeps status/menu contained.
- Typecheck, ESLint with zero warnings (both components, the file-row unit tests, and both browser specs), Prettier check, and staged i18n ratchet: **passed**.
- Catalog validation: **299 decisions and 1112 specifications**. Specification linter tests: **36 passed**. Full specification lint and `git diff --check`: **passed**.
- Four fresh, inspected, compressed screenshots cover phone flat/tree lists, the action sheet, and desktop. Assets use disposable E2E data and remain outside the product branch.
- Rendered hierarchy matches UI-01/UI-02. No new API, persistence, or locale keys. Public review guide updated with the menu entry point.
