---
id: "07-pre-launch-warning"
title: "Pre-launch reachability warning"
status: pending
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
that split was retired at the backend. The event always lands in the launched
session's own stream, and a client that initiated that launch renders the same
event inline at the initiation point as well. Both are this task's
responsibility.

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

Pending.
