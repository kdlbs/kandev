---
id: "02-enforce-task-links"
title: "Migrate and enforce task URL construction"
status: done
wave: 2
depends_on: ['01-shared-task-links']
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-001.1
  - AC-UI-TASK-NAVIGATION-001.2
  - AC-UI-TASK-NAVIGATION-001.3
system_design:
  - ../../specs/ui/system-design/task-navigation.md
---

# Task 02: Migrate and enforce task URL construction

## Summary

Make the default lint command reject recurring URL and anchor bypasses, with zero current violations.

## In scope

- Add the bounded AST rule and actual-config wiring test described in the design.
- Inventory and migrate every production task-detail builder; classify route-recognition constants separately.
- Keep existing compliant Link/router consumers; migrate raw task anchors to TaskLink.
- Update apps/web/AGENTS.md with URL and anchor ownership. No new ADR is needed: this applies existing authorities, with alternatives documented in the design.

## Out of scope

Backend behavior and unrelated navigation redesign.

## Acceptance

- RED: rule fixtures reject original DependencyRow, raw anchors using aliased helper calls, and hand-built URLs; valid API/Office/matcher cases pass.
- Final production lint has no task-link violations or migration baseline; real config catches a new component in an arbitrary directory.
- Migrated caller tests preserve context and selection side effects; new workbench destinations use canonical URLs.

## Verification

Run from repository root. Install once with `(cd apps && pnpm install --frozen-lockfile)` if this worktree lacks dependencies.

```bash
(cd apps/web && pnpm exec vitest run eslint-rules/no-task-link-bypass.test.ts scripts/lib/task-link-wiring.test.ts lib/links.test.ts)
(cd apps/web && pnpm exec vitest related --run $(cat /tmp/kandev-task-link-changed-sources.txt))
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
git diff --check
```

## Files likely touched

- `apps/web/eslint-rules/no-task-link-bypass.mjs (new)`
- `apps/web/eslint-rules/no-task-link-bypass.test.ts (new)`
- `apps/web/scripts/lib/task-link-wiring.test.ts (new)`
- `apps/web/eslint.config.mjs`
- `apps/web/AGENTS.md`
- `First-party callers listed in migration-inventory.md and their tests`

## Dependencies

01-shared-task-links.

## Risks

Preserve existing route guards, query context, and browser-native alternate-tab behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-navigation.md)
- [System design](../../specs/ui/system-design/task-navigation.md)
- Existing AppLink, linkToTask, local ESLint rules, and task dependency tests.

## Results

Completed on 2026-09-21.

- The bounded `task-links/no-task-link-bypass` rule and real-config wiring test
  passed, with 31 tests in the initial focused rule and link suite.
- All known first-party task-detail builders and raw task anchors were migrated;
  the production rule scan found zero violations.
- Review follow-up now checks quoted and expression literal task URLs on custom
  `Link` and `AppLink` elements while preserving shared-builder cases. Static
  evaluation uses branch-local recursion guards, so prefix aliases combined
  with dynamic task IDs are detected in anchors and `location.assign` calls.
- The broad related Vitest run before review follow-up passed 437 files and
  4,254 tests.
- Review follow-up coverage passed 3 files and 38 rule/link tests; the combined
  rule, AppLink, wiring, and link suite passed 4 files and 48 tests.
- `pnpm run typecheck`, `pnpm run lint`, Prettier checks, and `git diff --check`
  passed.
