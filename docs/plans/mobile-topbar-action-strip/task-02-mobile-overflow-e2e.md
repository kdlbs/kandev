---
id: "02-mobile-overflow-e2e"
title: "Prove mobile overflow behavior"
status: done
wave: 2
depends_on: ["01-mobile-action-strip"]
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-quick-chat-topbar.md"
---

# Task 02: Prove mobile overflow behavior

## Intent

Prove the phone header with real metrics and plugin content. Replace the obsolete 44px launcher
expectation with relational geometry while preserving the complete terminal flow.

## Acceptance

- Pixel 5 coverage proves the Kandev link and menu stay fixed while middle actions scroll.
- The test proves native control equality, metric and plugin icon sizing, directional fades, and no
  document horizontal overflow.
- A scrolled action remains operable. Existing Quick Terminal dialog and focus behavior still pass.
- The test restores user settings, uninstalls the plugin fixture, and leaves no test-owned state.

## Files likely touched

- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`

## Dependencies

- Task 01 must be complete.

## Parallelism

`sequential`. This task verifies Task 01 against the production build.

## Inputs

- Spec scenarios for equal sizing, fixed edges, directional fades, and contained overflow.
- Plan section `E2E Tests`.
- Plugin install and cleanup pattern in `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts`.
- User-settings cleanup rules in `.agents/skills/e2e/references/ui-state-and-cleanup.md` and
  `apps/web/AGENTS.md`.

## TDD sequence

1. Add the mobile plugin topbar scenario and update the old terminal geometry assertion.
2. Run the focused specs against the current implementation and record the expected failures.
3. Complete any minimal selector or geometry correction required by the specification.
4. Run both focused specs against a fresh production build with retries disabled.

## Verification

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-topbar.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --no-build --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0)
```

## Output contract

Report test discovery counts, RED and GREEN results, artifact paths, cleanup evidence, blockers, and
risks. Update this task and `plan.md` in the same conversation.

## Results

- Added `mobile-plugin-topbar.spec.ts` to the existing `mobile-chrome` Pixel 5 project.
- The test installs the real fixture package, enables real metrics, checks native and plugin
  geometry, verifies the actual 44px terminal/chat hit areas, all directional fade states, scroll
  reachability, fixed edge positions, and document overflow, then restores settings and removes the
  fixture.
- Updated the existing quick-terminal test to compare launcher geometry with the fixed menu instead
  of requiring the obsolete 44px launcher size.
- Initial runner attempt caught a duplicate local variable in the edited test and was corrected
  before the production run.
- Managed production E2E: 1 plugin topbar test passed with retries disabled.
- Managed no-build regression E2E: 1 quick-terminal test passed with retries disabled.
- Follow-up production-build E2E: 1 plugin topbar test passed with actual 44px hit-target checks;
  the quick-terminal regression also passed against that build with no rebuild.
- No PR screenshot artifacts were created by these test runs.
