---
id: "04-e2e-and-verification"
title: "E2E and verification"
status: done
wave: 3
depends_on: ["02-unified-frontend-sessions", "03-clarification-responsive-ux"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 04: E2E and Verification

## Acceptance

- Desktop E2E covers config launch, large modal, send/receive, refresh restoration, continuation,
  clarification context/controls, and config deletion.
- Mobile E2E covers full-screen launch, config indicator, clarification controls, and unclipped
  primary actions; ordinary repository-backed Quick Chat remains green on desktop and mobile.
- Required formatting, typecheck, test, and lint commands pass in order and exact results are recorded.

## Verification

- `cd apps/web && pnpm e2e:run e2e/tests/settings/config-chat-popover.spec.ts`
  - Passed: 3 desktop tests (Settings launch/restore/continue/delete, Command Palette launch, and
    inline clarification context/collapse/answer).

- `cd apps/web && pnpm e2e:run e2e/tests/settings/mobile-configuration-chat.spec.ts -- --project=mobile-chrome`
  - Passed: 1 mobile test (full-screen typed config tab and collapsible clarification answer).

- `cd apps/web && pnpm e2e:run e2e/tests/chat/clarification-resize.spec.ts e2e/tests/chat/quick-chat.spec.ts e2e/tests/chat/mobile-quick-chat-repository.spec.ts`
  - Passed: 9 desktop tests. The mobile repository spec is project-gated and was run separately.

- `cd apps/web && pnpm e2e:run e2e/tests/chat/mobile-quick-chat-repository.spec.ts -- --project=mobile-chrome`
  - Passed: 1 mobile repository-context regression test.

- `make fmt`
  - Passed across backend, web, CLI, and packages.
- `make typecheck`
  - Passed across all applications; rerun after lint corrections and passed.
- `make test`
  - Passed across the full repository.
- `make lint`
  - Initial run reported nine changed-file warnings. After extracting the config setup footer,
    splitting the hydration test group, and deduplicating test fixtures, the full rerun passed with
    zero backend issues, zero web warnings, and all 120 harness files passing.

Additional focused verification:

- 12 affected Vitest files: 112 tests passed.
- Six lint-correction Vitest files: 65 tests passed.
- `git diff --check`: passed.

## Files Likely Touched

- `apps/web/e2e/tests/settings/config-chat-popover.spec.ts`
- `apps/web/e2e/tests/settings/mobile-configuration-chat.spec.ts`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-repository.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` if a missing lifecycle helper is required.

## Inputs

- All acceptance scenarios in the spec.
- Tasks 01-03 implementation and targeted tests.
- Existing E2E fixtures and managed runner conventions.

## Dependencies

Tasks 02 and 03.

## Output Contract

Report exact E2E and repository verification commands/results, files touched, blockers and artifacts,
then mark this task `done` in this file and `plan.md`.
