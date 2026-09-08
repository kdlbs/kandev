---
id: "02-agent-launch-e2e"
title: "Cover agent launch flows"
status: done
wave: 2
depends_on: ["01-reuse-shared-composer"]
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-launch-prompt-composer.md"
---

# Task 02: Cover agent launch flows

## Acceptance

- Desktop E2E proves saved-prompt keyboard selection inserts content without launching and explicit Start Agent creates and activates the second session.
- Pixel 5 E2E proves the mobile session entry, touch selection, and explicit launch deliver the same outcome without viewport escape or document horizontal overflow.
- Test-created prompts are cleaned up even after failure, and the page object uses stable test IDs for the shared composer and mobile entry.

## Verification

Follow E2E TDD: add the desktop regression first and observe the current dialog fail to open the mention menu. After task 01 is integrated, make desktop GREEN, then add and run the mobile scenario against a fresh production build.

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/session/new-session-dialog.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts
```

Confirm the runner discovers the intended desktop and mobile tests before recording a pass. Record screenshots or traces only when produced by the focused runs; do not add a separate broad verification pass.

## Files likely touched

- `apps/web/e2e/tests/session/new-session-dialog.spec.ts`
- `apps/web/e2e/tests/session/mobile-new-session-dialog.spec.ts`
- `apps/web/e2e/pages/session-page.ts`

## Dependencies

- Task 01 must be complete and its focused unit/type/i18n checks must pass.

## Parallelism

Sequential. These scenarios depend on the shared composer integration and share the New Agent page object.

## Inputs

- Spec scenarios: desktop saved-prompt selection, non-submitting Enter, phone parity, and explicit launch.
- Plan: E2E Tests and Mobile design contract.
- Existing patterns: `apps/web/e2e/tests/task/task-create-prompt-autocomplete.spec.ts`, `apps/web/e2e/tests/session/new-session-dialog.spec.ts`, and `apps/web/e2e/tests/session/mobile-handoff.spec.ts`.

## Output contract

Report RED and GREEN commands, discovered/passing test counts, viewport and overflow assertions, any generated screenshots/traces, prompt cleanup evidence, changed files, blockers, risks, and synchronized task/plan status.

## Results

- RED evidence: before the integration, the New Agent prompt was a plain textarea without the shared saved-prompt mention or voice wiring; the new desktop scenario targets that missing behavior.
- GREEN desktop: `cd apps/web && pnpm e2e:run tests/session/new-session-dialog.spec.ts` — 6 tests passed. The new scenario selected a saved prompt with Enter, verified the dialog stayed open and session count stayed at one, then launched explicitly and observed session two.
- GREEN mobile: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts` — 1 test passed. The phone entry, touch mention selection, viewport bounds, no-horizontal-overflow assertion, and explicit launch all passed.
- Changed E2E surfaces: `session-page.ts`, `new-session-dialog.spec.ts`, and `mobile-new-session-dialog.spec.ts`; the page object now targets `task-description-input` and exposes the mobile launch entry.
- Cleanup: both focused specs delete their test-created saved prompt in `afterEach`; no screenshots or traces were produced for check-in.
