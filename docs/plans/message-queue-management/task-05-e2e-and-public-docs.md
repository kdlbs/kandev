---
id: "05-e2e-and-public-docs"
title: "Prove end-to-end behavior and update public docs"
status: completed
wave: 4
depends_on:
  - "03-visible-queue-removal-ui"
  - "04-message-queue-settings-ui"
plan: "plan.md"
spec: "../../specs/ui/message-queue-management.md"
---

# Task 05: Prove End-to-End Behavior and Update Public Docs

## Acceptance

- Desktop Playwright creates agent-origin queued messages through the mock MCP
  path, removes one, clears the remainder, and proves capacity is available.
- Mobile Playwright performs individual remove and clear-all through touch-sized
  controls while the composer remains visible, the queue scrolls internally,
  and displayed positions compact after a middle-row removal.
- Desktop and mobile General Settings navigation reach Message Queue; an admin
  change saves and reloads, while environment/member states remain read-only.
- Any test changing the install-wide cap captures and restores its baseline in
  teardown, including failure paths.
- Public coordination, configuration, sessions/review, and operations docs
  explain visible-origin removal, UI configuration, environment precedence,
  `0` unlimited, live application, and non-pruning.

## TDD Sequence

1. Extend the cross-task message fixture to hold a target busy while multiple
   sender tasks queue agent-origin rows. Add desktop remove/clear assertions and
   run RED.
2. Add a mobile management scenario based on the shipped queue-scroll helper;
   assert coarse-pointer hit boxes, one scroll owner, and visible composer. Run
   RED.
3. Add settings navigation/save/reload scenarios with baseline restoration and
   run RED.
4. Complete any missing wiring discovered by E2E, rerun all focused scenarios,
   and record GREEN.
5. Update public docs and run both documentation validators.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/chat/message-queue.spec.ts tests/chat/agent-message-attribution.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts
pnpm e2e:run tests/system/message-queue-settings.spec.ts
pnpm e2e:run --project mobile-chrome tests/system/mobile-message-queue-settings.spec.ts
cd ../../
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files Likely Touched

- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/agent-message-attribution.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/e2e/tests/system/message-queue-settings.spec.ts`
- `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`
- shared E2E API/page helpers only if needed
- `docs/public/coordination.md`
- `docs/public/configuration.md`
- `docs/public/sessions-and-review.md`
- `docs/public/operations.md`

## Dependencies

Tasks 03 and 04 must be complete so browser tests exercise production UI
rather than mocked components.

## Parallelism

Sequential final integration task. It mutates shared install-wide E2E state and
must own its setup/restore lifecycle.

## Output Contract

Report desktop/mobile scenario results, baseline restoration evidence, public
docs validation, changed files, and any residual flake or release risk. Update
this task, `plan.md`, and spec status when the feature is genuinely complete.

## Results

- GREEN: 19 desktop queue tests. The new mock-MCP scenario filled a busy
  target with ten agent-origin rows from two sender tasks, removed one, cleared
  the other nine, then admitted a fresh agent message.
- GREEN: one mobile queue-management test. The queue retained its single
  internal scroll owner and visible composer; row removal, clear-all, and close
  controls measured at least 44 by 44 CSS pixels.
- GREEN: three desktop settings tests and one mobile settings test. Desktop
  navigation, admin save/reload, environment lock, and member read-only states
  passed; mobile navigation and the shared single-scroll layout passed.
- The settings suite captures the install-wide baseline with GET before every
  test and restores it with PATCH in `afterEach`, so restoration also runs on a
  failed assertion.
- GREEN: 58 public-doc validator tests, 41 published pages validated, and
  `git diff --check`.
- GREEN: web lint, typecheck, `i18n:check`, and `i18n:ratchet` after the E2E and
  documentation changes.
- No additional production wiring gap appeared in the final browser wave. A
  subsequent spec audit found and fixed invalid persisted-setting replacement
  and overlapping-save ordering; the focused race-enabled backend suite is
  green. The browser tests use the shipped mock agent; repository races remain
  covered by Tasks 01 and 02.
- Follow-up RED: mobile middle-row removal displayed `#6` in the fifth visible
  slot, while desktop/mobile settings tests could not find Message Queue under
  General.
- Follow-up GREEN: three desktop General settings tests and two mobile
  queue/settings tests pass with retries disabled. Middle-row removal compacts
  the final visible label to `#9`; desktop and mobile navigation use General.
- Follow-up GREEN: 113 focused frontend tests, typecheck, lint, both i18n gates,
  58 public-doc tests, the 41-page docs validator, and `git diff --check` pass.
- The retained isolated evaluation target has four agent-origin rows with
  durable positions `4–7`; the live feature UI displays compact positions
  `#1–#4`. General navigation, the legacy redirect, and browser console are
  clean.
- PR fixup GREEN: synced current `main`; preserved raw SQLite metadata guards;
  made ordinary accepted-message retries bypass a lowered cap; corrected the
  General route, redirect coverage, drain compaction, and environment
  precedence docs; and scoped strict Playwright locators. Focused Go tests, 40
  frontend unit tests, typecheck, both i18n gates, 58 public-doc tests, the
  exact desktop queue scenario, and both mobile queue/settings scenarios pass.
