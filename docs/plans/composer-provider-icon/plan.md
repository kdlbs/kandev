---
created: 2026-09-16
status: implemented
requirements:
  - REQ-UI-COMPOSER-PROVIDER-ICON-001
system_design:
  - ../../specs/ui/system-design/composer-provider-icon.md
legacy_specs: []
---

# Composer provider icon implementation plan

## Overview and scope

One sequential work order adds a small leading CLI logo to the composer model
picker on desktop and phone. Reuse `AgentLogo` and existing identity data.
Exclude provider switching, settings changes, model-row branding and backend work.

## Technical approach

Follow the [design](../../specs/ui/system-design/composer-provider-icon.md):
optional provider icon slot in the trigger and popover heading, session identity resolution, explicit composer opt-in.

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

## Tests

Add `components/task/model-selector-provider-icon.test.tsx` for snapshot priority,
profile fallback, unknown identity, session changes and same-provider model changes
(AC .1 and .2). Extend `components/model-config-selector.test.tsx` for optional
slot rendering and unchanged accessible name (AC .3). Desktop and mobile E2E prove both composer presentations opt in; existing toolbar component tests guard unrelated controls.

## E2E tests

Add `tests/chat/model-selector-provider-icon.spec.ts` (chromium): loaded provider
logo, model change retaining logo, failed-logo fallback, keyboard use, long-label
containment and light/dark appearance. Extend `tests/chat/mobile-model-selector.spec.ts`
(mobile-chrome) to assert icon, touch model selection, >=44px hit target,
long-label containment and zero horizontal document overflow. These cover AC .1-.3.
Use existing isolated fixtures; capture the trigger for visual comparison with UI-01.

## Work orders

- [x] [Task 01: Add composer provider icon](task-01-provider-icon.md)

## Verification

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

## Verification results

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

## Risks

Snapshot values are untyped and require a nonempty string check. Long summaries
can crowd the new icon; preserve nonshrinking glyphs and text truncation. Logo
fallback must not be mistaken for proof that the branded image loaded.
