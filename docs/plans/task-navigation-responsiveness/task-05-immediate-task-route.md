---
id: "05-immediate-task-route"
title: "Render selected tasks before background hydration"
status: done
wave: 5
depends_on:
  - "04-navigation-evidence"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 05: Immediate Task Route Presentation

## Scope and root cause

Continue the user's explicitly authorized performance implementation. Every SPA
route change calls full boot enrichment and covers the workbench until its
slowest optional request settles. The shared optional deadline is five seconds.
Render existing selected-task state immediately and use essential task/session
resolution for client navigation. Full boot hydration remains unchanged.

Own `src/task-detail-route.tsx`, `lib/ssr/session-page-state.ts`, task projection
and details helpers, the existing background session reconciler and shared model
hydration mapper, their tests, and focused desktop/mobile navigation E2E.
The shared ownership resolver lives in `lib/routing/resolve-task-route.ts`;
the current-store presentation hook lives in `src/task-route-projection.ts`.
Do not change backend APIs, message pagination, task eligibility, or live data.

## Acceptance and regression sequence

1. RED: with task B present in the current workspace and a validated cached B
   session, navigate A to B with a held route response. B's shell must render
   synchronously without an inert ancestor or task-loading overlay.
2. Preserve requested-session ownership, sessionless/unknown loading, route
   errors, A-B-A cancellation, and live hydration epochs. Read tracking and
   session creation wait for authoritative hydration, independently of display.
3. Client loader resolves essential task/session data without issuing optional
   boot reads or marking unloaded messages/turns as empty. Projection retains
   workspace/repositories/status and does not invent archive eligibility.
4. Desktop and phone browser checks hold task refresh responses on a warm
   return. Assert selected task/chat and usable navigation before release.
   Verify cold navigation and missing-task recovery, plus no page errors.
5. Record comparable click-to-visible-content timings, overlay presence, and
   request counts using isolated fictional data. Do not claim live Firefox
   certification or literal zero latency for uncached network data.

## UI preview and mobile contract

Follow UI-03 in the plan. Desktop keeps the sidebar and Dockview workbench;
phone keeps `session-task-switcher-sheet.tsx` and the focused
`session-mobile-layout.tsx` chat surface, existing safe-area-aware bottom
navigation, touch targets, and one content scroll owner. No new overlay or
control is introduced. Both use the same route resolver and existing store.

## Verification

From `apps/web`, with the repository's Node/pnpm toolchain:

```bash
pnpm exec vitest run src/task-detail-route.test.tsx lib/routing/resolve-task-route.test.ts lib/ssr/session-page-state.test.ts components/task/task-page-content-helpers.test.ts components/task/task-page-content.test.tsx components/task/chat/use-session-read-tracking.test.ts hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/session/use-session.test.ts hooks/domains/session/session-state-reconciler.test.ts
pnpm run typecheck
pnpm e2e:run --project chromium tests/task/task-route-responsiveness.spec.ts
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-task-route-responsiveness.spec.ts
```

Run changed-file ESLint/Prettier, localization checks, and from the repository
root `python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all`. Add relevant route-recovery/session
selection regressions to the focused run when the implementation touches them.

## Results

RED/GREEN evidence covers the previously hidden cached destination, duplicated
detail reads, lost task projection metadata, and A-B-A readiness reuse. The last
case requires the exact new hydration snapshot to finish before read tracking
or session creation can proceed. Authoritative task data replaces the provisional
projection; optional boot reads are absent from the client route loader.

All 145 focused unit cases pass across route resolution, hydration, task
details/projection, read tracking, automatic session creation, and session
reconciliation. TypeScript, changed-file ESLint
and Prettier, localization checks, specification validation, harness validation,
and public documentation validation pass. The final managed production build
passes 14 Chromium and 10 mobile-chrome cases with one worker per run:

```bash
pnpm e2e:run --host --project chromium tests/task/task-route-responsiveness.spec.ts tests/task/task-loading-state.spec.ts tests/chat/unread-divider.spec.ts tests/chat/model-selector-consecutive-switch.spec.ts tests/terminal/terminal-ended-session.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-task-route-responsiveness.spec.ts tests/task/mobile-task-loading-state.spec.ts tests/chat/mobile-unread-divider.spec.ts tests/layout/mobile-spa-resilience.spec.ts
```

The browser cases prove a warm selected conversation is visible while its task
detail response is held. Mobile optional-hydration regressions now require the
destination to remain available while background requests fail. Cold/missing
tasks retain recovery, and saved secondary sessions and unread cursors retain
their existing behavior. No personal-instance data or runtime settings changed.

The isolated Harbor Logistics fixture has 18 tasks, two visited sessions with
50+ messages each, and 633 repository files. With debug logging disabled and
a 1440x960 viewport, 20 warm switches per browser/build produced:

| Browser | Previous fix median | Follow-up median | Follow-up p95 | HTTP reads per switch |
| --- | --- | --- | --- | --- |
| Chromium 149.0.7827.55 | 841 ms | 216 ms | 233 ms | 41 to 20 |
| Firefox 151.0 | 1,782 ms | 419 ms | 484 ms | 41 to 20 |

The previous fix's seeded preview blocked content on all 20 switches in each
browser; the final follow-up blocked none and reported no page errors. Timings
measure click capture to the selected cached conversation's DOM on animation
frames, with no loading overlay or inert ancestor, staying available for the
rest of a minimum one-second observation window. They are not compositor paint
measurements, cold-start timings, or certification of the personal browser.

The baseline asset was `index-wjFzt6FH.js`; the final follow-up asset was
`index-4hqlzLbb.js`. Ignored raw samples, measurement/capture scripts, and the
render profile are under `.kandev/diagnostics/task-route-followup/`. A separate
CPU sample shows remaining React/workbench, layout/virtualizer measurement, and
Markdown costs. This change removes the route network barrier; it does not claim
zero rendering latency or repair an unproven memory leak.


## Review and CI remediation

Review regressions now cover persisted model/configuration backfill without
replacing newer runtime events, selected secondary-session preservation during
route refresh, deferred reconnect refresh, filtered session-list totals, and
archived-task projection exclusion. Task and session projection share one store
snapshot. The persisted model mapper is shared with full boot hydration and
runs in the existing background session reconciler without an extra request.

CI's ended-session terminal case assumed a stopped agent always implies an
unavailable workspace. Protocol traces and the backend recovery contract show
that workspace-only restoration is allowed while the agent remains stopped.
The unavailable case now explicitly supplies a recovery-ineligible status and
retains its ended-notice/no-spinner assertion. A companion case executes a real
shell command after workspace readiness and verifies unchanged terminal session
state and no running agent. Both pass without retries; no terminal production
logic was changed to suppress valid recovery. The provisional shell-hydration
gate was disproved during diagnosis and removed.

The initial PR-head CI failure was limited to that terminal case and its two
aggregate checks. Final-head CI, automated review dispositions, current-base
merge validation, and the authorized merge are tracked in the external task plan.
