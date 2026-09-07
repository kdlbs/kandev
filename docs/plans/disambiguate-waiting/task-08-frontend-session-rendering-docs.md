# Task 08: `/tasks` row, session switcher, TS types, docs amendment

Spec: §"Rendering contract" 5-6, §"Contract amendment". ACs: AC-51, AC-52,
AC-73a, AC-75, AC-76 (guard only — no code, see note), plus the DTO/type
plumbing AC-21/22/etc. from task-06 need on the frontend side.

## `/tasks` list row

`app/tasks/rich-task-list-row.tsx:38` — pass `parkedOnBackgroundWork` into its
`getTaskStateIcon` call (resolver B, task-07). **This file reads
`task.foreground_activity` (snake_case) from `lib/types/http.ts`'s `Task`**,
not the camelCase store shape `kanban-card-content.tsx` uses — read the new
field with matching casing (`task.parked_on_background_work`) or the wrong
casing yields `undefined → false` and a silently-unparked row (round-5 F19).
AC-73a.

## Session switcher

`getSessionStateIconConfig` (`state-icons.tsx:297-311`) gains
`parkedOnBackgroundWork?: boolean` (default `false`), new branch:
`canRequestInput && parkedOnBackgroundWork` → `SESSION_BACKGROUND_ICON`,
positioned after both pending-input branches and after the existing
`foreground_activity === "background"` branch, before the coarse fallback.
Three call sites pass it: `sessions-dropdown.tsx:475`,
`session-reopen-menu.tsx:204`,
`apps/web/components/task/mobile/mobile-sessions-section.tsx:132` (note:
actual path has `task/mobile/`, not `mobile/` — see plan.md recon delta).
AC-51 (parked → background icon; parked + clarification → question mark wins),
AC-52 (unparked sessions unaffected, extends `state-icons.test.tsx`'s
session-icon describes).

## TS types (three places, per plan.md's recon delta)

Add `parked_on_background_work: boolean`, `revision`/`parked_revision:
number`, `parked_epoch: number` to:

- `apps/web/lib/types/http.ts` — `Task` (`:325`) and `TaskSession` (`:431`),
  snake_case, alongside the existing `cancellation_pending`/
  `cancellation_revision` fields (`:452-456`) as the naming precedent.
- `apps/web/lib/types/backend.ts` — the boot-payload mirrors of the same
  shapes (near `:106`, `:289`, `:307`, `:292-316`).
- `apps/web/lib/state/slices/kanban/types.ts` — store-normalized camelCase
  shape (`:84` area) — `parkedOnBackgroundWork`, `parkedRevision`/`revision`,
  `parkedEpoch`.
- Locate and update the wire→store hydration/merge function(s) that currently
  carry `cancellationPending`/`cancellationRevision` across that boundary (not
  located during recon — grep for `cancellationRevision` in `lib/state/` to
  find every call site and mirror it for the new fields, including the
  `(parked_epoch, revision)` discard-a-stale-update comparison from task-06,
  which must be replicated client-side per AC-39/AC-77).

## AC-75 (queued-prompt divergence — projection side)

Confirm (do not re-implement — this is backend behaviour from task-05) that a
queued-but-unadmitted prompt does not clear the frontend's rendered
affordance: this should fall out naturally once the backend correctly leaves
`parked_on_background_work: true` per task-05's D8 table, and the frontend
just renders whatever the DTO says. Add a state-slice test confirming the
store doesn't locally clear the flag on prompt-queue actions.

## AC-76 (guard — no code change, a completeness check)

Grep the diff from this whole card for any change under
`internal/notifications/` or `apps/web/lib/notifications/` — there must be
none. Record the (absence of) diff in the plan when this task closes.

## Docs amendment

`docs/specs/platform/background-work-liveness.md:25` — replace:

> A settled session follows its coarse state and does not remain visually busy
> solely because detached work is still registered.

with wording stating a settled session may remain visually busy **only when a
positive out-of-band liveness sample supports it**, never on registration
alone (spec's own suggested wording, §"Contract amendment"). Do not touch
`docs/specs/platform/notifications.md` (sibling spec's job). Update
`docs/specs/INDEX.md` status for this spec's row if it moves out of `draft`
once merged (leave as `draft` while still mid-build).

## Tests

- AC-51/52 as above.
- AC-73a: `/tasks` row renders the background test id for a parked task.
- Type-level: TS compiles with the new fields threaded end to end
  (`pnpm run typecheck` is the real check here, not a unit test).
