---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Link and unlink from the chip

`MRCIPopover` requires both `onLink` and `onUnlink`. What activating them does
is part of this contract, not an implementation detail.

**`onUnlink`** — a zero-argument closure over the acted-on MR's association `id`,
matching `MRCIPopover`'s declared `onUnlink: () => void`. It calls the extracted
`useUnlinkTaskMR(workspaceId)` described under API surface, passing the id it
closed over. (An earlier draft wrote this heading as `onUnlink(associationId)`,
which is the exact form the prop contract above says is wrong; the prop contract
governs.) Behaviour is the topbar's, unchanged: on success the row leaves the
store; on failure an error toast is shown and the row stays.

**`onLink()`** opens the existing `TaskMRLinkDialog`, which the chip mounts
itself as a sibling of its trigger. Its props come from the same places
`MRTopbarButton` sources them:

| Prop | Source |
|---|---|
| `open` | the chip's own dialog open state, which it owns per mounted instance |
| `onOpenChange` | sets that same state |
| `taskId` | the chip's own `taskId` |
| `workspaceId` | the active workspace |
| `taskRepositories` | `useTaskById(taskId)?.repositories`, defaulting to a module-level shared empty array |
| `repositories` | `state.repositories.itemsByWorkspaceId[workspaceId]`, defaulting to a module-level shared empty array |

Both defaults MUST be module-level constants, not a fresh `[]` per render, or
the dialog re-renders on every parent render. Both props are required and
non-nullable on `TaskMRLinkDialog`, and the workspace bucket can legitimately be
absent before repositories hydrate, so the fallback is reachable rather than
defensive. A dialog opened with an empty `repositories` list is the existing
component's own behaviour and this spec does not change it.

Ordering when the user activates "link another merge request":

1. The chip closes its own disclosure (popover or drawer) first.
2. Then `TaskMRLinkDialog` opens.

Closing first is required, not cosmetic: the drawer variant is itself a
focus-trapping dialog, and opening a second one inside it strands focus. The
dialog's own lifecycle is unchanged, so a successful link adds the association
to the store and the chip re-renders with the new MR in its input; if that MR
outranks the current selection the trigger updates, subject to the freeze rule
only while a disclosure is open, which by step 1 it is not.

Each mounted chip owns its own dialog instance and its own open state, like its
disclosure. Two chips mounted at once therefore have two dialog instances, both
closed until the user activates link on one of them; activating it opens only
that chip's dialog. The dialog is idempotent with respect to which chip opened
it, because both pass the same `taskId` and `workspaceId`.

**The dialog does not outlive its chip.** Because each chip mounts its own
dialog as a sibling of its trigger, a chip that unmounts takes its dialog with
it. So if the task's last open MR is unlinked from another surface, or goes
terminal, while this chip's link dialog is open, the dialog unmounts mid-edit
and any URL the user had typed is lost. That is the accepted behaviour, not an
oversight: the alternative is hoisting the dialog above the chip's own render
gate, which would leave a dialog on screen belonging to a chip that no longer
exists. The window is small (it needs a concurrent change on another surface)
and the user can reopen the dialog from `MRTopbarButton`, which is not gated on
an open MR existing.

## Failure modes

- **Task MR list never hydrated.** `useTaskMRs` returns the shared empty array
  and the chip renders nothing. There is no skeleton and no loading state: an
  absent chip and a not-yet-fetched chip are indistinguishable by design, which
  is the same contract `PRStatusChip` and `MRTaskIcon` already have.
- **Store holds a non-array value for the task** (possible during partial
  hydration). The chip guards with `Array.isArray` and renders nothing, matching
  `MRTaskIcon` and `PRStatusChip`.
- **No active workspace, or `taskId` is null.** The chip renders nothing.
- **Popover feedback fetch fails.** `useMRFeedback` already resolves with
  partial data and an error string; the popover degrades exactly as it does from
  the topbar button. The chip's own trigger status is unaffected, because it is
  derived from `TaskMR` fields rather than from the feedback response.
- **Unlink fails.** The existing behaviour is preserved: an error toast is
  shown and the association stays in the store. A successful unlink removes the
  row; if that was the task's last open MR the chip unmounts on the next render.
- **Two surfaces unlink the same association.** The chip and the topbar read the
  same store row. Whichever request succeeds first removes the row; the other
  surface's request fails and toasts. The end state is identical either way, so
  the outcome does not depend on ordering. The chip does not retry.
- **An open MR transitions to merged/closed while its popover is open.** On the
  next store update the chip re-evaluates. If the transitioned MR is the frozen
  one, the popover or drawer closes; if another open MR remains the trigger
  re-selects it, and if none does the chip unmounts. No stale MR is left
  rendered either way.
- **The GitLab connection status has not resolved yet.** `useGitLabAvailable()`
  returns `false` until the mount-time `fetchGitLabStatus` resolves (and it
  actively clears any cached value first — see Sync and freshness). So `canLink`
  is `false` on the chip's first render and may become `true` a moment later
  **without a remount**. The chip SHALL treat this as ordinary prop change, not
  as a terminal "GitLab unavailable" state: it renders normally, and the
  popover's link control simply appears when the status resolves. The chip SHALL
  NOT cache the first value, gate its own render on it, or show an error. If the
  status request fails, `useGitLabStatus` stores `null`, `canLink` stays `false`,
  and the popover renders without the link control — the same as a workspace
  with GitLab genuinely not configured, which is the existing behaviour of every
  other consumer.
- **The automation-options fetch fails.** `useTaskMRAutomationOptions` stores the
  error and its lazy effect is gated on `error`, so it does not retry for the
  life of the mount. The chip treats this exactly as options-not-loaded: status
  glyph renders, no badges, no error text and no retry affordance on the chip
  itself. The topbar's `MRAutomationControls` remains the surface that reports
  and retries automation errors.

## Sync and freshness (explicit non-responsibility)

GitHub's `PRStatusChip` is load-bearing for refresh. It calls
`usePRFeedbackBackgroundSync`, and `pr-topbar-button.tsx:162` documents the
topbar button relying on the chip being mounted for that warming. GitLab's
refresh does not work that way and this chip SHALL NOT copy the responsibility:

- MR list hydration is a one-shot `useWorkspaceMRs(workspaceId)` fetch whose
  "already fetched" guard is a per-hook-instance ref, so each additional call
  site issues its own `GET /task-mrs` per workspace. There are already **four**
  production call sites, and the chip SHALL NOT become a fifth:

  | Call site | Mounted by |
  |---|---|
  | `components/gitlab/mr-topbar-button.tsx` | the task topbar |
  | `components/kanban-board.tsx` | the kanban board |
  | `hooks/domains/gitlab/use-mr-key-to-tasks.ts` | `app/gitlab/gitlab-page-client.tsx` |
  | `hooks/domains/workspace/use-external-vcs-file-link.ts` | `useExternalVcsFileLinkHydration` in `components/task/task-page-content.tsx`, gated on the task having a GitLab-provider repository |

  The chip SHALL NOT call `useWorkspaceMRs`.
- There is no shared MR feedback cache. `useMRFeedback` is a per-instance
  reducer with no store slice, so there is nothing for the chip to warm and
  nothing the topbar would inherit from it. The chip SHALL NOT add a warmer.

**Two** fetch triggers are genuinely new, and both are accepted rather than
hidden.

**New trigger 1: the automation options.** The chip calls
`useTaskMRAutomationOptions(taskId)` to render its badges, and that hook lazily
issues a `GET` when the store holds no options for the task. Before this feature
those options were fetched only when the topbar dropdown or popover mounted
`MRAutomationControls`; now they are fetched for any session view that has a
linked open MR.

**New trigger 2: the GitLab connection status, which also has a cross-surface
side effect.** The chip calls `useGitLabAvailable()` to source `canLink`. That
helper (`hooks/domains/gitlab/use-task-mr.ts`) wraps `useGitLabStatus()`, whose
effect (`hooks/domains/gitlab/use-gitlab-status.ts`) runs on **every mount** and
unconditionally does `setStatus(workspaceId, null)` and then
`void loadStatus(workspaceId)` -> `fetchGitLabStatus`. Two consequences, both
stated rather than discovered at Build time:

1. **It is a request, not a cache read.** Despite the hook's "store-cached" doc
   comment, the guard is per-hook-instance: a populated store does not
   short-circuit the effect. Every chip mount issues one `GET`, and two mounted
   chips issue two.
2. **It transiently blanks a value other surfaces read.** Because the effect
   clears the status to `null` before refetching, every other
   `useGitLabAvailable()` consumer — `hooks/use-nav-availability.ts`,
   `components/kanban-external-link-availability.ts`,
   `components/task/task-session-sidebar-task-linking.ts`, and
   `components/app-sidebar/sections/settings/workspaces-group.tsx` — reads
   `false` for the duration of the in-flight request. Mounting the chip
   therefore has a brief, observable effect on unrelated navigation UI.

This is **accepted, not fixed here.** The chip is not the first consumer to do
it (`MRTopbarButton` already calls `useGitLabAvailable()` on the same task
route), the window is one request long, and the recovery is automatic when the
status resolves. Removing the blanking means changing `useGitLabStatus` for
every caller, which is its own card (see Out of scope). The chip SHALL NOT work
around it with a local cache, a mount-order guard, or a debounce, and SHALL NOT
read the status slice directly to dodge the hook. What the chip owes here is
only that `canLink` is allowed to be `false` on the first render and become
`true` without a remount — see Failure modes.

The popover's MR feedback read is **not** a new trigger in this sense. It is
`MRCIPopover`'s own existing fetch, it stays gated on that component's `enabled`
prop, and the chip passes its disclosure-open state there (see the prop table
under API surface). So it fires when a user opens the chip's popover or drawer,
exactly as it already fires when a user opens the topbar's, and never on mount.

What that does and does not buy across two mounted chips, stated precisely
because the obvious argument is invalid:

- **Guaranteed.** A chip whose disclosure is closed passes `enabled: false` and
  issues no feedback read. A chip that is never opened never fetches feedback.
- **NOT guaranteed.** Two chips mounted for one task may each have an open
  disclosure at the same time, and then each issues its own feedback read. An
  earlier draft argued they "do not double it, because at most one disclosure is
  open per chip" — that is a **non-sequitur** and is retracted: per-*chip*
  exclusivity gives no cross-*chip* exclusivity, and one open disclosure on each
  of two chips is exactly two concurrent `enabled: true`.

**Simultaneous open across two chips IS permitted.** This is a decision, not an
omission. Each mounted chip owns its own disclosure state (see Accessibility)
and this spec adds **no cross-chip coordinator**, no shared "currently open
chip" context, and no global registry. The state is reachable in practice
because the fine-pointer chip opens on **focus** as well as hover: a keyboard
user can focus chip A open and then hover chip B, leaving both open.

Two doubled feedback reads is the accepted cost. The alternative — a global
mutual-exclusion coordinator — would be new shared state that neither mount
point nor `MRTopbarButton` has today, would have to decide which chip loses,
and would make each chip's behaviour depend on what else is mounted. That is a
worse contract than an occasional duplicate idempotent `GET`. `useMRFeedback` is
per-instance with no store slice, so two concurrent reads cannot corrupt each
other; they resolve independently into their own popovers.

The dedupe this buys, stated precisely rather than optimistically:

- **Guaranteed.** Once options for a task are in the store, any number of
  further chip mounts for that task issue **zero** requests. The hook's effect
  is gated on `options || loading || error`, and a populated store short-circuits
  it on the first render.
- **Guaranteed.** A single chip mount issues at most one request.
- **NOT guaranteed, and not fixed by this card.** Two chips mounted for the same
  task in the **same React commit**, with no cached options, may each issue one
  request. The hook's guard reads render-captured values, so both effects run
  before either re-render observes `loading === true`. This is reachable:
  `PassthroughToolbar` has five mount sites and `ChatInputArea` three, several
  of them dockview-driven, so nothing structurally prevents two chips for one
  task existing at once.

The duplicate is a `GET` with no side effect, and the hook's own request-id
check (`isCurrentAndUnchangedExternally`) means the store converges to one
options object regardless of which response lands last, so the outcome does not
depend on ordering. Adding a shared in-flight guard to
`useTaskMRAutomationOptions` would fix it, but that hook is also used by
`MRAutomationControls` from the topbar, so the change belongs to a card that can
test both callers; it is named in Out of scope. The chip SHALL NOT work around
the gap with a module-level cache of its own.

The same render-captured pattern is present in `useTaskCIAutomationOptions`, so
`PRStatusChip` does not deliver cross-mount dedupe either. Parity with GitHub is
recorded here as an observation, not as evidence that the property holds.

**Hydration ownership, and why this spec pins no outcome for it.** An earlier
draft claimed `MRTopbarButton` was the only mount point for `useWorkspaceMRs`
and derived from that a specific consequence: that an archived task never
hydrates the MR store, so the chip renders nothing there. **Both halves were
wrong** and the claim is retracted. There are four call sites (table above), and
one of them — `useExternalVcsFileLinkHydration` at
`components/task/task-page-content.tsx` — runs on the `/tasks/:id` task page for
any task with a GitLab-provider repository, independently of the topbar and
independently of archived state.

So whether the MR store is hydrated for a given task depends on which surfaces
the session has visited and which route rendered the chip, not on a single
owner. This spec therefore states no archived-task rendering outcome and **no AC
depends on one.** What the chip guarantees is only the conditional already in
Failure modes: if the task's MR list is not hydrated, `useTaskMRs` returns the
shared empty array and the chip renders nothing, exactly as it does for a task
with no MRs. Tests SHALL seed the store rather than rely on any ambient
hydration path, which the E2E decision already requires.

Giving GitLab MR hydration a single provider-level owner is its own card (see
Out of scope). That card is what would make a hydration outcome specifiable;
until it lands, asserting one here would pin a route-history-dependent flake.

## Responsive behaviour

- Disclosure is chosen by pointer precision only, via `useTouchDrawer()`
  (`!isFinePointer`): coarse pointer renders the `Drawer` variant, fine pointer
  renders the hover `Popover` variant.
- The chip SHALL NOT gate any part of itself on a Tailwind width class
  (`sm:`, `md:`, ...). Pairing a width-gated class with the pointer-gated hook
  is what leaves 640-767px rendered by neither branch, and the chip has no
  width-dependent behaviour to justify the risk.
- The drawer variant carries a translated title, a screen-reader description,
  and a close button, and its body scrolls independently.
- **Hover-popover placement is pinned by value**, so two builders cannot diverge
  visually while passing every scenario: `align="end"`, `sideOffset={4}`, and a
  `w-80` content width — the values `MRTopbarButton` already uses. `side` is
  left to Radix's default collision handling, which is what keeps the content on
  screen for a chip sitting at the bottom of the viewport; the "bounding box `y`
  is greater than or equal to 0" scenario below is the observable consequence of
  that and SHALL NOT be satisfied by hard-coding a `side`.
