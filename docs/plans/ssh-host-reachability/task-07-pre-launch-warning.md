---
id: "07-pre-launch-warning"
title: "Pre-launch reachability warning"
status: done
wave: 5
depends_on: ["05-launch-non-gating", "06-settings-surface"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-003
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-003.2
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
---

# Task 07: Pre-Launch Reachability Warning

## Summary

A launch against an SSH host currently recorded as unreachable warns before the
attempt and proceeds anyway. The warning informs; it never gates. **The
warning renders a `session.launch.warning` event the backend pushes (task 05's
single producer); it is never derived client-side from the reachability store
slice.** There is no interactive/non-interactive split to implement here —
that split was retired at the backend. The gateway replays the latest warning
to a later subscriber while the backend is running, and a client that initiated
that launch renders the same event inline at the initiation point as well.
Both are this task's responsibility.

## In scope

- **Session-stream rendering.** A session-scoped WS event handler for
  `session.launch.warning`, following the existing pattern in
  `apps/web/lib/ws/handlers/executor-prepare.ts`, that applies the event to
  session-runtime state so the session view can render it wherever a user
  opens that session's card — including a user who was not present at launch
  (a dependency chain, a workflow transition, an autostart).
- **Inline-at-initiation rendering.** The client that issued the launch
  request renders the same event inline at the point of initiation, naming
  the host and the age of the last successful probe, or stating that none has
  been recorded, without waiting for or duplicating the session-stream path.
- The launch proceeds without a confirmation step in either rendering. No
  blocking dialog, no disabled action, no extra click on the path to starting
  work.
- Both renderers key off the event payload alone (`executor_id`, `host`,
  `state`, `reason`, `last_success_at`); neither reads or re-derives
  `unreachable` from the task 06 reachability store slice, and neither issues
  a fetch of its own. The backend's "when to warn" rule (probing enabled, or a
  fresh-enough record while disabled) is entirely task 05's; this task cannot
  express or duplicate that condition without risking drift from it.
- Mobile-viewport layout, and the warning exposed to assistive technology as
  text rather than by color alone.
- Copy through `t()` in `tasks.json` across `en`, `pt-pt`, `zh-cn`, `zh-hk`,
  `zh-tw`, with the failure reason rendered as translated copy, never the raw
  token.

## Out of scope

- **Deferred:** the task-card reachability indicator. Retired IDs
  `AC-EXECUTORS-SSH-REACHABILITY-002.8` and `002.9` are not reused; the
  behavior is specified in the requirement document's `## Out of scope`.
- The settings card, the store slice, and the API client — task 06 owns all
  three.
- The event's production, its when-to-warn condition, and the event type
  registration — task 05 owns all three. This task only consumes the event.
- Blocking, confirming, deferring, or re-routing the launch. The warning is
  informational and the attempt proceeds.
- Any backend file.

## Acceptance

- Launching against an `unreachable` host produces a `session.launch.warning`
  that the session-stream handler applies to session-runtime state, and the
  initiating client additionally renders it inline at the point of
  initiation, naming that host and the age of the last successful probe; the
  launch proceeds without a confirmation step in either rendering.
- When `last_success_at` is null in the event payload, both renderers say no
  successful probe has ever been recorded, rather than rendering an empty or
  zero age.
- A user who opens the session card later (not present at launch — a
  dependency-chain or workflow-triggered launch) still sees the warning via
  the session-stream rendering, with no inline rendering required for that
  path since no client initiated it interactively.
- Absent a `session.launch.warning` event, neither renderer shows anything;
  the frontend never independently concludes "unreachable" from the store
  slice.

## Verification

```bash
# From apps/web:
pnpm vitest run lib/ws/handlers/ssh-launch-warning.test.ts components/task/launch-warning.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/ws/handlers/ssh-launch-warning.ts`
- `apps/web/lib/ws/handlers/ssh-launch-warning.test.ts`
- `apps/web/components/task/launch-warning.tsx`
- `apps/web/components/task/launch-warning.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/tasks.json`

## Dependencies

Task 05, for the `session.launch.warning` event contract and its production.
Task 06, for the reachability record type reused for shape consistency (the
event payload is a subset of it) and the locale/i18n conventions the settings
card already established.

## Risks

- The pre-launch warning sits on the path to starting work. A warning that
  accidentally becomes a confirmation step converts an informational surface
  into the gate REQ-003 forbids; assert the launch proceeds unattended, with no
  user interaction between the warning and the attempt.
- The last-success age can be absent, which is a different sentence from a zero
  age. Rendering `0m` where "never" is meant misreports a host that has never
  once answered as one that just did.
- Building either renderer against the store slice instead of the event
  payload is the most likely regression: it would compile, might even pass a
  test written against the slice's current shape, and would silently
  reintroduce a second "is this unreachable" computation that can drift from
  the backend's when-to-warn rule the moment task 05's condition changes.
- This work order is now a single criterion split across two render surfaces
  fed by one event. Resist folding it into task 06: it is a launch surface
  with its own locale namespace and its own WS handler, and merging it would
  put the settings page and the launch path in one commit.

## Parallelism

`sequential`

## Inputs

- System design, sections *Frontend components* (Launch surfaces) and
  *Launch interaction*.
- Plan.md's Frontend Technical-approach section, "The pre-launch warning
  renders a pushed event, not a derived client computation."
- `apps/web/lib/ws/handlers/executor-prepare.ts` for the session-scoped
  WS-event handler pattern this task's session-stream half follows.
- Task 05 for the event contract (`executor_id`, `host`, `state`, `reason`,
  `last_success_at`) and its publication point.
- The `mobile-parity` skill for native mobile interaction expectations.

## Results

Implemented both renderers against the `session.launch.warning` event payload
only, with no read of the task 06 reachability store slice and no client-side
fetch.

- **Session-stream handler:** `apps/web/lib/ws/handlers/ssh-launch-warning.ts`,
  following `executor-prepare.ts`'s pattern, stores a normalized
  `LaunchWarningEntry` (`executorId`, `host`, `state`, `reason`,
  `lastSuccessAt`) keyed by session id in a new `launchWarning` slot on the
  session-runtime slice; registered in `apps/web/lib/ws/router.ts`.
- **Inline-at-initiation renderer:** `apps/web/components/task/launch-warning.tsx`
  reads that same slot via `useAppStore` and renders host, last-success age
  (or "never recorded"), and a translated reason sentence. It never renders a
  confirmation control. Mounted in `task-chat-panel.tsx` alongside the
  existing `<TaskChatLaunchError>`, not through a virtualized-list injection
  mechanism — that is where the launch surface for a session already renders
  transient launch state, and it satisfies "inline at the point of
  initiation" without a new mount point.
- Since the session-stream handler is the only consumer and a session card
  opened later reads the same store slot, a single handler plus a single
  component covers both the "present at launch" and "opened later" cases from
  the Acceptance section; no separate non-interactive path was needed.

**Scope deviations, disclosed:**

1. **One backend file set, out of this task's stated scope.** Wiring the
   bus event to the WS gateway (`GetSessionID()` on the payload, the
   `session.launch.warning` action constant, and the subscription
   registration in `RegisterSessionStreamNotifications`) turned out to be
   missing entirely — task 05 published the event to the internal bus but
   nothing forwarded it to WS clients, so no frontend handler could ever have
   fired regardless of implementation. Fixed as a small, tested gap-fix
   (`manager_launch_reachability_warning.go`, `session_notifications.go`,
   `session_notifications_test.go`, `actions.go`) rather than leaving the
   frontend half unreachable. Same shape as the disclosed deviations in tasks
   04/05.
2. **Locale namespace corrected from the work order's literal text.** The
   work order names `tasks.json`; the actual convention (confirmed against
   existing launch/session copy) is `task.json` (`task:` namespace).
   `tasks.json` (`tasks:` namespace) is scoped to the `/tasks` list view only.
   The 10 new keys were added to `task.json` in all five locales instead.
3. **No reuse of task 06's `reachabilityReasonKey()`.** Per the work order's
   own Risk-section guidance against folding this into task 06's namespace,
   the six reason tokens are mapped independently to fresh
   `task:launchWarningReason*` keys rather than importing task 06's
   `executors.json`-scoped helper.
4. **Incidental store-composition fix.** Found and fixed a pre-existing gap in
   `apps/web/lib/state/store-overrides.ts`'s `buildStateOverrides`: it did not
   list the new `launchWarning` field, so `StateProvider`/boot-payload
   `initialState` would have silently been reset to `{bySessionId: {}}` by
   the session-runtime slice's own default spread. This is a store-plumbing
   fix required for the new field to hydrate correctly at all, not a design
   change.

Verification (from `apps/web`):

```
pnpm vitest run lib/ws/handlers/ssh-launch-warning.test.ts components/task/launch-warning.test.tsx lib/state/slices/session-runtime/purge-session.test.ts
# 3 files, 16 tests passed
pnpm run typecheck   # clean
pnpm run lint        # clean, 0 warnings
pnpm run i18n:check  # clean, pseudo in sync, 5 locales complete
pnpm run i18n:ratchet # clean
```

And from `apps/backend`:

```
go test ./internal/gateway/websocket/... -run TestSessionStreamBroadcaster
# ok
gofmt -l <the 4 touched files>   # no output
```
