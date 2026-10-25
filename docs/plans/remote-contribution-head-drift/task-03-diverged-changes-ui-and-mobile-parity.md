---
id: "03-diverged-changes-ui-and-mobile-parity"
title: "Separate diverged histories across responsive UI"
status: done
wave: 3
depends_on: ["02-checkout-relation-and-action-semantics"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 03: Separate Diverged Histories Across Responsive UI

Replace the misleading flat commit list with explicit provider and checkout histories when the shared
relation model reports divergence. Apply the same remote-action policy to desktop and mobile controls.

## TDD sequence

1. Add Changes-panel component tests with old local SHAs and rewritten provider SHAs. Assert separate
   headings, one warning, correct row provenance, no flat 21-commit interpretation, and no green
   unpushed marker on superseded contributor commits.
2. Add `VcsSplitButton` tests proving Push/Pull counts use upstream values, a contribution remote is a
   valid upstream, base divergence pills stay unchanged, and diverged remote actions are disabled.
3. Add focused mobile Git menu tests proving the same actions are disabled while Commit remains
   available.
4. Implement the responsive presentation, accessible state, translations, and action wiring.

## Changes panel implementation

- Pass the relation from `changes-panel-data.tsx` into `ChangesPanelBody` and timeline props.
- Keep `mergeCommits` unchanged for non-diverged flows. In diverged mode, construct two lists instead:
  provider commits in provider order and local checkout commits in local log order.
- Add an explicit commit provenance/presentation field rather than overloading `pushed: boolean` for
  diverged local rows. `CommitRow` must announce “current PR commit” or “local checkout commit” through
  translated accessible text.
- Render a compact inline warning above the two lists. It explains that the contributor changed the PR
  history, local work was preserved, and reconciliation is required before remote mutation.
- Provider rows may open provider commit details but expose no local amend/revert/reset menu. Local rows
  retain their current local recovery actions.
- Do not change the PR Files, staged, unstaged, review progress, or single-scroll-owner layout.
- If provider commits fail to load, show the error state and keep the non-diverged/unknown presentation;
  never show the force-push warning from an empty array.

## Desktop and mobile controls

- Update `VcsSplitButton` primary action and dropdown to consume `pushAhead`, `pullBehind`, and the
  relation capabilities. Do not equate “has upstream” with `origin/<current-branch>`.
- Keep base `ahead`/`behind` only in the existing divergence/rebase context.
- Disable remote actions in diverged state and provide a tooltip or adjacent accessible reason; do not
  leave an inert-looking control without explanation.
- Thread the same capability into `GitActionsDropdown` and `useMobileGitActions`. Disable Pull and the
  Push submenu in diverged state; keep the 44px trigger and local Commit action.
- Use the shared Changes panel on mobile. No mobile-only banner, drawer, nested scroll region, or
  compressed desktop control is added.

## Internationalization

Add keys for the warning, current-provider heading, local-checkout heading, row provenance, provider
history failure, and disabled remote-action reason in:

- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/pseudo/task.json`

Use `count` with `_one`/`_other` for any count-bearing copy and never compare translated strings.

## Acceptance

- Rewritten provider and local histories are visually and accessibly separate on desktop and mobile.
- Old contributor commits are labeled as checkout history, not as Kandev-authored or waiting to push.
- Push, force-push, and generic Pull cannot be invoked from the Changes panel, desktop split button, or
  mobile Git menu while diverged.
- Local work and local recovery operations remain intact.
- Aligned/local-ahead/provider-ahead/non-PR presentations remain unchanged except for corrected
  upstream Push/Pull counts.
- The shared mobile panel has no horizontal overflow and preserves its existing vertical scroll owner.

## Files likely touched

- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/changes-panel-timeline.tsx`
- `apps/web/components/task/changes-panel-repo-groups.tsx`
- `apps/web/components/task/changes-panel-helpers.ts`
- `apps/web/components/task/commit-row.tsx`
- Changes-panel unit/component tests beside those files
- `apps/web/components/vcs-split-button.tsx`
- `apps/web/components/vcs-split-button.test.ts`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx` (new)
- four `task.json` locale files

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/changes-panel.test.ts components/task/changes-panel-remote.test.ts components/task/changes-panel-timeline-grouping.test.tsx components/vcs-split-button.test.ts components/task/mobile/session-mobile-top-bar-git-controls.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Mobile parity note

Nearest exemplars: the shared Changes panel in `task-layout.tsx` and the existing
`session-mobile-top-bar-git-controls.tsx`. The new warning is content, not a new navigation surface. It
must remain inside the panel's current scroll owner and use the mobile menu's existing touch target.

## Dependencies and parallelism

Depends on Task 02. Run sequentially because desktop/mobile control wiring and Changes-panel props share
the same relation API.

## Output contract

Record screenshots or component assertions for both layouts, action-policy assertions, translation
checks, typecheck output, and blockers. Update this task and the plan checkbox when complete.

## Completion evidence

- Diverged histories render as separate current-PR and local-checkout sections with provenance labels,
  a translated warning, preserved local recovery actions, and no misleading unpushed arrow.
- Desktop and Pixel 5 controls consume the same action policy: remote mutations are disabled while
  local Commit remains available; the existing mobile scroll owner and touch target are preserved.
- Focused Changes-panel, commit-row, VCS split-button, and mobile Git-control tests passed. Typecheck,
  lint, i18n check, and i18n ratchet passed.
