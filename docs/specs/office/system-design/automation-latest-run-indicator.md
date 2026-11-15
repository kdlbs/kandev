---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
created: 2026-11-05
owners:
  - kandev
---

# Automation Latest-Run Indicator

## Purpose and boundaries

Office owns this presentation because it owns automation run status, newest-run
selection, and the existing automation-navigation contract. This extends
[automation runs](automation-runs.md), specifically AC-001.1, .9, and .19 through
.22 of [REQ-OFFICE-AUTOMATION-RUNS-001](../requirements/automation-runs.md).
It does not create a UI-system requirement or change status persistence.

Issue [#5081](https://github.com/kdlbs/kandev/issues/5081) reports a green sidebar
dot next to an automation whose newest run is Failed. The current
`AutomationRowLink` uses `STATE_DOT_CLASS[row.state]`, and `automationState`
knows only open-run count and enabled/schedule state. Thus a terminal failure
becomes green idle. The server already provides the missing outcome.

User-settled choices: paused remains muted; cover the sidebar, agenda, and phone
drawer; show non-success outcome words, keeping successful sidebar metadata
age-only. No new architecture decision is needed.

## Requirement mapping

| Requirement criteria | Design section |
| --- | --- |
| AC-OFFICE-AUTOMATION-RUNS-001.1, .19, .20 | Data authority and presentation |
| AC-OFFICE-AUTOMATION-RUNS-001.9 | Refresh and failure boundaries |
| AC-OFFICE-AUTOMATION-RUNS-001.21 | Desktop composition and localization |
| AC-OFFICE-AUTOMATION-RUNS-001.22 | Phone composition and accessibility |

## Data authority and presentation

Keep `AutomationSummary.last_run` and `open_runs` from `automation.summaries`.
`Store.listSummaries` in `apps/backend/internal/automation/store.go` obtains the
newest row using `latestRunPerAutomationSQL`; `selectSummaries` includes the
existing read-time status projection. Ordering is `created_at DESC, id DESC`,
the same as the history. Do not scan the capped workspace feed, select the last
success, or regroup retry attempts. A skipped or scheduled-retry row can be the
newest row. No new backend field, event, migration, query, or endpoint is needed.

Preserve `automationState`, `AutomationActivityState`, `AutomationRow.state`,
`nextFiring`, and agenda ordering. Activity and newest outcome are separate facts.
Add a small pure presentation helper in
`apps/web/components/runs/automation-run-presentation.ts` (planned), consuming
activity state and newest run. Return indicator color, optional newest-status
label key, and whether the non-success label should be visible. Reuse
`statusDotClass`, `statusLabelKey`, and `isOpenRun` from `run-status.ts` instead
of duplicating status colors or vocabulary. Do not add Failed to the activity
state union: that would change running aggregation and scheduling semantics.

Indicator precedence:

1. `state === "running"`: blue. Desktop sidebar retains its animated loader.
   Agenda retains its existing dot geometry; phone retains its compact row
   indicator and explicit Running state. Open work wins even if a newer firing
   is skipped or an automation is switched off while work is still open.
2. `state === "paused"`: existing muted activity color, regardless of newest
   outcome. This includes disabled automation and disabled configured schedule.
3. Otherwise, newest run exists: `statusDotClass(lastRun.status)`.
4. Otherwise: existing idle color and label, no outcome or age.

The unchanged run-detail palette covers `failed` and `retry_scheduling_failed`
(red), `succeeded` (emerald), `cancelled` and `scheduled_retry` (amber),
`skipped` and `archived` (muted), `triggered` (neutral), and `task_created`
(blue). Preserve the existing unknown-status fallback in `run-status.ts`.
Do not infer failures from `error_message` or `summary` prose.

Newest-status metadata remains independent of this precedence. For example,
an older open run plus a newer skip means Running activity and Skipped newest
outcome; a paused automation with a failed newest run has a muted indicator
and visible Failed metadata. Do not label the entire automation successful
just because it is idle.

## Desktop composition and localization

`AutomationRowLink` consumes the shared helper with `row.state` and `row.lastRun`.
Keep the link destination, active styling, ordering, 30-second `useNow` age tick,
and `formatRelativeTime(lastRun.created_at)`. Status and age always reference
that same run. Use visible status before age for every known status except
`succeeded` and the open statuses (`triggered`, `task_created`). Examples:
`Failed, 4m ago`, `Skipped, 4m ago`, `Retry scheduled, 4m ago`. Success remains
`4m ago`. This also applies to paused rows; muting the dot must not discard the
latest failure text. With an open newest run, preserve age-only metadata and
the Running semantic state.

`AgendaRow` in `runs-list-page.tsx` consumes the same helper for its dot. Keep
next-firing/reason first, automation name second, and add non-success status
metadata after the name. Do not replace next-firing data with the last-run time
or alter `buildAgenda` ordering. The agenda is shared across viewports; outcome
metadata must wrap or stack on narrow screens without widening the document.

Use existing translated run-status and activity keys at render time. Introduce
a translated interpolation frame for status plus relative time rather than
English concatenation. Add it to English and all six complete locales; generate
the Traditional Chinese pair with `pnpm run i18n:zh-hant` and the pseudo locale
with `pnpm run i18n:pseudo`. Never call `t()` at module scope. Long translations,
particularly Retry could not be scheduled, must wrap within metadata; do not
clip away the failure word or use a non-shrinking unbounded suffix. Preserve
compact single-line rows for ordinary short labels.

## Phone composition and accessibility

The shipped phone entry point is `AppNavSheet` ->
`MobileSidebarLayoutNavigation` -> `MobileResource` ->
`SavedMobileAutomationRows` -> `ShortcutRows`. The no-children
`MobileAutomationRows` fallback is not selected for a current workspace by
`NavigationAutomations`, because `useHasSavedSidebarLayout` returns true.
Changing only that fallback would not fix the phone experience.

Keep `useSidebarLayoutNavigation` and `useShortcutActivity` as the active
phone data/refresh owners. Extend `ShortcutActivity` with optional newest-run
data populated from the matching summary only for settled, known activity.
Loading/error/unknown results must not expose a fabricated outcome. Leave its
`state`, `hasRunningActivity`, and all custom pinned-shortcut presentation
unchanged. The additional newest-run field is real presentation input, not a
legacy compatibility alias.

Render builtin phone automation rows through a small domain component
`apps/web/components/navigation/mobile-automation-row.tsx` (planned), using
that activity snapshot and the shared presentation helper. Retain catalog
labels, hrefs, availability and row identities; replace only the builtin row
presentation, not generic `ShortcutRows` or `ShortcutActivityIndicator`.
The phone row keeps name first and a visible localized status after it; success
can say Succeeded where the old row said Idle. Paused remains explicitly Paused,
with the newest non-success status additionally visible. Running remains
explicitly Running, with any newer non-success outcome additional metadata.
No age is added to phone rows. Update the reachable fallback to consume the same
row presentation so two implementations cannot diverge if routing selects it.
Do not activate the fallback or give the active route a second summary hook.

Nearest shipped composition is the existing `MobileAutomationsSection` inside
`AppNavSheet`; the curated `MobileMenuSheet` precedent supplies inset drawer,
fixed header, internal scroller and safe-area behavior. Reuse those boundaries,
not a new sheet. This is a content-only row extension: same disclosure, row-tap
history navigation and dismissal, same at-least-44-px hit area, same drawer
scroll owner, no focus or breakpoint changes. Permit a second metadata line for
long localized status, with no horizontal document overflow.

Keep dots/loaders decorative. Accessible text describes the newest run status
and distinct Running/Paused activity once each, without redundant Idle plus
Failed announcements. Successful age-only desktop rows still expose Succeeded
semantically. Never-run rows retain Idle/Paused. Preserve `ShortcutAction`-style
link-name/description separation where a row sets an explicit `aria-label`;
visible child text alone is not sufficient for such a link.

## Refresh and failure boundaries

Do not add pollers or modify `useAutomationSummaries`, `useLiveRefresh`,
`useForegroundRefresh`, or definition publication. The desktop expanded/visible
summary gate and ten-second cadence remain; the active phone route keeps its
existing activity refresh owner. A newly accepted summary changes the displayed
outcome without navigation. A later success clears the failure label and
restores success color when running/paused priority does not apply.

Preserve workspace/request-order guards and existing loading/error handling.
When phone activity is unavailable, keep its Unknown/error behavior and retry
rather than painting stale data as healthy. Do not broaden this change into a
redesign of desktop first-load or failed-refresh presentation. Historical outcome
disappears when the accepted summary no longer contains a newest run (for
example, all run history was deleted). Rendering must not mutate preferences,
run records, task state, retry policy, or telemetry. No new metrics or logging.

## Verification and delivery

The [implementation package](../../../plans/automation-latest-run-indicator/plan.md)
owns exact commands, case names, and desktop/phone previews. Unit coverage must
exercise precedence and status families; component coverage must exercise the
real active phone composition rather than only the inactive fallback. Browser
proof compares a real seeded failed newest run with its detail status, then
observes a newer success without remounting the sidebar/drawer. Preserve the
existing open-run indicator and hidden-section refresh controls.

Related implemented packages remain historical records: [running indicator](../../../plans/automation-sidebar-running-indicator/plan.md)
and [definition freshness](../../../plans/sidebar-automation-definition-refresh/plan.md).
This package does not reopen their completed tasks or change detail-refresh
ownership in [run status refresh](../../../plans/automation-run-status-refresh/plan.md).
