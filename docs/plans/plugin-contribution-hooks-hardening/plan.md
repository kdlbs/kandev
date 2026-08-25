---
spec: docs/specs/plugins/requirements/plugins.md
created: 2026-08-04
status: completed
---

# Implementation Plan: Plugin Contribution Hooks Hardening

## Overview

Harden PR #2152 without changing its plugin-facing hook names or storage wire
contracts. The repair first introduces authoritative frontend lifecycle state and
fail-closed user-state cleanup, then corrects responsive menu context and mobile panel
composition, proves the combined behavior end to end, synchronizes author-facing
documentation, and removes the remaining unrelated PR delta.

## Confirmed root causes

- `useCloseRevokedPluginPanels` equates a missing registration lasting 500 ms with
  disable/uninstall even though `loadPlugins` is sequential and `loadPlugin` removes
  prior registrations before awaited initialization.
- `Service.Uninstall` removes the package and plugin record before a best-effort
  `plugin_user_state` purge, leaving no retry path if deletion fails.
- `buildPluginMenuContext` hardcodes `presentation: "desktop"` although the mobile
  kanban renders the same `KanbanCard` through `SwipeableColumns`.
- Mobile session state retains a removed plugin panel, and the bottom bar directly
  appends an unbounded number of plugin panel buttons.

## Backend

### Fail-closed per-user state purge

- In `apps/backend/internal/plugins/service.go`, introduce the narrow store seam needed
  to inject `DeleteAllForPlugin` failures while retaining the concrete production
  `state.UserStore` implementation used by handlers.
- Change the per-user purge helper to accept the request context and return an error.
  Invoke it after the plugin process and vault namespace are stopped/purged, but before
  `pkgtar.Remove`, `store.Delete`, and registry removal.
- Reuse the existing stopped-but-installed error reconciliation used for secret-cleanup
  failures. A purge failure must return a retryable uninstall error, preserve the
  package and record, avoid deliverer success notification, and never claim uninstall
  succeeded.
- Preserve the existing nil-store no-op for narrowly constructed tests where per-user
  storage could never have been written.

## Frontend

### Authoritative plugin UI lifecycle

- Extend `apps/web/lib/plugins/registry.ts` with host-internal lifecycle snapshots and
  transition methods. Do not add methods to the plugin-facing `PluginRegistry` type in
  `apps/web/lib/plugins/types.ts`.
- In `apps/web/lib/plugins/host.ts`, mark the current generation `loading` before import
  or unregister; publish `ready` only after the current generation finishes
  initialization, and `failed` on terminal import, missing registration, thrown
  initialization, or timeout. Stale generations must not publish completion.
- Make `unloadPlugin` distinguish explicit removal from a reload/update transition;
  update `apps/web/components/settings/plugins/use-plugin-actions.ts` so install/update,
  enable, disable, and uninstall drive the correct lifecycle state.
- Replace the timer in
  `apps/web/components/task/use-close-revoked-plugin-panels.ts` with deterministic
  reconciliation: preserve unresolved panels during `loading`/`failed`, close missing
  panels after `ready`, and close all owned panels after `removed`.

### Responsive task-menu context

- Add an explicit kanban presentation prop through
  `apps/web/components/kanban/swipeable-columns.tsx`,
  `apps/web/components/kanban-column.tsx`, and
  `apps/web/components/kanban-card.tsx`.
- The desktop layout uses `"desktop"`; `SwipeableColumns` uses `"mobile"`.
  `buildPluginMenuContext` forwards the supplied value to both dropdown and context
  menu actions.

### Bounded mobile plugin panels

- Replace direct plugin panel expansion in
  `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx` with one localized,
  touch-sized Panels action whenever at least one mobile-enabled registration exists.
- Add a focused picker component beside the mobile task components, using
  `MobilePickerSheet`: fixed header, `min-h-0` internally scrolling body, safe-area
  behavior from the shared drawer, 44 px rows, icons/titles, and no document horizontal
  overflow. Selecting a row focuses the existing full-height `MobilePluginPanel` and
  dismisses the picker.
- In `apps/web/components/task/mobile/session-mobile-layout.tsx`, reconcile the focused
  plugin panel with authoritative lifecycle state. Definitive removal selects `chat`;
  `loading`/`failed` preserves the stored selection so reload or recovery does not
  overwrite desktop/mobile user state.

Mobile design contract: desktop retains dockview's add-panel menu and tabs. Phone uses
one bottom-nav entry as a temporary hierarchy choice, following
`MobilePickerSheet`/`MobileSessionsPicker`; the selected panel remains the single
full-height focal surface and owns its internal content scrolling. The bottom bar and
drawer retain safe-area handling, all picker rows meet the 44 px target, and the same
registry registrations and session panel state drive both viewports.

## Contract and documentation

- Update `docs/plans/plugins/PLUGIN-API.md` and
  `docs/public/plugins-authoring.md` to describe lifecycle-safe reloads, definitive
  revocation, the grouped phone picker, and responsive task-menu presentation.
- Keep `docs/specs/plugins/requirements/plugins.md`, ADR-2026-08-01-plugin-task-panel-contributions,
  and ADR-2026-08-04-plugin-contribution-lifecycle-authority consistent with the final
  implementation.
- Revert `apps/web/scripts/lib/git-base.mjs` to the PR base/current `main`; its
  in-progress-merge ratchet behavior is unrelated to plugin contribution hooks.

## Tests

- **Slow reload preserves a panel:** extend
  `apps/web/lib/plugins/host.test.ts` and
  `apps/web/components/task/use-close-revoked-plugin-panels.test.ts` with a controlled
  initialization gate that exceeds the old 500 ms boundary and confirms no panel is
  removed before the current generation becomes ready.
- **Successful replacement can revoke a panel:** prove a ready generation that omits a
  prior panel closes it exactly once; failed/timed-out generations preserve it.
- **Lifecycle generation fencing:** extend `apps/web/lib/plugins/host.test.ts` so an old
  load cannot overwrite a newer lifecycle state or registration set.
- **Fail-closed uninstall:** add a service test with an injected user-state deletion
  failure. Assert the error, retained package/record, absence of successful refresh,
  and successful idempotent retry; retain the existing every-user happy-path test.
- **Responsive menu context:** add focused frontend tests proving mobile cards supply
  `"mobile"` and desktop cards supply `"desktop"` to both `visible` and `run`.
- **Mobile revocation and picker capacity:** extend the mobile layout/nav component
  tests with definitive removal, loading preservation, multiple registrations,
  duplicate titles, selection, dismissal, and bounded bottom-nav item count.

## E2E Tests

- **Slow/reloaded desktop panel:** in
  `apps/web/e2e/tests/plugins/plugin-task-panel.spec.ts`, keep an opened Notes panel
  through an update/reload and confirm it remains usable, while explicit disable still
  removes it.
- **Mobile picker and revocation:** extend
  `apps/web/e2e/tests/plugins/mobile-plugin-task-panel.spec.ts` to open Notes through
  Panels, assert touch/viewport geometry, disable it while focused, and observe Chat as
  the deterministic fallback with no horizontal overflow.
- **Mobile task-menu presentation:** make the fixture action expose its received
  presentation through an observable stored value, invoke it from the phone kanban,
  and assert `"mobile"`; retain the desktop assertion for `"desktop"`.

## Verification Results

Implementation waves are complete. Recorded verification results:

- `rtk go test ./internal/plugins` — 281 tests passed.
- `rtk pnpm run typecheck` — passed from `apps/web`.
- `rtk pnpm run i18n:check && rtk pnpm run i18n:ratchet` — passed.
- `rtk pnpm run lint` — passed from `apps/web`.
- Focused frontend lifecycle/menu/mobile suite — 106 tests passed across 10 files.
- `rtk node --test scripts/validate-public-docs.test.mjs` — 58 tests passed;
  `rtk node scripts/validate-public-docs.mjs` — 41 published pages validated.
- `rtk git diff --exit-code origin/main -- apps/web/scripts/lib/git-base.mjs` and
  `rtk git diff --check` — passed.
- Managed desktop plugin E2E — 6 passed; managed mobile plugin E2E — 2 passed.
- PR #2278 was created from `aa6096cd4` with three published UI screenshots. The
  15-minute wait completed before fixup.
- Review fixup commit `c065783e5` localized the plugin Edit task submenu label;
  focused menu tests (11), i18n checks/ratchet, lint, typecheck, commit hooks,
  and the push all passed. The review thread was replied to and resolved.
- CodeRabbit's four follow-up threads were fixed in `13841d68d`: shared fixture
  installation, shared writer-id composition with 35 storage tests, failed-read
  retry state, and serialized conflict-safe public storage example writes. The
  desktop plugin/realtime E2E suite passed 7 tests, mobile plugin E2E passed 2,
  and docs validation passed 58 tests across 41 published pages.
- CodeRabbit then identified a task-change race in that documentation queue;
  `8d7b8299a` fences stale callbacks, resets queue state, and clears timer refs.
  Public-doc validation passed again with 58 tests and 41 published pages.
- The final `8d7b8299a` audit has CodeRabbit successful, zero failed checks, and
  zero unresolved threads; remaining CodeQL jobs were still running at handoff.

Task files contain the wave-specific commands, red-test evidence, and results.

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates; user authorization required for subagents):

- [x] [Task 01 — Authoritative plugin lifecycle](task-01-authoritative-plugin-lifecycle.md)
- [x] [Task 02 — Fail-closed user-state uninstall](task-02-fail-closed-user-state-uninstall.md)
- [x] [Task 03 — Responsive task-menu context](task-03-responsive-task-menu-context.md)
- [x] [Task 07 — Remove unrelated PR delta](task-07-remove-unrelated-pr-delta.md)

Wave 2:

- [x] [Task 04 — Bounded mobile plugin panels](task-04-bounded-mobile-plugin-panels.md)

Wave 3:

- [x] [Task 05 — Plugin hook E2E regressions](task-05-plugin-hook-e2e-regressions.md)
- [x] [Task 06 — Plugin contract documentation](task-06-plugin-contract-documentation.md)

Default execution is sequential in this conversation. Waves only identify disjoint
work; they do not authorize delegation.

## Risks

- Lifecycle completion must remain generation-fenced or a stale import can resurrect
  contributions after disable/update.
- A timed-out `initialize()` promise can continue running; its scoped registry and
  lifecycle completion must both remain fenced.
- The uninstall order cannot be fully transactional across SQLite, filesystem, vault,
  and YAML store. The required invariant is narrower: never report successful removal
  while per-user data is known to remain, and preserve a retryable installed record on
  cleanup failure.
- Mobile picker work touches a recently merged navigation refactor. Reuse the task
  mobile picker rather than extending the global navigation manifest.

## Out of scope

- New plugin-facing methods, manifest fields, database migrations, HTTP routes, or WS
  payloads.
- Sandboxing plugin JavaScript or changing the trusted in-origin execution model.
- Generalizing task-menu actions beyond the existing kanban Edit group.
- Keeping the unrelated `git-base.mjs` behavior in this PR.

## Delivery

- The hardening changes are consolidated into one commit directly on top of the
  contributor head from #2152.
- #2152 remains the canonical contributor PR and receives the hardening commit;
  #2278 is a temporary superseding PR and is closed in its favor.
- The contributor branch remains responsible for the combined feature and
  lifecycle-hardening change, with no new plugin-facing hook or wire contract.
