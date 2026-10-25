---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Accessibility, focus, and duplicate mounts

- The trigger is a `<button type="button">` with `cursor-pointer` and a
  translated `aria-label` naming the MR (or the MR count) and its status. The
  drawer variant additionally carries `aria-haspopup="dialog"` and
  `aria-expanded`.
- **The `aria-label` describes the acted-on MR**, so it follows the same regime
  as `data-mr-iid`: the live selected MR while the disclosure is closed, the
  frozen MR while it is open. It therefore always agrees with `data-mr-iid` and
  always names the MR whose controls the disclosure actually operates on. It
  does NOT follow `data-status`'s live tracking, because a label that renamed
  the MR under a screen-reader user mid-interaction would describe controls
  other than the ones in front of them.
- The fine-pointer hover lifecycle SHALL be the existing
  `useHoverPopover({openDelayMs, closeDelayMs, disabled})`
  (`apps/web/hooks/domains/github/use-hover-popover.ts` — note it takes a single
  options object, not three positional arguments), wired exactly as
  `MRTopbarButton` wires it. That wiring is, in full and with nothing omitted —
  **thirteen** handlers, not the four-plus-two a reader might assume:

  | Element | Event | Handler |
  |---|---|---|
  | trigger | `onMouseOver` | `onTriggerEnter` |
  | trigger | `onMouseEnter` | `onTriggerEnter` |
  | trigger | `onMouseMove` | `onTriggerEnter` |
  | trigger | `onPointerOver` | `onTriggerEnter` |
  | trigger | `onPointerEnter` | `onTriggerEnter` |
  | trigger | `onPointerMove` | `onTriggerEnter` |
  | trigger | `onFocus` | `onTriggerEnter` |
  | trigger | `onMouseLeave` | `onTriggerLeave` |
  | trigger | `onPointerLeave` | `onTriggerLeave` |
  | trigger | `onBlur` | `onTriggerLeave` |
  | content | `onMouseEnter` | `onContentEnter` |
  | content | `onMouseMove` | `onContentEnter` |
  | content | `onMouseLeave` | `onContentLeave` |

  **The redundancy is deliberate and SHALL be reproduced, not tidied.** An
  earlier draft of this spec listed only six of these rows while also declaring
  the list complete; that was wrong, and a builder who implements six rows ships
  a chip that is measurably flakier than the topbar rather than identical to it.
  Two of the omitted rows are individually load-bearing:

  - **`onMouseMove` on the content.** `use-hover-popover.ts`'s own doc comment
    states the reason: "The content also treats mouse-move as 'enter' so a flaky
    or missed portal mouseenter can't strand a pending close." This is precisely
    the mechanism the "pointer crosses the gap" scenario below depends on.
    Without it that scenario does not fail cleanly — it flakes.
  - **`onMouseMove` / `onPointerMove` on the trigger.** These re-assert hover for
    a pointer that is resting on the trigger without generating a fresh
    `mouseenter`, which is the state the closed-trigger click scenario describes.

  The remaining aliases (`onMouseOver` / `onPointerOver` / `onPointerEnter`, and
  `onPointerLeave`) are defensive duplicates covering browsers and input devices
  that dispatch the pointer family but not the mouse family, or vice versa.
  `onTriggerEnter` and `onTriggerLeave` are idempotent — they set a boolean ref
  and schedule or cancel a timer — so firing them several times for one physical
  gesture is harmless by design, and that is what makes the duplication safe.

  Because reproducing thirteen rows by hand is exactly the kind of thing that
  drifts, the chip SHOULD prefer **exporting and reusing** the existing private
  `useMRPopoverInteractions` wrapper (and, if it is convenient, the trigger's
  handler-spreading shape) from `mr-topbar-button.tsx` over re-typing the set.
  Reuse is the preferred closure of this requirement; hand-copying is permitted
  only if every row above is present.

  The content handlers are what let the pointer cross the `sideOffset` gap
  without closing. `disabled` is `useTouchDrawer()`. `openDelayMs` and
  `closeDelayMs` SHALL both be **150**, the values `MRTopbarButton` uses. Those
  constants live in a private `useMRPopoverInteractions` wrapper inside
  `mr-topbar-button.tsx`, so "wired as the topbar wires it" does not by itself
  pin them; they are pinned here by value. The chip MAY either export that
  wrapper for reuse or wire `useHoverPopover` directly with the same values.
- **The popover DOES close on blur**, because `onBlur` is wired to
  `onTriggerLeave` exactly as the topbar wires it. It also closes on Escape, on
  outside interaction (Radix `onOpenChange`), and when the pointer leaves both
  the trigger and the content. It opens on keyboard focus of the trigger and
  SHALL NOT steal focus when it opens.

  This corrects a claim an earlier draft of this spec made. For the record,
  because the wrong version is the kind of thing that gets re-derived: it is
  true that `useHoverPopover` itself declares no blur handler, but
  `MRTopbarButton` wires `onBlur` to the hook's `onTriggerLeave` at the
  component level. So wiring blur is what matches the topbar and omitting it is
  the divergence. Nor does wiring it re-introduce the portal race:
  `onTriggerLeave` does not close anything directly, it calls `scheduleClose`,
  which re-checks `overTrigger` and `overContent` at the moment the timer fires
  and commits the close only when the pointer is over neither region. That
  re-check is precisely the mechanism the hook exists to provide.

  Closing on blur does not resurrect the keyboard-operability problem, because
  this spec does not claim the popover's controls are Tab-reachable — see the
  next bullet, which states the opposite plainly. Focus-to-open makes the
  content *readable* by a keyboard user; tabbing away closes it again; the
  controls inside it were never Tab-reachable from the trigger in the first
  place, so nothing is lost by the close.
- **Scope of the keyboard claim.** Focus-to-open makes the popover's *content
  readable* by a keyboard user. It does NOT make the popover's controls
  (link, unlink, merge) Tab-reachable: Radix portals the content to the end of
  the document and provides no tab bridge from the trigger, so Tab moves past
  the chip rather than into it. This spec does not claim otherwise, and does not
  fix it. No capability is lost: link and unlink are keyboard-operable today
  through `MRTopbarButton`'s click-driven `DropdownMenu`, and on a coarse
  pointer the chip's own drawer is a focus-trapping dialog in which every
  control is operable. Making `MRCIPopover`'s controls keyboard-reachable from a
  hover popover is a pre-existing gap shared with `PRStatusChip` and the topbar,
  and is named in Out of scope.
- **Clicking the fine-pointer trigger is a no-op in both states.** While the
  popover is open, it stays open and nothing navigates; Radix treats the trigger
  as outside the content, so this requires an explicit outside-pointer-down
  guard. While the popover is closed, the click also does nothing on its own:
  the fine-pointer chip opens on hover and focus only, and there is no
  click-to-open path. A user who clicks a closed chip faster than the 150ms
  open delay therefore sees the popover open on the hover timer as normal,
  because the pointer is still over the trigger; the click neither accelerates
  nor suppresses it. This is a deliberate divergence from `MRTopbarButton`,
  whose trigger navigates to the MR detail panel on click — the chip has no
  detail-panel action at all (see Out of scope), so it has nothing to do with a
  click.
- **Closing the drawer returns focus to the trigger when the trigger is still
  mounted.** It often will not be: unlinking the task's last open MR from inside
  the drawer, or that MR going terminal, unmounts the chip and its trigger in
  the same update that closes the drawer. In that case the chip makes no focus
  claim and focus falls to `document.body` per browser default. This is a named,
  accepted consequence rather than a specified behaviour, it is shared with
  `PRStatusChip`, and improving it is listed in Out of scope. No AC asserts a
  focus target for the unmounted case.
- The two mount points are alternative surfaces, but nothing guarantees only one
  is mounted at a time. Two `mr-status-chip` elements may therefore coexist in
  the DOM. Both are correct and identical; E2E selectors SHALL scope to
  `chat-status-bar` or `passthrough-status-row` rather than matching the testid
  globally, and each mounted chip owns its own disclosure state.
- On a workspace switch the chip does not show the previous workspace's MRs.
  **The mechanism is workspace-scoped selection, not a reset of the outgoing
  bucket** — an earlier draft of this spec said the opposite and it was wrong.
  What actually happens: `useWorkspaceMRs` calls `resetTaskMRs(workspaceId)`
  with the **incoming** workspace id (the all-buckets `resetTaskMRs()` runs only
  when the workspace goes null), so the outgoing workspace's bucket is left in
  the store untouched. The chip reads nothing from it because `useTaskMRs`
  selects `byWorkspaceId[activeWorkspaceId]`, and `activeWorkspaceId` is already
  the incoming one. The chip therefore unmounts until the incoming workspace's
  MRs land.
  The correction matters for what it forbids: the chip SHALL NOT add any
  outgoing-workspace cleanup of its own, and no AC asserts that the previous
  workspace's bucket was cleared.
