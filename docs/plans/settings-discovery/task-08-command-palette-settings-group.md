---
id: "08-command-palette-settings-group"
title: "Separate Cmd+K Settings section"
status: completed
wave: 8
depends_on: ["07-capitalized-dynamic-labels"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 08: Separate Cmd+K Settings section

## Root cause

`CommandsListContent` ranks typed matches correctly but renders every command inside one hardcoded
Commands group. Search-only catalog entries therefore lose their Settings information hierarchy
once they become visible.

## Acceptance

- Regular typed matches remain under Commands.
- Search-only settings matches render under their existing localized Settings group.
- A group is omitted when it has no matching results.
- Ranking, secondary context, selection, and exact-target navigation remain unchanged.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/command-panel-content-search.test.tsx`
- `cd apps/web && pnpm e2e:run --host tests/command-panel.spec.ts --grep "settings discovery"`

## Files

- `apps/web/components/command-panel-footer.tsx`
- `apps/web/components/command-panel-content-search.test.tsx`
- `apps/web/e2e/tests/command-panel.spec.ts`

## Mobile parity

The command list renderer is shared by desktop and phone. This change adds no viewport branch,
touch target, overlay, navigation, or scroll-owner behavior; the existing shared component contract
is sufficient for this presentation-only hierarchy correction.

## Parallelism

Sequential; the regression and implementation modify the same command-list contract.

## Results

- RED: 2 focused component assertions failed because Settings results rendered under Commands.
- RED E2E: the fresh production build exposed `Commands` as the sole typed-result heading.
- GREEN: regular matches render under localized Commands while search-only matches retain their
  localized group; empty groups are omitted and ranked order is preserved within each group.
- Focused component tests passed (14/14), and the fresh-build browser test passed (1/1).
- Typecheck, focused ESLint, i18n checks, the new-code i18n ratchet, and `git diff --check` passed.
- The isolated local and Tailscale Settings URLs returned HTTP 200 and served the updated module;
  the main Kandev process on `:9998` remained untouched.
- No separate mobile E2E was needed because the same command-list component renders at every
  viewport and no touch, overlay, navigation, or scrolling behavior changed.
