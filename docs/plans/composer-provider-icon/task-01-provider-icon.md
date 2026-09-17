---
id: "01-provider-icon"
title: "Add composer provider icon"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-PROVIDER-ICON-001
acceptance_criteria:
  - AC-UI-COMPOSER-PROVIDER-ICON-001.1
  - AC-UI-COMPOSER-PROVIDER-ICON-001.2
  - AC-UI-COMPOSER-PROVIDER-ICON-001.3
system_design:
  - ../../specs/ui/system-design/composer-provider-icon.md
---

# Task 01: Add composer provider icon

## Summary and scope

Add the session CLI's existing logo before the model summary and in the open popover in both composer
presentations. Implement the optional shared trigger slot and explicit composer
opt-in. Exclude backend changes, non-composer consumers and model-menu redesign.

## Acceptance

1. Correct session provider logo, snapshot priority and profile fallback pass AC .1.
2. Unknown identity and failed logo remain usable under AC .2.
3. Desktop and phone preserve selection, accessibility and containment under AC .3.

## ASCII UI preview

### UI-01: Closed composer model picker, desktop and phone

```text
Before: [ Model name / Low       v ]
After:  [ <CLI> Model name / Low v ]
Narrow: [ <CLI> Long model...    v ]
```

`<CLI>` represents the actual 14px provider logo, not visible text. Leading logo,
truncatable summary, trailing chevron is the required order; sample text and ASCII
spacing are illustrative. Phone uses the same ordering inside its existing toolbar
and touch hit area. Missing identity uses the before state; loading/failed logo
uses a terminal glyph in the same slot. Open popover: `<CLI> Model` heads the model group, using the same provider icon. Model rows and option navigation remain unchanged.
Maps to AC-UI-COMPOSER-PROVIDER-ICON-001.1 through .3.

Full preview and test mapping: [plan](plan.md#ascii-ui-preview).

## Files likely touched

- `apps/web/components/model-config-selector.tsx` and its `.test.tsx`.
- `apps/web/components/task/model-selector.tsx`, `model-selector-provider.ts`, and new `model-selector-provider-icon.test.tsx`.
- `apps/web/components/task/chat/chat-input-toolbar-desktop.tsx`, `chat-input-toolbar-mobile.tsx`, and adjacent toolbar tests.
- `apps/web/e2e/tests/chat/model-selector-provider-icon.spec.ts` (new) and `mobile-model-selector.spec.ts`.

## Inputs and dependencies

Read the linked requirement/design, UI-01, scoped web guidance and existing
`AgentLogo`. No preceding work orders. Apply TDD to identity resolution and prop
wiring, using the exact component/browser cases described in the plan.

## Verification

Commands run from repository root. Build local development binaries and fresh frontend assets explicitly, then run the guarded E2E runner without redundant cross-platform builds.
Run desktop and mobile sequentially. Inspect captured glyphs in both themes.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/model-config-selector.test.tsx components/task/model-selector.test.ts components/task/model-selector-consecutive-switch.test.tsx components/task/model-selector-provider-icon.test.tsx components/task/chat/chat-input-toolbar.test.tsx components/task/chat/chat-input-toolbar-composer.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/model-config-selector.tsx components/model-config-selector-content.tsx components/task/model-selector-provider.ts components/task/model-selector.tsx components/task/chat/chat-input-toolbar-desktop.tsx components/task/chat/chat-input-toolbar-mobile.tsx)
make -C apps/backend build-dev
(cd apps/web && pnpm build:e2e)
make -C apps/backend e2e-plugin-ui e2e-plugin-package
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/chat/model-selector-provider-icon.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-model-selector.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks

Untyped snapshots, delayed profile hydration, and crowded phone labels. Do not
infer provider from the model string or duplicate the logo registry.

## Parallelism

Sequential.

## Results

Completed 2026-09-16; revalidated against the updated base on 2026-09-17.

- Frozen workspace install passed.
- Listed focused Vitest command passed: 6 files, 89 tests, including provider identity and model-switch regression coverage.
- `pnpm run typecheck` passed. Targeted ESLint and Prettier checks passed.
- `make -C apps/backend build-dev`, `pnpm build:e2e` from `apps/web`, and fixture plugin build/package passed. Existing Vite chunk warnings remain.
- Desktop runner command with `--host --no-build`: 2 passed. Covers both icon placements, loaded logo, failed-logo fallback, keyboard opening, model switching and long-label containment.
- Mobile runner command with `--host --no-build`: 1 passed. Covers both placements, 44px trigger, touch configuration selection and viewport containment. Existing test captures the mobile result.
- Desktop screenshots inspected. Browser evidence caught shared button CSS shrinking the terminal fallback; explicit `size-3.5` fixes it. A desktop backend-readiness timeout passed on rerun; the initial mobile heading locator was corrected before its passing run.
- Catalog validation, specification lint and `git diff --check` passed.

Full remote-platform build was stopped because local browser checks require only
host helpers. All browser runs used freshly rebuilt frontend assets and an isolated
runtime. Fresh desktop and phone PR screenshots were captured with temporary specs, inspected, and retained outside the production diff. The temporary capture specs were removed.

PR review remediation: dynamic snapshots carry `agent_id` without `agent_name`.
Resolve that ID through the existing profile catalog rather than using it as a
logo name. The new regression failed before the fix and passes afterward.
Memoized provider JSX preserves the selector's existing memo boundary; heading
assertions now report missing accessible labels explicitly. The focused suite
passes 89 tests after these changes. Typecheck and targeted lint pass.
