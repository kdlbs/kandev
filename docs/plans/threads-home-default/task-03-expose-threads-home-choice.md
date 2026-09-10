---
id: "03-expose-threads-home-choice"
title: "Expose the Threads Home choice"
status: done
wave: 3
depends_on:
  - "02-resolve-threads-home-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003
acceptance_criteria:
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.1
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.2
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.3
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.1
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.2
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.3
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.6
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.7
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.8
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
---

# Task 03: Expose the Threads Home Choice

## Summary

Expose the saved Threads choice in the existing Startup Page radio card and
prove it through desktop and native phone navigation. Publish accurate help
copy and task-listing documentation with the finished behavior.

## In scope

- Use `/tdd`, `/e2e`, and `/mobile-parity`. Start with failing desktop and phone
  tests for selecting the missing Threads radio option.
- Add Threads to the existing card; use Appearance draft/save/discard/error
  coordination. Add pure draft/patch/rebase tests for the new value, including
  preservation of unrelated settings and edits during live updates.
- Revise the startup descriptions and discovery aliases. Author English,
  pt-pt, and zh-cn; generate zh-hk/zh-tw and pseudo. Use `t()` at render time.
- Implement the plan's named E2E flows in the existing startup specs. Extend
  Home-focused Office navigation cases for the new choice. Keep device-memory
  and legacy last-task scenarios as regressions.
- Update `docs/public/tasks-and-workflows.md` using `/docs-maintainer`; retain
  its how-to focus and existing page route. Record rendered phone evidence.

## Out of scope

- A new Settings page or save mechanism, parent demo data, media production,
  topbar normalization, swipe feedback, or new Threads deck functionality.

## Acceptance

- The localized third option is discoverable on desktop and phone, stays a
  draft until saved, can be discarded, and retains a retryable draft on failed
  save without claiming a new durable default.
- Real UI flows prove Home, startup/reload, workspace/Office behavior, explicit
  route protection, and portable persistence with an independent browser
  context while all original defaults remain valid.
- Phone taps reach the existing native deck; settings rows and Save are
  reachable with safe-area clearance and no document overflow. Public help
  matches shipped behavior and all locale checks pass.

## Verification

Dependencies are installed by Task 01. Run each line from the repo root. The
managed E2E commands rebuild production artifacts and isolate test data; use
one project per invocation, sequentially. Do not point tests at parent demos.

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web test components/settings/appearance-settings-state.test.ts components/settings/general-settings.test.tsx components/settings/settings-save-provider.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/startup-page-settings-card.tsx components/settings/general-settings.tsx components/settings/appearance-settings-state.ts e2e/tests/settings/startup-page.spec.ts e2e/tests/settings/mobile-startup-page.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/startup-page.spec.ts tests/task/task-listing-view-preferences.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-startup-page.spec.ts tests/task/mobile-task-listing-display.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/office/sidebar-navigation.spec.ts -- --grep 'Home')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-office-navigation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

New shared browser helpers, if extracted, join the focused eslint invocation.
Confirm test discovery and record pass counts. Inspect a rendered phone
viewport or saved Playwright screenshot of the selected row and Threads
destination; record its artifact path. No separate media seed/capture task is
needed for normal test evidence.

## Files likely touched

- `apps/web/components/settings/startup-page-settings-card.tsx`
- `apps/web/components/settings/general-settings.tsx` (copy wiring only if needed)
- New `apps/web/components/settings/appearance-settings-state.test.ts`
- Existing `apps/web/components/settings/general-settings.test.tsx` if needed
  for save-error or contributor wiring; do not add markup-only snapshot tests
- `apps/web/src/locales/en/settings.json`, `pt-pt/settings.json`,
  `zh-cn/settings.json`, generated `zh-hk/settings.json`, `zh-tw/settings.json`,
  and `pseudo/settings.json`
- `apps/web/lib/settings-discovery/catalog/preferences.ts` only if the existing
  alias key cannot carry the additional search terms
- `apps/web/e2e/tests/settings/startup-page.spec.ts`,
  `mobile-startup-page.spec.ts`, and a shared `startup-page-helpers.ts` if needed
- `apps/web/e2e/tests/office/sidebar-navigation.spec.ts` and
  `mobile-office-navigation.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

[Task 02](task-02-resolve-threads-home-navigation.md).

## Risks

- Existing copy says Home always opens overview and must be revised together
  with the new option. The Task overview description must include remembered
  Threads as well as Kanban/Pipeline/List.
- The startup card is mocked out in `general-settings.test.tsx`; those tests
  cannot alone prove the new radio exists or is touch-usable.
- Worker-scoped settings leak if a test fails before cleanup. Restore captured
  baselines in `afterEach`, and avoid fixture resets in a second browser context.
- New UI strings need every shipped locale; older contradictory prose in
  `docs/i18n.md` does not override current root/scoped AGENTS enforcement.

## Parallelism

`sequential`

## Inputs

- Design **Settings and mobile composition**, **Entry resolution**, and
  **Failure and recovery**; plan **E2E tests** supplies the named scenarios.
- Shipped `StartupPageSettingsCard`, `AppearanceState`, `SettingsSaveProvider`,
  `MobileMenuSheet`, and existing startup-page desktop/mobile tests.
- `/e2e` fixture/cleanup guidance, `/mobile-parity` mobile UI language,
  `/docs-maintainer`, and `apps/web/AGENTS.md`.

## Results

Desktop and phone RED runs both failed on the missing Threads radio before
the card and locale changes. The existing Appearance contributor and shared
Save mechanism need no production changes; the new option uses them directly.

- Draft/patch/rebase, general settings, and Save provider: 35 tests passed.
  The consolidated frontend run passed 272 tests across 21 files, covering
  settings transport, navigation, bootstrap, and existing listing behavior.
- Typecheck and focused ESLint passed. Locale generation, `i18n:check`, and
  `i18n:ratchet` passed for all five languages and pseudo. Unrelated
  pre-existing Traditional Chinese wording was preserved after generation.
- Desktop startup and listing E2E: 8 passed. Real UI Save, Reset, failed Save
  and Retry, independent browser persistence, all Home entry points, workspace
  isolation, explicit destinations, and history passed with existing defaults.
- Phone startup and listing E2E: 5 passed. The native menu, labelled row,
  touch Save, settings/Home reloads, focused task/session route, viewport
  containment, and original last-task and Pipeline fallback flows passed.
- Desktop Office Home E2E: 3 passed, including the shared Appearance page.
- Phone Office E2E: 3 passed after the final rebuild, including Settings Home
  retaining Office priority and switching to a non-Office Threads workspace.
- Public docs validation passed for 46 pages; its validator test passed.
  Specification lint and `git diff --check` passed.

Browser runs used the managed host runner, one worker/project at a time,
strict WS assertions, and `--retries=0`. After the production rebuild, unchanged
artifacts were reused with `--no-build`; `E2E_PORT_OFFSET=19` isolated subsequent
runs from the initial disconnected runtime. A writable temporary `GOCACHE`
was used for backend builds. Empty List assertions target its real empty
state. Phone hit testing waits for the transient update toast to clear without
extending the assertion timeout or changing product layout.

The phone screenshots were visually inspected and copied outside the runner's
cleaned output directory to `/tmp/kandev-threads-home-evidence.CTXlH8/`:

- `threads-home-settings-phone.png`: all three labelled choices, selected
  Threads, and the reachable shared Save action.
- `threads-home-deck-phone.png`: native Threads Home with an empty workspace.
- `threads-home-focused-phone.png`: one full-width conversation preserving
  task and session focus, with no horizontal document overflow.

These are temporary local verification artifacts, not public media. Public
documentation was updated at `docs/public/tasks-and-workflows.md`.

### PR review remediation

The E2E API helper's settings response now declares `workspace_id` and
`workflow_filter_id` as optional strings, matching the existing response and
save contracts. A targeted TypeScript compiler assertion over the startup
spec reproduced TS2322 at both cleanup fields before the declaration fix and
passed afterward. The normal web typecheck excludes E2E files.

`pnpm exec vitest run e2e/helpers/api-client.test.ts` passed both helper tests,
including contract coverage for reading and restoring the startup choice and
workspace scope. Focused ESLint over both helper files passed. This helper
change is test-only; Task 02 records the separate disabled-Office routing fix.

After `make build-web`, the following command from `apps/web` passed all seven
phone scenarios with strict WebSocket checks and retries disabled:

```bash
E2E_PORT_OFFSET=21 pnpm e2e:run --host --no-build --project mobile-chrome tests/office/mobile-office-navigation.spec.ts tests/settings/mobile-startup-page.spec.ts -- --retries=0
```

The new disabled-Office scenario proved brand tap, workspace retention, and
Threads reload. Its `office-disabled-threads-phone.png` screenshot was inspected.
The explicit-destination scenario completed in 11.0 seconds; reducing its causal
session-wait budget would only fail a slow precondition earlier, so no speculative
timeout change was made. Runtime environment is restored in `finally` and saved
preferences remain restored by the suite's existing cleanup.

Public how-to documentation now explicitly names Kanban/Pipeline selections and
disabled-Office Home behavior. All verification blocks use scoped directory
changes. Public-docs validation passed for 46 pages and its validator test passed.
