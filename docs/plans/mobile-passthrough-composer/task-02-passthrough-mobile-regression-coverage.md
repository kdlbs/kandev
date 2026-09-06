---
id: "02-passthrough-mobile-regression-coverage"
title: "Passthrough mobile regression coverage"
status: completed
wave: 2
depends_on: ["01-touch-safe-mobile-controls"]
plan: "plan.md"
spec: "../../specs/cli/requirements/mobile-passthrough-composer.md"
design: "../../specs/cli/system-design/mobile-passthrough-composer.md"
---

# Task 02: Passthrough mobile regression coverage

## Outcome

The mobile browser suite proves the supported passthrough composer flows and
the repository has a concrete real-device sign-off checklist.

## Scope

- Extend the existing mobile passthrough Playwright specification instead of
  creating a parallel fixture or page model.
- Cover computed touch geometry, composer focus, explicit send, literal slash
  text, `@` selection, operating-system file selection, session-scoped draft
  restoration, visual-viewport containment, and document overflow.
- Record iPhone Safari and Android Chrome smoke results before the issue closes.

## Exclusions

- No ACP slash-command suggestions in passthrough mode.
- No one-tap raw terminal-key controls. Create a follow-up issue if that
  capability is still required after the real-device smoke test.
- No changes to the companion remote-access documentation tracked by `#2807`.

## Traceability

- `REQ-CLI-MOBILE-PASSTHROUGH-COMPOSER-001`
- `AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.1` through
  `AC-CLI-MOBILE-PASSTHROUGH-COMPOSER-001.8`
- `docs/specs/cli/system-design/mobile-passthrough-composer.md`

## Implementation acceptance

- The mobile Chromium specification covers all automated flows in the issue
  matrix and fails if a target becomes smaller than 44-by-44 CSS pixels.
- Draft restoration is proven across two sessions, including text, selected
  context, and a ready attachment.
- The implementation record contains iPhone Safari and Android Chrome smoke
  results or an explicit blocker that prevents issue closure.

## Files likely touched

- `apps/web/e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts`
- `docs/plans/mobile-passthrough-composer/plan.md`
- `docs/plans/mobile-passthrough-composer/task-02-passthrough-mobile-regression-coverage.md`

## Verification

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts)
python3 scripts/lint-spec-files.py --all
```

## Results

The mobile passthrough specification now covers touch geometry, composer focus,
explicit single-message send, literal slash text, inline `@` prompt selection,
visual-viewport containment, document overflow, operating-system file
selection, and session-scoped draft restoration with text, context, and a ready
attachment.

Verification passed:

- `pnpm e2e:run --project mobile-chrome e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts`: 5 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.

Physical-device smoke is blocked in this environment: no iPhone Safari or
Android Chrome device runner is available. Issue closure still requires the
manual keyboard, `@` selection, file picker, task-switch, and explicit-send
smoke matrix on those devices.
