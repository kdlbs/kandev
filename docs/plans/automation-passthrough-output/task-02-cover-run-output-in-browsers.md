---
id: "02-cover-run-output-in-browsers"
title: "Prove persisted run guidance on desktop and phone"
status: done
wave: 2
depends_on:
  - 01-explain-passthrough-output
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
  - REQ-UI-EMPTY-TURN-NOTICE-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.13
  - AC-OFFICE-AUTOMATION-RUNS-001.14
  - AC-OFFICE-AUTOMATION-RUNS-001.15
  - AC-OFFICE-AUTOMATION-RUNS-001.16
  - AC-OFFICE-AUTOMATION-RUNS-001.17
  - AC-OFFICE-AUTOMATION-RUNS-001.18
  - AC-UI-EMPTY-TURN-NOTICE-001.11
system_design:
  - ../../specs/office/system-design/automation-run-output.md
  - ../../specs/office/system-design/automation-runs.md
  - ../../specs/ui/system-design/empty-turn-notice.md
---

# Task 02: Prove persisted run guidance on desktop and phone

## Summary

Add browser regressions for the capture guidance using persisted hidden-run
fixtures. Prove direct load, reload, run selection, and phone reading behavior
against a fresh managed production build.

## In scope

- Extend the existing mock-only session seed endpoint with optional `is_passthrough`.
  Preserve false as the default and the existing primary-session behavior.
- Extend `seedAutomationTaskSession` with an optional fourth options argument.
  Existing third-argument `isPrimary` callers must remain compatible.
- Cover fixture persistence with a focused backend handler test.
- Add shared API setup and focused desktop/Pixel 5 Playwright tests.
- Record rendered comparison against UI-01 and UI-02.

## Out of scope

Production session APIs, database schema, live providers, PTY startup, terminal
streaming, folder trust, and changing automation execution behavior.

## Acceptance

1. The fixture's omitted, true, and explicit false modes round-trip through
   stored sessions. Direct URLs and reloads show guidance only for passthrough.
2. Desktop and phone run selection preserve status, stored prompts/errors, and
   the composer. A structured run remains readable without capture guidance.
3. Phone guidance wraps inside the full-height reader, and the existing runs
   drawer remains usable by touch. Reading triggers no terminal/start/resume request.

## Implementation and TDD

1. Add `TestE2ECreateTaskSessionPassthrough` in a separate backend test file.
   Exercise omitted, true, and false request fields and persisted session reads.
   Run it RED before the fixture change, then add the optional seed field.
2. Extend the API helper and use it to seed two hidden runs with different
   session modes. Seed real stored prompts and genuine error/status entries.
   Persist `origin=automation_run` through the existing E2E task-origin path.
3. Add `automations-passthrough-output.spec.ts` and
   `mobile-automations-passthrough-output.spec.ts`. Keep shared setup in
   `e2e/helpers/automation-passthrough-output.ts` if both specs need it.
4. Open the selected run directly, assert guidance, reload, and select a
   structured sibling. Select passthrough again and assert guidance returns.
   Include running and parked fixture states without launching a provider.
5. Assert the full guidance text and future-session advice. Preserve stored
   messages, genuine errors, status, and the reply composer. Unit/handler tests
   from task 01 own completion-event suppression and the pre-hydration race.
6. On Pixel 5, use `.tap()` to select a run through the existing drawer.
   Assert dismissal/focus behavior, long localized guidance, visible composer,
   and zero document horizontal overflow.
7. Arm socket/request observers before navigation. Wait for session hydration
   causally before absence assertions. Use `dwell` with category
   `negative-assertion` for a bounded observation window and state its reason.
   Assert no agent-terminal socket and no automatic start/resume request.

The component and real-store tests provide the production RED gate. These
browser tests follow the integrated correction because deterministic persisted
mode fixtures need the seed expansion. Record that distinction in Results.
Do not present a missing test ID as a behavioral RED result.

## ASCII UI preview

UI-01 and UI-02 excerpt from [the combined preview](plan.md#ascii-ui-preview):

```text
DESKTOP                       PHONE
+----------+--------------+   +-------------------------+
| Runs     | Explanation  |   | Run status       [Runs] |
| rail     | Future advice|   | Explanation             |
|          | Messages     |   | Future-session advice   |
|          | Composer     |   | Messages                |
+----------+--------------+   | Composer                |
                             +-------------------------+
```

For Office .13 through .18, verify that the explanation precedes the existing
message scroller. The phone retains one focused reader and its existing drawer.
Spacing is illustrative. Use localized copy and preserve safe-area behavior.

## Verification

Run from the repository root, after task 01 passes. Reuse its dependency install.

```bash
(cd apps/backend && go test -trimpath ./internal/backendapp \
  -run '^TestE2ECreateTaskSessionPassthrough$' -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/backendapp/e2e_reset.go \
  apps/backend/internal/backendapp/e2e_task_session_passthrough_test.go
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  e2e/helpers/api-client.ts e2e/helpers/automation-passthrough-output.ts \
  e2e/tests/automations-passthrough-output.spec.ts \
  e2e/tests/mobile-automations-passthrough-output.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/automations-passthrough-output.spec.ts tests/automations-run-detail.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/mobile-automations-passthrough-output.spec.ts tests/mobile-automation-detail.spec.ts)
git diff --check
```

The managed runner builds the current backend and web, limits resources, and
cleans up. Run the two project commands sequentially. Do not use `--no-build`
after source changes or add all-worker overrides. If the optional shared helper
is unnecessary, remove its lint path here and record the final command.

Run the [plan's documentation gates and coverage preflight](plan.md#verification-results)
with actual changed paths before the package becomes implemented.
Record exact results and screenshots or rendered observations. Report any
environment blocker without marking browser verification complete.

## Files likely touched

- `apps/backend/internal/backendapp/e2e_reset.go`
- `apps/backend/internal/backendapp/e2e_task_session_passthrough_test.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/helpers/automation-passthrough-output.ts`
- `apps/web/e2e/tests/automations-passthrough-output.spec.ts`
- `apps/web/e2e/tests/mobile-automations-passthrough-output.spec.ts`

## Dependencies

[Task 01](task-01-explain-passthrough-output.md).

## Risks

Client-only mode injection cannot prove reload behavior. A normal visible task
can hide the missing-boot-row condition, so seed hidden origin explicitly.
Never mount a terminal as a fixture readiness shortcut. The fixture does not
prove a real provider's turn lifecycle, which remains outside this package.

## Parallelism

`sequential`

## Inputs

- [Office output design](../../specs/office/system-design/automation-run-output.md).
- Existing desktop `automations-run-detail.spec.ts` and phone
  `mobile-automation-detail.spec.ts` seed patterns.
- Existing `handleE2ECreateTaskSession`, task-origin seed path, and API helper.
- Repository backend/frontend guidance and `/e2e` and `/mobile-parity` skills.

## Results

The E2E task-session fixture now round-trips omitted, true, and explicit false
passthrough modes. `seedAutomationTaskSession` retains its existing third
`isPrimary` argument and accepts mode as its fourth argument. Tasks are tagged
with persisted `automation_run` origin before direct navigation.

The focused backend fixture test passed. Desktop Chromium passed 9 tests and
Pixel 5 mobile passed 8 tests, including the new run-output regressions and the
existing run-detail suites. Both managed runs built the backend and production
Vite assets. The browser scenarios verified persisted mode on direct load and
reload, switching among running passthrough, parked passthrough, and structured
runs, stored prompts and genuine errors, run status, and the reply composer.
The phone drawer closed after touch selection, its guidance wrapped within the
reader, the composer remained inside the viewport, and the document had no
horizontal overflow. Captured desktop and phone views show the guidance before
the conversation. No terminal WebSocket or automatic session launch, resume,
or restart request appeared during passive reading.

The final focused frontend suite passed 171 tests across seven files. Web
typecheck and focused Oxlint passed. The backend test and changed-Go lint passed.
Localization, public documentation, specification, documentation coverage, and
whitespace checks passed as recorded in the parent plan.
