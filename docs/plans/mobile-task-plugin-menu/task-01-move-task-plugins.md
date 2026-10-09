---
id: "01-move-task-plugins"
title: "Move task plugins into the phone menu"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-CHROME-001.5
  - AC-UI-MOBILE-TASK-CHROME-001.6
  - AC-UI-MOBILE-TASK-CHROME-001.7
system_design:
  - ../../specs/ui/system-design/mobile-task-chrome.md
---

# Task 01: Move Task Plugins into the Phone Menu

## Summary

Add an explicit responsive contract to the task session plugin slot, move its
phone presentation from the fixed task header into the shared menu's Plugins
section, and prove it with the packaged plugin fixture. Preserve the inline
tablet/desktop surface and all existing task and session context.

## In scope

- Add RED component and contract assertions before production changes.
- Export and consume the canonical `ChatTopBarSlotProps` SDK type with a
  desktop/mobile presentation.
- Add the mobile containment wrapper and reactive registration-presence check.
- Thread phone task plugin content through `AppNavSheet`, `AppNavSections`, and
  `MobilePluginNavSection`.
- Remove the inline phone plugin slot while retaining first-party controls.
- Update the plugin API reference and public authoring guide.
- Extend the packaged fixture and add the mobile Playwright regression after a
  focused component assertion records the fixed-header placement failure.
- Verify long-title geometry, wrapping, interaction, menu focus/dismissal, and
  zero document overflow through the managed production build.
- Capture and validate the synthetic closed-header and open-menu PR assets.

## Out of scope

- First-party task header action placement.
- Backend or persisted-state changes.
- Publishing screenshot binaries on the PR branch; `/pr` publishes ignored
  capture output on the dedicated media ref.
- Reusing the user's attached screenshot in a public PR.

## Acceptance

- The fixed phone header never contains a `chat-top-bar` registration, and the
  same registration appears in the shared Plugins section only when present.
- Mobile slot props preserve task/session identity, report `presentation:
  "mobile"`, wrap within the menu, and give host buttons a minimum 44px active
  target; desktop reports `presentation: "desktop"` and keeps existing markup.
- The combined Plugins section renders once, preserves plugin link dismissal,
  and disappears when it has neither actions nor destinations.
- A real packaged plugin with multiple contributions leaves a synthetic
  long-title phone header usable, stays inside the menu viewport, activates its
  control, and produces both expected PR-capture manifest entries.

## ASCII UI preview

### UI-01: Phone task header with plugin contributions

Full preview: [plan.md](plan.md#ui-01-phone-task-header-with-plugin-contributions).

```text
Before: | < | task... | [plugin] [status] | = |
After:  | < | task identity + branch...   | = |

Menu:   Plugins
        [plugin action] [session status]
        [plugin destination]
```

The fixed header owns task identity and the menu trigger. The existing menu
owns wrapping plugin content. Maps to
`AC-UI-MOBILE-TASK-CHROME-001.5`, `.6`, and `.7`.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run components/task/task-top-bar-plugin-actions.test.tsx components/task/mobile/session-mobile-top-bar.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/navigation/app-nav-sheet.test.tsx lib/plugins/sdk-contract.test.ts)
(cd apps && pnpm --filter @kandev/plugin-sdk typecheck)
(cd apps && pnpm --filter @kandev/web exec eslint components/task/task-top-bar-plugin-actions.tsx components/task/task-top-bar-plugin-actions.test.tsx components/task/mobile/session-mobile-top-bar.tsx components/task/mobile/session-mobile-top-bar.test.tsx components/plugins/mobile-plugin-nav-section.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/navigation/app-nav-sheet.tsx components/navigation/app-nav-sheet.test.tsx components/navigation/app-nav-sections.tsx lib/plugins/types.ts lib/plugins/sdk-contract.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-topbar.spec.ts)
test -s apps/web/.pr-assets/manifest.json
node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync("apps/web/.pr-assets/manifest.json","utf8")); const names=m.assets?.map((a)=>a.file??a.name)??[]; for (const expected of ["mobile-task-plugin-header.png","mobile-task-plugin-menu.png"]) { if (!names.some((name)=>String(name).endsWith(expected))) throw new Error(`missing ${expected}`); }'
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/packages/plugin-sdk apps/web docs/public docs/plans/plugins docs/specs/ui docs/plans/mobile-task-plugin-menu
```

## Files likely touched

- `apps/packages/plugin-sdk/src/index.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/sdk-contract.test.ts`
- `apps/web/components/task/task-top-bar-plugin-actions.tsx`
- `apps/web/components/task/task-top-bar-plugin-actions.test.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar.test.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-repository.test.tsx`
- `apps/web/components/navigation/app-nav-sheet.tsx`
- `apps/web/components/navigation/app-nav-sheet.test.tsx`
- `apps/web/components/navigation/app-nav-sections.tsx`
- `apps/web/components/plugins/mobile-plugin-nav-section.tsx`
- `apps/web/components/plugins/mobile-plugin-nav-section.test.tsx`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`
- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`
- `apps/web/.pr-assets/manifest.json` (ignored capture output)
- `apps/web/.pr-assets/mobile-plugin-topbar--mobile-task-plugin-header.png`
  (ignored capture output)
- `apps/web/.pr-assets/mobile-plugin-topbar--mobile-task-plugin-menu.png`
  (ignored capture output)

## Dependencies

None.

## Risks

- The slot accepts opaque plugin markup; enforce host containment without
  assuming a plugin's internal DOM shape beyond documented host buttons.
- Do not render an empty Plugins heading during registry install/uninstall.
- Fixture registrations are shared by tests that install the package; keep them
  inert until a task detail page mounts the slot and retain the existing listing
  scenario in the focused E2E run.
- Wait for finite drawer animations before geometry or screenshot reads.

## Parallelism

`sequential`

## Inputs

- `AC-UI-MOBILE-TASK-CHROME-001.5`, `.6`, and `.7`.
- `docs/specs/ui/system-design/mobile-task-chrome.md`, especially Components and
  responsibilities, Plugin contribution, and Mobile design contract.
- `apps/web/components/kanban/main-top-bar-plugin-actions.tsx` and
  `apps/web/components/kanban/mobile-listing-menu-actions.tsx` as the shipped
  responsive slot exemplar.
- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`,
  `apps/web/e2e/helpers/layout-assertions.ts`, and
  `apps/web/e2e/helpers/animations.ts` for production-build proof.
- `apps/web/e2e/fixtures/test-base.ts` `prCapture` fixture.

## Results

- RED: the phone-header component test failed because the plugin action was
  still inside `mobile-topbar-actions`; it passed after the menu handoff.
- Focused Vitest suite: 49 tests passed.
- Plugin SDK typecheck, web typecheck, and targeted ESLint passed.
- Managed production-build mobile E2E: 2 tests passed with the packaged plugin.
- Both synthetic PR screenshots are present in the capture manifest and were
  visually checked.
- Review fixup scoped the 44px guarantee to `host.ui.Button`, corrected the
  documented phone placement for `main-top-bar`, and reran the managed mobile
  E2E scenarios successfully after updating the fixture action.
- Public-doc and specification validation passed; final diff checks passed.
