---
id: "02-subagent-status-e2e"
title: "Assert settled subagent header in desktop and mobile E2E"
status: done
wave: 2
depends_on: ["01-header-status-ui"]
plan: "plan.md"
spec: "../../specs/ui/subagent-observability.md"
---

# Task 02: Assert settled subagent header in desktop and mobile E2E

## Intent

Lock the successful-completion header in the existing `/e2e:subagent` flow on desktop and phone. Do not add a mock-agent scenario: the live window is ~150ms, and active/failed/cancelled are covered by task 01 unit tests.

## Acceptance

- After chat idle on `/e2e:subagent`, the single `subagent-card` has no `role=status` named Loading, no `Working...` text, and one `Completed` label. Existing type, description, and metadata-chip assertions stay.
- The same settled-header assertions run on `mobile-chrome` via `mobile-subagent.spec.ts`.
- No new mock-agent scenario, no new i18n keys, no new Playwright file.

## Files likely touched

- `apps/web/e2e/tests/chat/subagent.spec.ts`
- `apps/web/e2e/tests/chat/mobile-subagent.spec.ts`

## Dependencies

Task 01. The settled header (no Working, no spinner) is the post-change contract; these specs already wait for chat idle and the metadata row.

## Parallelism

Sequential after task 01. Both files assert the same settled card.

## Inputs

- Spec: successful-completion header-status scenario.
- Plan: E2E Tests.
- Existing `/e2e:subagent` fixture and `SessionPage` helpers in both specs.
- After idle, `scenarioSubagent` has already called `completeSubagentTool`; do not race the brief start window.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --host --project chromium -- tests/chat/subagent.spec.ts && pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-subagent.spec.ts
```

`pnpm e2e:run` rebuilds the production Vite bundle. Do not run against a stale `apps/web/dist`.

## Output contract

Summary, files changed, tests run, blockers, risks, and this task plus `plan.md` status/results updated in the same conversation.

## Results

Done. After metadata is visible, both specs assert the card has no Loading status, no `Working...`, and one `Completed` label.

- Desktop: `pnpm e2e:run --host --project chromium -- tests/chat/subagent.spec.ts` — 1 passed (19.7s).
- Mobile: `pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-subagent.spec.ts` — 1 passed (14.4s).
