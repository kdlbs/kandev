---
id: "04-mobile-accessibility-and-tooling"
title: "Finish mobile and tooling details"
status: done
wave: 4
depends_on: ["03-dedup-recovery-tests"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-pr-merged-trigger.md"
---

# Task 04: Finish Mobile and Tooling Details

## Intent

Complete the responsive editor proof, fix the branch-field accessible name, and remove the unsupported
Node compatibility code added by the PR.

## Acceptance

- The base-branch label is programmatically associated with its input and remains localized.
- Mobile Chrome can select the trigger, edit its controls, save, reopen, and observe persisted state.
- The mobile page has no horizontal overflow and controls stay visible/touch-operable.
- `guard-allowlist.mjs` uses the Node 24 `globSync` path directly; the handwritten Node <22 fallback
  and its private glob implementation are removed.
- Existing allowlist behavior remains covered by the current focused test.

## TDD sequence

1. Add `mobile-automations-pr-merged-trigger.spec.ts` using the existing mobile automations scroll
   flow as its fixture and interaction baseline; confirm the missing coverage/failing accessible-name
   assertion.
2. Add a stable `id`/`htmlFor` pair to the shared config component and exercise the save/reopen flow
   with touch interactions.
3. Delete the compatibility fallback and restore the direct `fsImpl.globSync(entry, { cwd })` call.
4. Run the focused Vitest, typecheck, and mobile E2E.

## Files likely touched

- `apps/web/components/automations/trigger-configs/github-pr-merged-config.tsx`
- `apps/web/e2e/tests/mobile-automations-pr-merged-trigger.spec.ts`
- `apps/web/scripts/lib/guard-allowlist.mjs`
- `apps/web/scripts/lib/guard-allowlist.test.ts` only if a behavior assertion needs clarification

## Dependencies

Task 03, so the editor E2E runs against the finalized backend behavior.

## Parallelism

`sequential` — the mobile E2E and shared component must move together.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- scripts/lib/guard-allowlist.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-automations-pr-merged-trigger.spec.ts`

## Risks

- Use the existing settings scroll container; do not add document-level scrolling or a mobile-only
  editor state.
- Clean up the created automation even when assertions fail so state does not leak across E2E runs.

## Completed validation

- GREEN: `cd apps && pnpm --filter @kandev/web test -- scripts/lib/guard-allowlist.test.ts` (17 tests).
- GREEN: `cd apps/web && pnpm run typecheck`.
- GREEN: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-automations-pr-merged-trigger.spec.ts`.
- GREEN: the desktop merged-PR E2E covers the shared label/input association and the cleared-repository
  save race; the full spec passes 15 tests.
- The Node 24 `globSync` path is now direct; the private Node <22 fallback was removed.

## Output contract

Report mobile scenario coverage, accessibility evidence, removed fallback scope, command results, and
task/plan status updates.
