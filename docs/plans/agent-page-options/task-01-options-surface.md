---
id: "01-options-surface"
title: "Move the agent preference into Options"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PAGE-OPTIONS-001
acceptance_criteria:
  - AC-AGENTS-PAGE-OPTIONS-001.1
  - AC-AGENTS-PAGE-OPTIONS-001.2
  - AC-AGENTS-PAGE-OPTIONS-001.3
  - AC-AGENTS-PAGE-OPTIONS-001.4
  - AC-AGENTS-PAGE-OPTIONS-001.5
  - AC-AGENTS-PAGE-OPTIONS-001.6
  - AC-AGENTS-PAGE-OPTIONS-001.7
  - AC-AGENTS-PAGE-OPTIONS-001.8
system_design:
  - ../../specs/agents/system-design/agent-page-options.md
---

# Task 01: Move the agent preference into Options

## Summary

Implement the approved Options entry point with a desktop dialog and phone drawer.
Reuse the existing visibility hook and prove the complete interaction on both surfaces.

## In scope

- Toolbar integration, shared open state, responsive overlay, and the existing switch.
- Approved localized copy and semantic titles, descriptions, focus, dismissal, and touch targets.
- Focused component and E2E tests, public guide text, and updated legacy UI placement references.

## Out of scope

- New preferences, filtering changes, backend persistence, shared primitive changes, and Office controls.

## Acceptance

- The rendered page and desktop dialog match UI-01/UI-02 with immediate application and correct dismissal.
- The phone drawer matches UI-03 with intrinsic height capped by the dynamic viewport and passes touch, containment, focus, and breakpoint checks; the tablet dialog retains touch-sized controls on a coarse pointer.
- All eight linked criteria have targeted evidence, translations pass their gates, and public guidance describes the implemented entry point.

## ASCII UI preview

These excerpts use the [full plan previews](plan.md#ascii-ui-preview).
Structure is required. Spacing is illustrative. Applicable criteria: .1 through .8.

```text
UI-01: Proposed desktop toolbar
[Options] [Terminal] [Rescan] [+ Add custom agent]
Agent cards follow directly.

UI-02: Desktop Options, open
+--------------------------------------------+
| Agent options                          [x] |
| Hide disabled profiles                     |
| from navigation                      [OFF] |
| Disabled profiles remain available         |
| on this page.                              |
| Changes apply immediately.          [Done] |
+--------------------------------------------+

UI-03: Phone Options drawer, open
+-------------------------------+
| Dimmed agents page            |
| +---------------------------+ |
| |            ---            | |
| | Agent options             | |
| | Hide disabled profiles    | |
| | from navigation     [OFF] | |
| | Disabled profiles remain  | |
| | available on this page.   | |
| | Changes apply immediately.| |
| | [         Done          ] | |
| +---------------------------+ |
+-------------------------------+
```

Phone: fixed title/footer, one internal body scroll region, safe-area clearance, and 44px targets.
Successful toggles leave the active surface open. Dismissal retains the value and returns focus to Options.

## Verification

Run from the repository root. In a fresh worktree, first install dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Use TDD for the changed interaction. Run these checks after implementation:

```bash
(cd apps/web && pnpm exec vitest run app/settings/agents/page.test.tsx app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts hooks/use-local-storage-boolean.test.ts)
(cd apps/web && pnpm exec eslint app/settings/agents/page.tsx app/settings/agents/agent-options-dialog.tsx app/settings/agents/hide-disabled-agent-profiles-setting.tsx --max-warnings 0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-page-options.spec.ts tests/settings/hide-disabled-agent-profiles-nav.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-page-options.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds before each project. Confirm discovery and retain all running command handles.
Compare screenshots with UI-02/UI-03 and record measured target sizes and viewport containment.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx`
- `apps/web/app/settings/agents/agent-options-dialog.tsx` (new)
- `apps/web/app/settings/agents/page.test.tsx`
- `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx`
- `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx`
- `apps/web/e2e/tests/settings/agent-page-options.spec.ts` (new)
- `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-page-options.spec.ts` (new)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/{agents,settings,common}.json` (only catalogs with changed keys)
- `docs/public/agents-and-profiles.md`
- `docs/specs/agents/requirements/hide-disabled-profiles-nav.md` (UI placement/label references only)
- `docs/specs/agents/requirements/agent-page-options.md`
- `docs/specs/agents/system-design/agent-page-options.md`
- `docs/plans/agent-page-options/plan.md`
- `docs/plans/agent-page-options/task-01-options-surface.md`

## Dependencies

None. Reuse the current preference hook, settings control sizing, and Dialog/Drawer primitives.

## Risks

Primitive responsive styles, translated labels, focus during breakpoint changes, and small switch targets need rendered evidence.
Keep Options outside agent-management permission gates.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-page-options.md).
- [System design](../../specs/agents/system-design/agent-page-options.md).
- Scoped web guidance, mobile-parity and e2e skills, existing preference tests, and the mobile menu drawer pattern.

## Results

Completed on 2026-10-05. The route-local Options trigger now appears before
Terminal and owns the responsive dialog/drawer open state. The existing profile
visibility preference is inside that surface, applies immediately, stays open
after a toggle, and retains its value when reopened, reloaded, or resized across
the 768px boundary. Focus follows the preference when the open surface changes
at that breakpoint and returns to Options on dismissal. The standalone
preference row is removed. Options remains available to a member while the
management-only Add custom agent action is hidden.

- Focused Vitest suite passed: 4 files, 23 tests.
- Targeted ESLint (`--max-warnings 0`) and Prettier checks passed.
- `pnpm run typecheck` passed. The production Vite build completed in the
  managed E2E runner.
- `pnpm run i18n:check` passed for all supported catalogs and the pseudo locale.
- Managed Chromium E2E passed: 7 tests across
  `agent-page-options.spec.ts` (6) and
  `hide-disabled-agent-profiles-nav.spec.ts` (1). Coverage includes keyboard
  Enter activation, focus across both breakpoint transitions and dismissal,
  navigation filtering, member access, and 44px touch targets on a coarse
  pointer.
- Managed Pixel 5 mobile Chromium E2E passed: 1 test in
  `mobile-agent-page-options.spec.ts`. All three touch targets are asserted to
  meet the 44px minimum in both dimensions. For the single-preference surface,
  the drawer is below 70% of viewport height and has an 80dvh dynamic maximum.
  Rendered measurements: Options 44px high, switch target 44x44px, Done 48px
  high, and footer bottom padding at least 16px. The drawer remains within the
  viewport, has one internal scroll region, and adds no horizontal document
  overflow.
- Host screenshots were inspected against UI-02 and UI-03. The desktop dialog
  and phone drawer match their approved structure.
- Public-doc tests passed (62 tests), and all 47 public pages validated.
- `python3 scripts/list-docs.py validate` passed (351 decisions and 1357
  specifications); all 36 spec-linter tests and the full spec lint passed.
- `git diff --check` passed.
