---
id: "12i-shared-task-indicator-hover-summary"
title: "Reuse the native pull-request summary from registered task indicators"
status: completed
wave: 3i
depends_on: ["12h-native-create-unlink-indicators-saved-queries"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 12i: Reuse the Native Pull-Request Summary from Registered Task Indicators

## Root cause

`PRTaskIcon` renders GitHub's structured status summary, while
`RegisteredChangeRequestTaskIcon` independently renders only the association count as
tooltip text. The registered-provider path never subscribes to or lazily refreshes the
provider's existing task review snapshot, so it cannot supply title/review/CI detail.

## Mobile design contract

- Desktop task switcher/Kanban/rich-list icons disclose the shared structured summary on
  mouse hover and keyboard focus.
- The nearest mobile exemplar is the existing registered topbar/composer status drawer
  and mobile Review navigation. Those touch paths already expose the same normalized
  review data and remain the mobile entry point; the sidebar hover affordance is not
  mounted as a phone-only interaction.
- Shared provider state and summary derivation remain viewport-neutral. No new scroll
  owner, safe-area surface, or touch-only control is introduced.

## Owned paths

- `apps/web/components/{github,integrations}/**` shared task-summary presentation,
  registered task icon, and focused tests
- `apps/web/e2e/tests/plugins/**` desktop hover contract and existing mobile parity path
- `docs/{specs,plans,public}/**` provider contract documentation

## TDD and implementation

1. RED: prove a registered linked task's hover/focus tooltip lacks the PR number, title,
   review, and CI rows even when its provider publishes `ReviewItemSummary.taskStatus`.
2. Extract/reuse the exact GitHub task-summary renderer as host-owned, provider-neutral
   presentation. Map normalized registered review status into that renderer.
3. Subscribe to task review snapshots without refreshing every task row. Refresh only
   when the icon opens, using the existing deduplicated provider refresh lease.
4. Preserve association-only initial rendering, registry unload cleanup, keyboard
   focus/Escape behavior, and semantic status color/count.
5. Prove the real packaged Bitbucket provider in the isolated seed; retain the existing
   mobile status/Review E2E as the touch path.

## Acceptance

- Bitbucket's linked-task icon opens the same structured summary anatomy as GitHub.
- The first workspace association refresh remains one bounded request across task rows;
  task detail is not fetched until hover/focus or another active review surface needs it.
- Multiple linked PRs render separate summary entries; missing detail degrades to the
  existing accessible linked-count label while loading.
- Provider unload removes the icon and cancels in-flight work.
- Focused frontend tests, typecheck/lint, rendered desktop verification, existing mobile
  parity coverage, and `git diff --check` pass.

## Risks

- Mount-time use of the general review hook would reintroduce N-per-task provider calls;
  use subscriptions plus open-time refresh instead.
- A hover tooltip has no touch equivalent; the existing mobile Status/Review path must
  remain discoverable and tested.

## Completion evidence

- Focused Vitest: 4 files / 16 tests passed.
- Web typecheck and full ESLint passed.
- Packaged Bitbucket plugin E2E proved association discovery stays detail-lazy until
  hover, then renders PR #42, title, Approved review, and Passed CI through the shared
  host summary.
- Mobile plugin contract E2E passed on `mobile-chrome`, preserving the touch Review path.
- The isolated seed rendered the correctly embedded production bundle and the same
  structured summary; `apps/web/.pr-assets/bitbucket-sidebar-pr-summary.png` records it.
- Follow-up manual acceptance found the registered glyph remained hard-coded muted.
  Focused regression coverage now proves normalized provider detail drives the same
  host-owned semantic status colors as first-party pull requests; the muted association-
  only fallback remains until lazy detail arrives.
- The packaged-provider desktop E2E now asserts the resolved semantic color, and the
  public plugin-authoring reference maps every code-host hook to its host-owned result
  with eight focused screenshots from an isolated packaged-plugin run.
