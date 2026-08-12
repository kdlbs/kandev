---
id: "01-collapsible-section"
title: "Make the section collapsible"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/available-to-install-collapsible.md"
---

# Task 01: Make the section collapsible

## Acceptance

- The "Available to Install" heading row on `/settings/agents` is a keyboard-accessible toggle (native `button` semantics, Radix-managed `aria-expanded`) with a chevron that rotates when expanded.
- The section renders expanded by default; clicking the heading row hides the card grid; clicking again restores it.
- Collapsing does not affect install streaming or the success rescan; no new i18n strings; `pnpm run i18n:ratchet` stays green.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx` (only `SuggestInstallSection` and its imports)

## Inputs

- Spec: Scenarios 1-4.
- Plan: **Frontend** section and **Mobile design contract**.
- Patterns: `apps/web/components/office/agents/[id]/agent-wake-reason-overrides.tsx` and the office run-detail collapsibles (`apps/web/components/office/agents/[id]/runs/[runId]/components/invocation-panel.tsx`) for the `@kandev/ui/collapsible` + rotating chevron shape; `apps/web/components/app-sidebar/app-sidebar-section.tsx` for the header-as-trigger pattern.

## TDD sequence

1. Write the desktop E2E scenario (task 02) first and confirm it fails because `available-to-install-trigger` does not exist.
2. Implement the collapsible in `SuggestInstallSection`.
3. Run the desktop E2E spec; confirm it passes.

## Verification

- `cd apps && pnpm install --frozen-lockfile` (fresh-worktree bootstrap; skip when dependencies are already installed)
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:ratchet`
- `cd apps/web && pnpm e2e:run tests/settings/available-to-install-collapsible.spec.ts`

## Dependencies

- None

## Output contract

Summary, files changed, tests run with exact commands and outcomes, blockers, risks, and this task file updated to `done` with a `## Results` section; synchronize `plan.md`'s Wave 1 checkbox and Verification Results.

## Results

- Red first: `pnpm e2e:run --host tests/settings/available-to-install-collapsible.spec.ts` failed as expected on `getByTestId('available-to-install-trigger')` → element not found (before the implementation existed).
- Implemented the collapsible in `SuggestInstallSection` (`apps/web/app/settings/agents/page.tsx`): `Collapsible`/`CollapsibleTrigger asChild`/`CollapsibleContent` from `@kandev/ui/collapsible`, header row as the trigger button with rotating `IconChevronDown`, `useState(true)` default-expanded, `pt-4` grid spacing restored inside `CollapsibleContent`.
- Green: `pnpm e2e:run --host tests/settings/available-to-install-collapsible.spec.ts` → `1 passed (6.6s)`.
- `cd apps/web && pnpm run typecheck` → clean.
- `cd apps/web && pnpm run i18n:ratchet` → `0 added + 1 modified file(s) clean` (no new strings).
- `make fmt` and `make lint-web` → clean.
- Security/trust boundaries: none (presentational change).
