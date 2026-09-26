---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-08-02
status: completed
---

# Implementation Plan: Restore Dockview Active Tabs Across Tasks

## Overview

Repair desktop task-environment restoration so a selected Agent tab survives
the dynamic replacement of `chat` or a stale `session:<id>` with the incoming
task's live `session:<id>`. The implementation will restore every group's
selection from the target task's saved layout, or from the effective default
for a fresh environment, while preserving exact non-Agent selections and
applying the saved global group last.

## Confirmed Root Cause

Task selection correctly resolves and focuses the incoming primary session.
The observed failure happens later, while Dockview restores the target
environment:

- `performEnvSwitch` reconciles the serialized task layout with the incoming
  session's dynamic panel ID;
- the fast path replays saved `activeView` values only by exact panel ID;
- a saved Agent selection such as `chat` or a stale `session:<id>` therefore
  does not match the newly materialized `session:<incoming-id>` panel;
- the slow path calls `fromJSON` before replacing stale session panels and does
  not replay the selection after replacement; and
- `useAutoSessionTab` then correctly declines to steal focus on a cross-task
  restore, leaving the neighboring PR Details panel active.

Changing the later `shouldActivateSessionPanel` decision to always select Agent
would violate the existing contract for tasks whose last selected tab really
was PR Details, Plan, Files, Changes, or another valid non-Agent panel. The
repair belongs in environment restoration, where the target layout and the
incoming session identity are both available.

## Frontend

Likely file:

- `apps/web/lib/state/dockview-env-switch.ts`

Introduce one post-reconciliation active-view restoration path that:

- obtains the target selections from the healthy saved serialized layout, or
  from the materialized effective default when no task layout exists;
- recognizes `chat` and `session:*` as the semantic Agent slot only when that
  selected view represents a session panel;
- maps that Agent selection to `session:${activeSessionId}` after stale and
  missing session panels have been reconciled;
- restores stable non-Agent panel IDs only by exact match;
- restores each group's selected panel without changing group structure or tab
  order; and
- applies the target globally active group last so per-group restoration does
  not leave focus in the wrong split.

Use the same restoration helper after both the structure-preserving fast path
and the `fromJSON` slow path. Keep the pending-inactive-task Changes override in
its existing later lifecycle position, so meaningful updates can still
deliberately supersede the saved selection.

## Tests

### Unit RED/GREEN

File:

- `apps/web/lib/state/dockview-env-switch.test.ts`

Add focused regressions for:

- fast-path restoration of saved `activeView: "chat"` to the live incoming
  session panel beside PR Details;
- fast-path restoration of a stale `session:<old-id>` selection to the live
  incoming session panel;
- slow-path `fromJSON` restoration after stale-session replacement;
- a deliberately selected `pr-detail` remaining selected exactly;
- selections in more than one group being restored while the serialized
  `activeGroup` is activated last; and
- a fresh environment using the effective default's Agent selection instead
  of inheriting PR Details from the outgoing environment.

Retain the existing session-tab activation tests in the targeted command to
guard the downstream policy that intentionally avoids stealing restored focus.

## E2E Tests

### Desktop PR Details round trip

File:

- `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`

Create two desktop tasks with settled sessions. In the first task, use the
Default center group containing Agent and PR Details, select PR Details and then
Agent so Agent is the last real selection, switch to the second task, and return
through the normal task UI. Assert that:

- the first task's live Agent tab and content are active;
- PR Details remains present but inactive;
- the current session panel exists exactly once.

Add a companion round trip where PR Details is deliberately left selected and
prove it remains selected. This distinguishes semantic Agent rebinding from an
unconditional Agent activation.

Run the scenario through Kandev's managed production-build Playwright runner;
use API helpers only for task/session preconditions and verify focus through
the rendered workbench plus the exposed Dockview API where group-local identity
is not observable from content alone.

## Mobile And Tablet

No mobile or tablet regression is required. Phone and tablet task detail use
`SessionMobileLayout` and `SessionTabletLayout`, not `DockviewDesktopLayout`,
and do not execute the environment-switch restoration being repaired. This
change introduces no rendered UI, copy, responsive composition, touch target,
scroll owner, or mobile navigation behavior.

## Implementation Waves

Wave 1 (sequential):

- [x] [Task 01 — Restore semantic active views](task-01-restore-semantic-active-views.md)

## Verification

Targeted unit RED/GREEN and downstream activation policy:

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  lib/state/dockview-env-switch.test.ts \
  components/task/dockview-session-tab-activation.test.ts \
  components/task/dockview-session-tabs.test.ts
```

Frontend typecheck:

```bash
cd apps/web && pnpm run typecheck
```

Managed production-build desktop E2E:

```bash
cd apps/web && pnpm e2e:run tests/pr/pr-detail-layout.spec.ts \
  -- --grep "restores each task's selected center tab"
```

## Risks

- Dockview's group-local selected panel and globally active panel are distinct;
  restoring one without ordering the other can produce a correct-looking
  center tab while moving keyboard focus to the wrong split.
- Treating every missing panel ID as Agent would resurrect removed panels or
  overwrite legitimate PR Details, Plan, preview, or file selections. Only
  reusable Chat and serialized session identities may rebind.
- Replaying selections before stale-session replacement repeats the bug because
  the target live Agent ID does not yet exist.
- The no-saved-layout fast path must take selection intent from the effective
  default without applying the default's entire structure over the live layout.
- Pending inactive-task Changes attention must remain a deliberate later
  override and must not be folded into the generic restoration helper.

## Out Of Scope

- Changing task/session selection or primary-session resolution.
- Always activating Agent during cross-task navigation.
- Changing Dockview versions, saved-layout schemas, or storage keys.
- Altering pending Changes attention behavior.
- Changing mobile or tablet task-detail composition.
- Adding new user-facing copy or public configuration.

## Documentation Impact

No public documentation changes are required. This repair makes the desktop
workbench conform to its existing task-layout persistence contract and changes
no command, configuration key, API, or user-facing terminology.

## Implementation Results

- RED unit run reproduced the bug: saved `chat`, stale `session:<id>`, and
  fresh-default Agent selections were not activated, while the neighboring PR
  Details panel remained eligible to win focus.
- GREEN implementation centralizes target active-view replay, resolves only
  semantic Agent identities to `session:${activeSessionId}`, and runs after
  reconciliation on both fast and slow environment-switch paths.
- Targeted unit suite: 70 tests passed.
- Frontend typecheck: passed.
- Managed production-build Chromium E2E: 1 test passed, covering Agent-last and
  deliberate PR-Details-last round trips.
- Targeted ESLint: no errors; two max-lines warnings remain in the large
  Dockview environment-switch test file.
- No mobile/tablet code or tests changed because those layouts do not execute
  Dockview environment restoration.
