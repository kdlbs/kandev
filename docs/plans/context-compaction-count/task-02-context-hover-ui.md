---
id: "02-context-hover-ui"
title: "Show compactions in context hover"
status: done
wave: 2
depends_on: ["01-persist-compaction-count"]
plan: "plan.md"
spec: "../../specs/context-compaction-count/spec.md"
---

# Task 02: Show compactions in context hover

## Intent

Carry the persisted count through live and hydrated frontend state, then show it with an accessible, translated accuracy disclosure in the existing context hover on every viewport.

## Acceptance

- Live session patches and page hydration populate `ContextWindowEntry.compactionCount`, treating absent, invalid, or negative legacy values as zero.
- The existing context hover shows zero and non-zero counts and explains that Kandev infers them from token drops, so missing samples or provider resets can make the value approximate.
- The shared inline help remains accessible by pointer, keyboard, and touch without nesting another tooltip or changing the existing desktop/mobile composition.

## TDD sequence

1. Add parser, live-handler, hydration-hook, and rendered-hover cases; run them and confirm count expectations fail.
2. Add English i18n keys, regenerate the pseudo locale, and implement the smallest state/hook/UI changes.
3. Rerun the exact tests, typecheck, and i18n checks; refactor only where needed to keep the component within repository limits.

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/context-window.ts`
- `apps/web/lib/state/slices/session-runtime/context-window.test.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/hooks/domains/session/use-session-context-window.ts`
- `apps/web/hooks/domains/session/use-session-context-window.test.ts`
- `apps/web/components/task/chat/token-usage-display.tsx`
- `apps/web/components/task/chat/token-usage-display.test.tsx`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

Task 01, which defines the persisted and live metadata field.

## Parallelism

`sequential` — the UI consumes the backend metadata contract and shares its state/parser files with all hover behavior.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/session-runtime/context-window.test.ts lib/ws/handlers/agent-session.test.ts hooks/domains/session/use-session-context-window.test.ts components/task/chat/token-usage-display.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web run i18n:check`

## Inputs

- Spec `What`, `API surface`, and the final scenario.
- Frontend state flow and i18n rules in `apps/web/AGENTS.md`.
- Mobile content-only guidance in `.agents/skills/mobile-parity/SKILL.md`.
- Existing source disclosure and pinnable parent hover in `apps/web/components/task/chat/token-usage-display.tsx`.

## Output contract

Report the result, actual files changed, exact tests and counts, translated keys, rendered-check status, blockers, risks, and synchronized task/plan status in this conversation.

## Results

- RED: the rendered-hover case failed before the count row existed; parser/live-handler tests also established the missing-field baseline.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/session-runtime/context-window.test.ts lib/ws/handlers/agent-session.test.ts hooks/domains/session/use-session-context-window.test.ts components/task/chat/token-usage-display.test.tsx` — 4 files passed, 68 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web run i18n:check` — passed; English and pseudo locale contain the three new keys.
- `cd apps && pnpm --filter @kandev/web run i18n:ratchet` — passed.
- Public docs impact: updated `docs/public/sessions-and-review.md` with the inferred-count limitation; public-doc validation passed.
