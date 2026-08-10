---
status: draft
created: 2026-08-09
updated: 2026-08-10
owner: nova28
---

# Per-workflow step visibility filter on the kanban board

## Amendment R2 — 2026-08-10 (review follow-ups, same PR)

Everything below the R1 line was locked across five Spec Review rounds. R2 leaves R1's
**board behaviour and its persisted contract** exactly as they were — nothing changes
about which tasks are hidden, which columns render, what is stored, or what goes on the
wire. R2 does change one thing R1 specified: how the Steps section *discloses* its rows
when more than one workflow is eligible (item 3 below). That is a deliberate, scoped
change to R1's rendering, and this amendment carries its consequences — it amends R1's
checkbox-observation AC and requires an expand step in R1's desktop E2E. Read any "R1 is
untouched" statement below as scoped to board behaviour and persistence, never to the
Steps section's disclosure. R2 was raised by `carlosflorencio`'s
review of PR [#2467](https://github.com/kdlbs/kandev/pull/2467)
([comment 1](https://github.com/kdlbs/kandev/pull/2467#issuecomment-5233457869),
[comment 2](https://github.com/kdlbs/kandev/pull/2467#issuecomment-5233467664)) and
**lands inside that same PR**, before it merges — so `main` never carries the feature
in a state this amendment describes as broken.

R2 does four things and no more:

1. **Adds the phone surface** ([Display surfaces](#display-surfaces-desktop-tablet-mobile)).
   R1 assumed one Display surface; there are two, and the phone gets the other one, so
   on a phone the Steps filter is currently **unreachable** while its predicate still
   applies to the phone board. This is a correctness gap in R1's own contract, not new
   scope: R1 already states "the group set is the same on mobile as on desktop".
2. **Moves two pure selectors out of the component layer**
   ([Implementation layering](#implementation-layering)), and repoints every R1
   sentence that named their old home. Pure move, no behaviour change.
3. **Locks a disclosure rule for the Steps section**
   ([Steps-section disclosure](#steps-section-disclosure--density)), so N workflows ×
   S steps of always-expanded checkboxes does not dominate the dropdown or the phone
   drawer. This governs *disclosure*, never the *ticked* default — every step is still
   shown/ticked by default, exactly as R1 requires.
4. **Extends the E2E requirement** with the mobile spec and a required Vitest list, and
   records its declines and residues as named exclusions under
   [Out of scope](#out-of-scope) — including the two relocations raised in review
   (`useMultiSelectDerived`, `filterTasks`) and one **accepted residue**: a step hidden
   inside a `workflow.hidden` workflow stays restorable only from the desktop/tablet
   Workflow filter, never from a phone. That residue is real, it is the one place where
   this amendment's "no state the phone cannot undo" framing does not reach, and it is
   named rather than papered over.

**Delivery:** commit onto `feature/board-filter-per-wor-p2u` (PR #2467's head branch);
do not open a second PR. Keep the phone surface, the selector move, the disclosure
rule, and this spec edit as separately reviewable commits so a reviewer can accept or
reject the disclosure rule without unpicking the rest.

---

## Why
Users want to declutter the kanban board by hiding tasks that sit in steps they
do not currently care about. The motivating request is "show all tasks except
the ones in Done" — but it generalises to any step on any board (hide `Backlog`,
hide `Review`, etc.).

There is deliberately **no** cross-workflow semantic layer to lean on:

- `workflow_steps.stage_type` is `"custom"` on nearly every step. The office /
  workflow engine *does* branch on `stage_type` in places (e.g. review-phase
  routing in `apps/backend/internal/office/service/prompt_builder.go` and
  `scheduler_integration.go`), but that signal is meaningless on the kanban board:
  because nearly every step is `"custom"`, the board has no reliable per-column
  phase signal and cannot know which column is "the Done phase". The board UI must
  therefore never classify columns by `stage_type` (see the field comment in
  `apps/web/lib/state/slices/kanban/types.ts`).
- The same step title in two workflows is not the same step — `Review` in one
  workflow has a different id, prompt, and meaning than `Review` in another.
- `task.state` is the agent-session lifecycle, not the board column, and the two
  can disagree (a task can sit in the Done column with `state: REVIEW`). Filtering
  on `state` would misreport what is actually in a column.

The only ground truth is: each workflow owns an ordered set of steps, and a task
is in exactly one step of exactly one workflow. So the feature is, precisely,
**per-workflow column show/hide keyed on the real `workflowStepId`.**

## What
- The kanban board's **Display** surface SHALL gain a new "Steps" section, a sibling
  to the existing Workflow and Repository sections. On desktop and tablet that surface
  is the Display dropdown (`apps/web/components/kanban-display-dropdown.tsx`); on a
  phone it is the mobile menu drawer's Display-options block
  (`apps/web/components/kanban/mobile-menu-sheet.tsx`). The two render the **same**
  section from one shared component and are mutually exclusive — see
  [Display surfaces](#display-surfaces-desktop-tablet-mobile) (R2).
- The Steps section SHALL present **one group per workflow selected by
  `selectWorkflowSwimlanes`** (`apps/web/lib/kanban/workflow-swimlanes.ts` — see
  [Implementation layering](#implementation-layering)) — the board's *eligible*
  workflow set (workflows with a
  loaded snapshot, honouring the active Workflow filter), NOT the post-filter set the
  board actually renders. This distinction is load-bearing: a workflow whose every step
  is hidden is dropped from the rendered board (it has zero visible tasks) but its group
  MUST remain in the Steps section so the user can re-tick a step (see
  [Steps-section scope](#steps-section-scope)). Within each group the section SHALL list
  that workflow's steps as checkboxes in `position` order (tiebreak by `id`, see
  [Ordering](#ordering--determinism)). The group set is the same on mobile as on desktop:
  the mobile board's single-workflow focus navigator changes which board is rendered, not
  which groups the Steps section offers.
- Every step SHALL be **shown (ticked) by default**. A step is hidden only when
  the user explicitly unticks it. ("Ticked" is the checkbox state and is unconditional.
  Whether a workflow group's rows are *disclosed* by default is a separate, purely
  visual question answered in
  [Steps-section disclosure](#steps-section-disclosure--density) — R2.)
- Unticking a step SHALL, on every board surface that renders that workflow
  (single-workflow board and multi-workflow swimlane, kanban and pipeline views):
  1. **hide that step's tasks**, and
  2. **collapse (remove) that step's now-empty column** — not merely empty it.
  Both effects are required together (see
  [The dual-filter contract](#the-dual-filter-contract)).
- The filter SHALL be **scoped strictly per workflow id**: hiding step `S` in
  workflow `A` SHALL NOT affect any step of workflow `B`, including a step in `B`
  that shares `S`'s title. Cross-workflow bleed SHALL be impossible by
  construction (the selection is keyed by workflow id, and a task is matched only
  against its own workflow's hidden set).
- Re-ticking a step SHALL restore its column and its tasks with no other side
  effect.
- The selection SHALL **persist** across reloads and sessions in backend user
  display settings (the same tier as the Workflow and Repository filters), not
  session-only.
- The selection SHALL track step **id**, never title, so renaming a step keeps
  its hidden/shown state.
- The Steps filter SHALL **compose (AND)** with the Workflow filter, Repository
  filter, search query, and any plugin task filters: a task is visible only if it
  passes all of them.

## Data model & state shape

### Store (frontend)
`UserSettingsState` (`apps/web/lib/state/slices/settings/types.ts`) gains one
field holding the **hidden** set, keyed by workflow id:

```
hiddenWorkflowStepIds: Record<string /* workflowId */, string[] /* hidden stepIds */>
```

Rationale for storing *hidden* (not *shown*): a step absent from the map defaults
to visible, so a newly-added step is shown by default rather than silently hidden,
and the map only grows with explicit user choices.

The persisted/serialized form is arrays (JSON-friendly). The runtime predicate
MAY build a `Set` per workflow for O(1) membership; the "hidden set" in the
locked design refers to this membership semantics, realised on the wire as a
`string[]`.

Normalisation: within each workflow's array the ids SHALL be de-duplicated and sorted
ascending (mirroring how `buildNormalizedSettings` normalises `repositoryIds` via
`Array.from(new Set()).sort()`), so the serialized form is canonical; a workflow whose
array becomes empty SHALL be removed from the map entirely (no empty-array keys
persisted), mirroring how the plugin-filter store drops an empty selection. Because the
persisted form is already sorted, `isSettingsUnchanged`'s order-insensitive per-workflow
comparison is stable.

### Persistence round-trip (exact contract)
The setting persists through the same path as the other display filters. All
field names below are the contract; they exist so Build is not forced to invent
them.

- **DB / wire (snake_case): `kanban_hidden_step_ids`**, typed as a JSON object
  `map[string][]string` (workflowId → hidden stepIds).
  - `apps/backend/internal/user/models/models.go` — `UserSettings.KanbanHiddenStepIDs map[string][]string` (`json:"kanban_hidden_step_ids"`).
  - `apps/backend/internal/user/dto/dto.go` — add to `UserSettingsDTO` (value) and `UpdateUserSettingsRequest` (as `*map[string][]string`, `,omitempty`, so an absent field is not treated as "clear"); map it in `FromUserSettings` (the `*models.UserSettings → UserSettingsDTO` mapper — there is no `ToAPI`). Cited by symbol, not line: an earlier revision said `dto.go:217` and the function has since drifted to `:219`.
  - `apps/backend/internal/user/controller/controller.go` and `.../service/service.go` — copy the field in the update path (`if req.KanbanHiddenStepIDs != nil { settings.KanbanHiddenStepIDs = *req.KanbanHiddenStepIDs }`), include it in the settings event data map and in the boot-state settings map.
  - `apps/backend/internal/user/store/sqlite.go` — include it in the marshalled payload and the scan struct (settings persist as a JSON blob in `users.settings`).
  - `apps/backend/internal/backendapp/boot_state_routes.go` (`mapUserSettingsState`, and `boot_state.go` if it mirrors the map) — emit under the boot key **`hiddenWorkflowStepIds`**.
- **Boot payload key: `hiddenWorkflowStepIds`** — the **store-field name**, NOT the
  camelCased wire name. The boot payload's `userSettings` object is deep-merged into
  the store by direct key match (`deepMerge(draft, source)` in
  `apps/web/lib/state/hydration/hydrator.ts`), with no snake→camel remapping, so the
  emitted key MUST equal the Zustand field. This matches the existing precedent:
  `mapUserSettingsState` emits `WorkflowFilterID` under `workflowId` and `RepositoryIDs`
  under `repositoryIds` — store names, not `workflowFilterId` / `repository_ids`. Emitting
  `kanbanHiddenStepIds` (the DTO json name camelCased) would write a dead key that no
  selector reads, leaving `hiddenWorkflowStepIds` at `{}` on every cold boot.
- **Frontend hydration:** the REST / SSR path (`apps/web/lib/ssr/user-settings.ts`,
  `mapUserSettingsData`) reads the **snake_case wire** field and maps
  `s.kanban_hidden_step_ids ?? current.hiddenWorkflowStepIds` into
  `hiddenWorkflowStepIds`; the default is `{}`. (Boot payload uses the store-name key
  above; REST uses snake_case — both resolve to the same store field.)
- **Frontend persist:** `apps/web/hooks/use-user-display-settings.ts` —
  `CommitPayload` gains the field, `buildNormalizedSettings` normalises it,
  `isSettingsUnchanged` compares it (deep, order-insensitive per workflow), and
  `persistSettingsPayload` sends `kanban_hidden_step_ids` in the
  `user.settings.update` payload.
- **WS echo:** the `user.settings.updated` broadcast handler
  (`apps/web/lib/ws/handlers/users.ts` — the server echo, distinct from the
  `user.settings.update` *request* action the client sends via `persistSettingsPayload`)
  that refreshes `userSettings` from a server echo SHALL carry the new field through the
  same mapping used at REST hydration.

Postgres compatibility: the field is stored inside the existing JSON settings
blob, so no new column and no dialect-sensitive SQL is introduced.

## Steps-section scope
The Steps section mirrors the board's own workflow grouping so it never offers
controls for boards the user is not looking at:

- **Workflow filter = "All Workflows":** one group per workflow that has a loaded
  snapshot and is not hidden — i.e. the set returned by
  `selectWorkflowSwimlanes(null, workflows, snapshots)` in
  `apps/web/lib/kanban/workflow-swimlanes.ts` (R2; it lived in
  `apps/web/components/kanban/swimlane-container.tsx` in R1).
- **Workflow filter = a specific workflow:** exactly one group, for the selected
  workflow (hidden or not), matching `selectWorkflowSwimlanes(workflowId, …)`.
- **(R2) The two bullets above are deliberately asymmetric about `workflow.hidden`,
  and that asymmetry is pre-existing R1/upstream behaviour, not something R2 changes.**
  `selectWorkflowSwimlanes`'s explicit-filter branch returns the selected workflow
  *hidden or not*, while its "All Workflows" branch filters on `!workflow.hidden`. Because
  the Display dropdown's Workflow `<Select>` is built from the **unfiltered** workflow
  list, a desktop or tablet user can select a hidden workflow (e.g. Improve Kandev), get
  its group, and hide one of its steps — and that step is then **not** re-tickable from
  "All Workflows" on any surface, nor from a phone at all. The consequences of that are
  named and accepted under
  [Out of scope → hidden-workflow steps](#out-of-scope); Build SHALL implement the two
  bullets exactly as written and SHALL NOT widen either branch's eligibility to close it.
- Group membership is independent of the Repository filter, the search query, the
  render-time `getVisibleWorkflows` drop, and the mobile single-workflow focus
  navigator (those hide tasks or narrow the rendered board, not the Steps controls).
  **A non-hidden** workflow group with all-hidden steps still lists its steps so the user
  can re-tick them, even though that workflow is no longer rendered on the board. **(R2)
  This recoverability guarantee is scoped to non-hidden workflows on purpose** — it is
  delivered by the "All Workflows" bullet above, which excludes `workflow.hidden`
  workflows from eligibility, so the guarantee cannot and does not extend to them. Do not
  read it as an unconditional promise; the hidden-workflow case is the named exclusion
  linked in the previous bullet.
- Each group's steps come from that workflow's snapshot `steps`
  (`kanbanMulti.snapshots[workflowId].steps`), the same source the board renders,
  and are listed in `position` order (see [Ordering](#ordering--determinism)).
- The synthetic **"Needs Reassignment"** orphan column (`ORPHAN_STEP_ID`,
  `apps/web/components/kanban/swimlane-kanban-content.tsx`) is a display-only
  fallback and is NOT a real step: it SHALL NOT appear in the Steps section and
  SHALL NOT be hideable.

## Steps-section selectors
The new Steps-section controls SHALL carry stable `data-testid`s so the required
Playwright spec has a selector contract (matching the testid-precision of the rest of
this spec — `kanban-column-<stepId>`, `task-context-step-<stepId>`,
`bulk-move-step-<stepId>`, `display-button`). Build MAY choose the exact copy but NOT
drop these ids:

- Each per-workflow group container: `data-testid="steps-filter-group-<workflowId>"`.
  This container is present whether the group is expanded or collapsed (R2).
- Each per-step checkbox: `data-testid="steps-filter-step-<stepId>"`, reflecting
  checked/unchecked state through the control's standard checked semantics
  (e.g. `aria-checked` / `data-state`) so a test can assert ticked vs unticked. This is the
  **state locator**, and it MAY be the checkbox control itself.
- **(R2)** Each per-step **interactive row** — the full-width clickable element (the
  `<label>` wrapping that checkbox) that a user actually taps:
  `data-testid="steps-filter-step-row-<stepId>"`. This is the **geometry locator**, and it
  is the element the ≥ 44 px assertion measures. It is a separate id from the state
  locator on purpose: R1 anchored `steps-filter-step-<stepId>` to the control so checked
  state is assertable, and a ~20 px control is not the tap target. Build SHALL emit both;
  clicking either toggles the step, because the row is the checkbox's label.
- **(R2)** Each per-workflow disclosure header, where one is rendered:
  `data-testid="steps-filter-group-toggle-<workflowId>"`, with `aria-expanded`
  reflecting the group's disclosure state. This element is **both** the state locator and
  the geometry locator for the header — it needs no separate row id, because the header
  control *is* the interactive row.

Step ids are the real `workflowStepId` / `workflow.id`, never the title. The synthetic
"Needs Reassignment" orphan column (`ORPHAN_STEP_ID`) has no Steps-section control and
therefore no such testid.

**(R2)** These ids are identical on both Display surfaces. Because at most one surface
renders the section at any breakpoint (see below), a test never faces two live copies
and Playwright strict mode is never violated.

## Display surfaces (desktop, tablet, mobile)
**Added by R2.** R1 named only the Display dropdown. There are two Display surfaces and
the phone gets the other one:

- `apps/web/components/kanban/kanban-header.tsx` branches on
  `useResponsiveBreakpoint()`: `isMobile` (< 768px) → `KanbanHeaderMobile`; `isTablet` →
  `TabletHeader`; otherwise `DesktopHeader`.
- `TabletHeader` and `DesktopHeader` each render `KanbanDisplayDropdown`
  (`data-testid="display-button"`). **`KanbanHeaderMobile` renders neither** — the phone's
  display settings live in `MobileMenuSheet` → `MobileDisplayOptions`, reached from the
  topbar hamburger ("Open menu").
- The hidden-set predicate is viewport-independent: `useSwimlaneData`
  (`swimlane-container.tsx`) reads `userSettings.hiddenWorkflowStepIds` unconditionally and
  the phone board renders through the same `SwimlaneContainer` → `WorkflowItemContent` path.

So without R2 a user who hides a step on desktop, on a tablet, or by resizing past 768px
loses those columns and tasks on their phone **with no control anywhere on the phone to
restore them**. That is a trapdoor, and it contradicts R1's own statement that "the group
set is the same on mobile as on desktop".

Requirements:

- The Steps list body SHALL be **one** shared presentational component
  (`apps/web/components/kanban/steps-visibility-section.tsx`) consumed by both
  `kanban-display-dropdown.tsx` and `mobile-menu-sheet.tsx`. Given the same
  `(eligibleWorkflows, snapshots, hiddenWorkflowStepIds, overrides)`, the rendered group
  set, step set, ordering, disclosure state, and checked state SHALL be identical on both.
  Build SHALL NOT fork a phone-only implementation.
- **(R2) `overrides` is the fourth input, and it is per-surface — this requirement pins the
  shared rendering FUNCTION, not output equality between two live surfaces.** The bullet
  above originally named only three inputs and said "disclosure behaviour", which read as a
  promise that the two surfaces always *show* the same expansion state. They do not, and
  they must not: the override map is scoped to one visit to one surface and NEVER crosses
  between them (see
  [Steps-section disclosure → Lifetime](#steps-section-disclosure--density)), so the same
  three persisted inputs can legitimately yield an expanded group on one surface and a
  collapsed one on the other at the same moment. Both surfaces resolve disclosure through
  the *same* `effectiveExpanded(workflowId) = overrides[workflowId] ?? defaultExpanded(workflowId)`
  rule, and that identity is what this requirement asserts. Build SHALL NOT hoist the
  override map into the store, a cross-surface context, or any other shared state in order
  to make the two surfaces agree — that would satisfy a misreading of this bullet while
  violating the lifetime rule.
- WHILE the viewport is mobile AND `currentPage === "kanban"`, `MobileDisplayOptions`
  SHALL render the Steps section **after** the Repository field and **before** the
  Preview-panel field, mirroring the dropdown's order (Workflow, Repository, plugin
  filters, Steps, Preview panel).
- The section SHALL render on **exactly one** surface at a time. IF the viewport is not
  mobile, THEN `MobileMenuSheet` SHALL NOT render it — the tablet branch renders
  `TabletHeader` (with the dropdown) *and* `MobileMenuSheet`, so an ungated mobile copy
  would duplicate `steps-filter-step-<stepId>`. IF `currentPage !== "kanban"`, THEN
  neither surface renders it, matching the dropdown's existing gate.
- Toggling on the phone SHALL invoke the same
  `onToggleStepVisibility(workflowId, stepId)` from `useKanbanDisplaySettings()`, with the
  same idempotency, normalisation and persistence contract as the dropdown. R2 introduces
  **no** new store field, wire field, or persistence tier.
- **Phone presentation is inline in the existing drawer** — not a nested drawer, not a
  route. The drawer's existing `min-h-0 flex-1 overflow-y-auto overscroll-contain` region
  remains the single scroll owner; the section adds no scroll container and no fixed
  height. The drawer already handles dynamic viewport height
  (`h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))]`) and safe-area padding. The
  section reuses the `mobileFieldClass` / `mobileFieldLabelClass` tokens from
  `mobile-menu-styles.ts`, and for checkbox-row geometry it copies **`MobileDisplayOptions`'
  List-rows field only** — the row whose class list is `flex min-h-11 …` (44 px). Do **not**
  copy the Preview-panel row: it is `flex h-10` (40 px) and would fail the ≥ 44 px
  requirement below. Note the List-rows field is gated behind
  `showTaskDetails: currentPage === "tasks"`, so it is **not rendered on the phone kanban
  page** where the Steps section lives — read it in `mobile-menu-sheet.tsx` rather than
  expecting to see it on screen next to the new section.
- Every interactive row in the section SHALL measure **≥ 44 CSS px** in height on a phone
  viewport. **(R2) The measured elements are named, not left to the test author:** every
  `steps-filter-step-row-<stepId>` and every `steps-filter-group-toggle-<workflowId>` that
  is in the DOM (see [Steps-section selectors](#steps-section-selectors)). The ≥ 44 px
  assertion SHALL NOT be made against `steps-filter-step-<stepId>`, which is the state
  locator and may be the ~20 px control itself — measuring it would fail a conforming UI.
- WHILE the section is rendered on a phone viewport,
  `document.documentElement.scrollWidth` SHALL NOT exceed its `clientWidth`, and long step
  titles SHALL truncate rather than widen the drawer.
- **(R2) Truncation is pinned, not left to Build.** A long step title SHALL truncate to a
  **single line with an ellipsis** (Tailwind `truncate`, i.e.
  `overflow-hidden text-ellipsis whitespace-nowrap`), and the element SHALL carry the
  **full, untruncated title in its `title` attribute** so the text is still recoverable.
  Multi-line clamping is NOT the contract: it satisfies the no-horizontal-overflow rule
  above while silently growing row height and leaving no way to read the full title, so the
  two treatments are observably different and only this one is conforming. Workflow names
  in a group header truncate the same way. This is observed by the Vitest item in
  [E2E requirement](#e2e-requirement); it is deliberately NOT asserted in Playwright,
  because a pixel-level ellipsis check is brittle across font rendering.
- On the phone board, a hidden step's column SHALL be **absent** and the mobile board
  navigator (`mobile-board-navigator` → `column-tab-*`) SHALL no longer offer that step.
  Note the phone mounts every column inside `SwipeableColumns` and reveals one at a time,
  so conformance is judged on `kanban-column-<stepId>` **count**, not visibility; and
  `column-tab-<index>` is positional over the already-collapsed `steps` array, so hiding a
  step renumbers the remaining tabs — assertions use step **titles**, never a fixed index.

## Steps-section disclosure & density
**Added by R2.** Not a layout bug: `@kandev/ui`'s shared `DropdownMenuContent` already
applies `max-h-(--radix-dropdown-menu-content-available-height)` with `overflow-y-auto`, so
the dropdown caps to the viewport and scrolls internally. The problem is information
architecture — R1 renders every eligible workflow's full checklist always expanded, i.e.
N × S rows of ~44px before the user has expressed interest in any workflow, and R2 puts
that same list into a phone drawer that already carries six sections.

Direction: **per-workflow collapsible groups, with the single-workflow case rendering
exactly as R1 does.** Chosen over "show N of X" truncation (which hides steps *within* the
workflow the user is already looking at — the case where they most need the full list) and
over a dedicated sub-surface (heavier than a checkbox list warrants).

- WHILE exactly one workflow group is eligible, the section SHALL render as R1 does: no
  group header, no disclosure control, **no shown-count summary**, all step rows inline.
- **(R2) The single-workflow rule WINS over the shown-count rule when they meet.** The
  shown-count summary lives in the group header, and with exactly one eligible workflow
  there is no header — so there is no summary either, *including* when that one workflow's
  snapshot has zero steps. In that case the section renders its `kanban:steps` label and
  its description with no step rows and no summary, which is what keeps the workflow from
  being silently absent from the filter surface. The "0 of 0 shown" reading in
  [Nil / empty / boundary](#nil--empty--error--defaults--boundary) therefore applies only
  where a header is rendered, i.e. when more than one workflow is eligible. Build SHALL NOT
  invent a headerless standalone summary to satisfy both rules at once.
- WHILE more than one workflow group is eligible, each group SHALL render a header control
  (`steps-filter-group-toggle-<workflowId>`) showing the workflow name, a shown-count
  summary, and an expand/collapse affordance, with correct `aria-expanded`.
- **Default disclosure:** a group SHALL default to expanded IF
  `hiddenWorkflowStepIds[workflowId]` contains at least one id matching a step in that
  workflow's current snapshot; collapsed otherwise. This guarantees that on every arrival at
  the surface, a hidden step is discoverable without the user having to hunt for it.
- **(R2) The discoverability guarantee binds the DEFAULT, not an explicit user collapse.**
  A user MAY collapse any group, including one holding a live hidden step. Its shown-count
  summary ("4 of 6 shown") stays visible and is the discoverability affordance in the
  collapsed state. Build SHALL NOT refuse, ignore, or auto-reopen a collapse on a
  hidden-bearing group, and SHALL NOT special-case such a group in any way. Rationale: a
  disclosure control that will not close is a worse surprise than a collapsed group whose
  own summary already reports that something is hidden, and the reset rule below means the
  expanded default reasserts itself the next time the surface opens.
- **Override state — a map, not a set.** User disclosure overrides SHALL be held as
  `Record<string /* workflowId */, boolean>` recording, per workflow, the **last explicit
  disclosure choice the user made** during the current visit to the surface: `true` for
  expand, `false` for collapse. It SHALL NOT be modelled as "the set of ids the user
  toggled". A bare set cannot encode direction, so the only set-coherent reading is
  "membership inverts the default" — and because the default itself moves with
  `hiddenWorkflowStepIds`, that reading collapses a group in response to the user's own
  checkbox click (see the coupling bullet below). The map exists precisely to make the
  user's intent survive a moving default.
- **Override resolution (exact):**
  `effectiveExpanded(workflowId) = overrides[workflowId] ?? defaultExpanded(workflowId)`.
  An entry is written ONLY by an explicit disclosure toggle on that workflow's header
  (`steps-filter-group-toggle-<workflowId>`), and once written it wins over the default for
  as long as it lives. **Each toggle writes the negation of the group's current effective
  disclosure:** `overrides[workflowId] = !effectiveExpanded(workflowId)`. Build SHALL
  compute it that way and SHALL NOT assume any fixed end value.
  **(R2) Toggling a group twice therefore leaves an EXPLICIT entry — never an absent key —
  whose value is whatever that group started at, now recorded explicitly.** For a group
  that started **collapsed** (the common case: nothing hidden, so `defaultExpanded` is
  false) two toggles leave `false`. For a **hidden-bearing** group, which defaults to
  *expanded*, two toggles leave `true`, not `false`. An earlier revision of this bullet
  asserted `false` unconditionally; that was wrong for the hidden-bearing case, and a unit
  test written to `expect(overrides[id]).toBe(false)` after two toggles would be asserting
  a defect. The load-bearing claim — the one this bullet exists for — is that the key is
  **present**, not that it holds any particular value: "revert to default", i.e. deleting
  the key, is not a reachable state within one visit and does not need to be.
- **(R2) A step toggle SHALL NOT override the user's own disclosure choice — but it MAY
  move the default.** These are two different claims and the headline used to state only
  the stronger one. Precisely:
  - **Where an override exists, disclosure does not change.** Ticking or unticking a step
    changes `hiddenWorkflowStepIds` and therefore changes `defaultExpanded`, but an explicit
    override wins over the default — so a group the user expanded stays expanded when they
    untick their first step in it, and stays open when they re-tick its last hidden step.
  - **Where no override exists, the recomputed default applies, and it is allowed to move
    the group.** On an untick it can only expand (a hidden step appearing makes the default
    expanded). On a re-tick it collapses the group **if and only if** that re-tick removed
    the workflow's last live hidden step. That collapse is intended, not a defect: the group
    closes on the click that produced it, which is the one visibly surprising interaction in
    this design, and it is observed by its own acceptance criterion in
    [R2 scenarios](#r2-scenarios). Build SHALL NOT suppress it, defer it, or animate around
    it into a different end state.
  Build SHALL NOT introduce any coupling between step toggles and disclosure beyond these
  two rules.
- **Lifetime — a visit is one surface, open once.** The override map is **ephemeral** and
  is scoped to a single visit to a single Display surface. It SHALL be reset on exactly two
  named events, and no others:
  1. **The Display surface closes** — the dropdown closing, or the mobile drawer closing.
     "Per mount" is NOT the contract. Build SHALL NOT rely on React unmount-on-close to
     achieve this — the dropdown unmounts its content on close but the drawer is not
     guaranteed to — so this reset SHALL be driven by the surface's own open/closed state.
  2. **(R2) The viewport crosses the 768px mobile boundary while a surface is open** —
     which hands the section from the dropdown to the drawer or back. Equivalently and
     more usefully stated: **disclosure overrides NEVER cross from one Display surface to
     the other.** Arriving at the other surface is a new visit, so every group resolves
     from `defaultExpanded` again.
     - **(R2) A crossing changes which surface OWNS the section. It SHALL NOT open or
       close either surface.** "Hands the section from the dropdown to the drawer" is a
       statement about ownership, not a transfer of open state: the dropdown and the drawer
       own their open state independently, and neither is opened, closed, or re-opened by a
       viewport change. So after a tablet→phone crossing with the dropdown open, the phone
       drawer is **closed** — exactly as it was — and the user opens it themselves.
     - **(R2) Which path actually distinguishes this trigger from trigger 1 — read this
       before writing the test.** In the tablet-dropdown → phone-drawer direction the
       destination is closed, so the user's next action is *opening* the drawer, which is a
       fresh visit that clears overrides under **trigger 1** whether or not trigger 2 was
       ever implemented. That direction therefore CANNOT distinguish the two triggers and
       SHALL NOT be the only coverage. The distinguishing path is a crossing with the
       surface **held open**: the drawer is open on a phone, the viewport widens into the
       tablet range — where `kanban-header.tsx` renders `TabletHeader` **and**
       `MobileMenuSheet` together, so the drawer's open state survives — and then narrows
       back. The drawer never closed, so only trigger 2 can have cleared the overrides. Its
       acceptance criterion is
       [(R2) Overrides do not survive a breakpoint crossing](#r2-scenarios).
  This is not a re-opening of the earlier "one trigger" simplification, which collapsed
  *per-mount vs on-close* on a **single** surface; trigger 2 is a different event — the
  surface itself changing — that the one-surface framing never covered. The practical
  consequence is that Build MAY own the map inside each surface's section owner rather than
  hoisting it above both header branches, and SHALL NOT hoist it into the store or any
  cross-surface context in order to preserve overrides across a crossing.
  The map is NEVER persisted: no wire field, no store field, no new concurrency surface.
- A group whose snapshot arrives while the surface is open computes its default at that
  moment by the same rule, and any existing override for that workflow still wins, so
  disclosure is a pure function of `(hiddenWorkflowStepIds, overrides)` and never depends on
  arrival order.
- WHILE a group is collapsed, its `steps-filter-step-<stepId>` checkboxes **and their
  `steps-filter-step-row-<stepId>` rows** SHALL NOT be in the DOM; its
  `steps-filter-group-<workflowId>` container and its
  `steps-filter-group-toggle-<workflowId>` header SHALL still be present.
- Expanding or collapsing SHALL have **no** effect on `hiddenWorkflowStepIds`, on the
  board, on move targets, or on any persisted value.
- The shown-count summary SHALL read the number of the workflow's **live snapshot** steps
  not in its hidden set, over the total number of live snapshot steps (e.g. "4 of 6
  shown"). Stale hidden ids SHALL NOT be counted, consistent with `H ∩ liveStepIds`. A
  workflow with zero steps reads "0 of 0 shown".
- R2 SHALL NOT add any `max-height`, fixed height, or nested scroll container to the Steps
  section on either surface.

## Implementation layering
**Added by R2.** R1 exported `selectWorkflowSwimlanes` from
`apps/web/components/kanban/swimlane-container.tsx` and imported it from
`apps/web/hooks/use-kanban-display-settings.ts` — a domain hook depending on a component
module. That is not only a layering smell: `use-kanban-display-settings.test.ts` does not
mock the component module (its only `vi.mock`s target `@/components/state-provider`,
`@/hooks/use-task-listing-view`, `@/hooks/use-user-display-settings`), so a hook unit test
transitively loads the whole swimlane component tree.

- `selectWorkflowSwimlanes`, `selectMobileNavigatorWorkflows`, and the shared
  `WorkflowLike` type SHALL move to **`apps/web/lib/kanban/workflow-swimlanes.ts`**, joining
  the existing pure selectors there (`filters.ts`, `resolve-workflow.ts`, `task-order.ts`,
  `mobile-column-index.ts`, `wip-limit.ts`), and SHALL NOT be exported from
  `swimlane-container.tsx`.
- **Both** selectors move. They share `WorkflowLike` and are the same concept ("which
  workflows does the board offer"); moving only one would leave `swimlane-container.tsx`
  duplicating the type or importing it back out of `lib/`, and would split one
  `describe`-pair across two files for no gain.
- `filterTasks` **stays** in `swimlane-container.tsx`. It is the dual-filter seam this spec
  pins there and it has no cross-layer importer.
- Import sites to update, verified by `git grep`: `swimlane-container.tsx` (self-use),
  `swimlane-container.test.ts` (its two selector `describe` blocks move to
  `apps/web/lib/kanban/workflow-swimlanes.test.ts` verbatim in assertion content, leaving
  its `filterTasks` coverage behind), and `use-kanban-display-settings.ts`.
  `use-kanban-display-settings.test.ts` names `selectWorkflowSwimlanes` only inside a test
  *title* string — no import, nothing to change. IF Build finds an importer outside this
  list, THEN it SHALL update that one too rather than assume the list is exhaustive.
- This is a **pure move**: no rendered output, ordering, or persisted value differs, and
  every acceptance criterion in this spec still holds afterwards.

## The dual-filter contract
This is the single most important behavioural detail and the easiest to get
wrong. For a workflow with hidden set `H`:

1. **Task hiding:** tasks whose `workflowStepId ∈ (H ∩ liveStepIds)` are removed from
   the tasks passed to the view — where `liveStepIds` is the set of `id`s in that
   workflow's current snapshot `steps`. The seam is `filterTasks` in
   `apps/web/components/kanban/swimlane-container.tsx`, applied per snapshot, AND scoped
   to the task's own `workflowId`. The intersection with `liveStepIds` matters ONLY for a
   stale hidden id (a hidden step that no longer exists): such an id hides nothing, so a
   task still pointing at it is left for orphan-remap exactly as with an empty hidden set
   (see [stale-id boundary](#nil--empty--error--defaults--boundary)). For any hidden step
   that still exists, `H ∩ liveStepIds` contains it, so its tasks are removed as required.
2. **Column collapse:** steps whose `id ∈ H` are removed from the `steps` list
   passed to the view component (the seam is the sorted `snapshot.steps` in
   `WorkflowItemContent`, `swimlane-container.tsx`). A stale id in `H` matches no rendered
   step, so this is a no-op for it.

Both are mandatory **and interdependent**:

- If only the column were removed but its tasks kept, the board's orphan-remap
  (`remapOrphanTasks` / `useOrphanDisplay`) would re-key those now-column-less
  tasks into the "Needs Reassignment" column, resurfacing exactly the tasks the
  user asked to hide. **Hidden steps MUST have their tasks removed before
  orphan-remap runs.**
- If only the tasks were removed but the step kept, the column would render empty,
  violating the collapse requirement.

Because a hidden step is removed from the rendered `steps` array, the two
**same-workflow** move affordances that derive from that array lose the hidden step **by
construction, with no extra filtering**: drag-and-drop (its column is not rendered, so it
is not a drop target) and the **board card's** per-card "Move to" step menu
(`task-context-step-<stepId>`, whose current-workflow list is `moveTargetSteps` = the
collapsed `steps` prop set in `swimlane-kanban-content.tsx`). The multi-select
**bulk-move** step list is the one same-workflow **board** surface that does NOT derive
from the collapsed array — it is built by `useMultiSelectDerived` / `multiSelectSteps` in
`apps/web/components/kanban-board.tsx`, which reads the **raw** `kanbanMulti.snapshots[…].steps`
of the selection's workflow, not the collapsed `steps` prop — so Build MUST filter that
list explicitly (see [Move targets](#move-targets)). None of this extends to the
cross-workflow "Send to workflow → step" submenu (sources *other* workflows' steps
directly from the store) or to the sidebar / mobile task-switcher "Move to" step menu (a
non-board surface that also reads raw snapshots and shares the `task-context-step-<stepId>`
testid); both are out of scope (see [Move targets](#move-targets)).

Note on the render path: the live board renders through `SwimlaneContainer` for
**both** the single-workflow case (`workflowFilter` set) and the multi-workflow
case. `filteredTasks` returned by
`apps/web/hooks/domains/kanban/use-kanban-data.ts` is not on the column-render
path today; if that hook's task/step derivation is touched it SHALL stay
consistent with this contract, but conformance is judged on observable board
behaviour, not on which hook computes it.

Note on the pipeline (graph) view: it renders through the same `WorkflowItemContent`
`steps` prop, so removing a hidden step from that array applies identically. The
pipeline view has no "columns"; there, "collapse the column" means the hidden step's
lane/node is removed from the rendered graph, and its tasks are hidden the same way as
in the kanban view. Both views therefore satisfy the dual-filter contract from the one
seam.

## Move targets
A hidden step SHALL NOT be offered as a manual move destination **within its own
workflow** while it is hidden, on all three of these same-workflow **board** surfaces —
the surfaces reachable from the kanban board itself. Two hold by construction; one
requires an explicit filter:

- drag-and-drop — its column is not rendered, so it is not a drop target. **By
  construction** (derives from the collapsed `steps` array).
- the board card's per-card "Move to" step menu for the task's own workflow
  (`task-context-step-<stepId>`, rendered by `useKanbanCardMoveTargets` /
  `kanban-card.tsx`, whose current-workflow list is overridden to `moveTargetSteps` = the
  collapsed `steps` prop — `kanban-card-menu-items.tsx` sets
  `result[currentWorkflowId] = steps`). **By construction** (same collapsed array).
- the multi-select bulk-move step list (`bulk-move-step-<stepId>`). **Requires an explicit
  filter.** This list is built by `useMultiSelectDerived` / `multiSelectSteps` in
  `apps/web/components/kanban-board.tsx` from the **raw** `kanbanMulti.snapshots[…].steps`
  of the selection's workflow, NOT from the collapsed `steps` prop, so the collapse does
  not remove hidden steps here on its own. Build SHALL filter `multiSelectSteps` (or the
  toolbar's step list it feeds) against the hidden set of the selection's own workflow, so
  that no `bulk-move-step-<hiddenStepId>` entry is rendered for a step hidden in that
  workflow. This filter uses the SAME per-workflow hidden set as the task predicate; it
  introduces no new state.

To move a task into a hidden step in its own workflow, the user re-ticks the step first.
This keeps "hidden" meaning "absent from this board" rather than "invisible but still a
target".

**Explicitly out of scope — the cross-workflow "Send to workflow → step" submenu.**
The per-card menu can also reassign a task into a *different* workflow's step
(`useKanbanCardMoveTargets` in `apps/web/components/kanban-card-menu-items.tsx` builds
those other-workflow step lists straight from `kanbanMulti.snapshots[…].steps`, not
from any collapsed array). Those destination lists SHALL continue to show the target
workflow's full step set regardless of that workflow's hidden set. Rationale: the
hidden set is a per-board *display* filter for the board you are viewing; suppressing a
destination in another workflow because of a display preference on your current board
would be surprising, and cross-workflow reassignment is not "this board". A task's own
current step is never a member of another workflow, so this cannot resurface a
same-workflow hidden step.

**Explicitly out of scope — the sidebar and mobile task-switcher "Move to" step menus.**
A *fourth* same-workflow "Move to step" surface exists that is NOT part of the kanban
board: the sidebar task switcher (`apps/web/components/task/task-session-sidebar.tsx` →
`task-switcher-row.tsx` → `TaskItemWithContextMenu`) and its mobile counterpart
(`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`). Both render
`TaskMoveContextMenuItems` (`apps/web/components/task/task-move-context-menu.tsx`), whose
current-workflow "Move to step" submenu sources its steps from the **raw**
`kanbanMulti.snapshots[…].steps` (via `useWorkspaceSidebarTasks` → `aggregateSidebarTasks`
in `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`), NOT from the board's
collapsed `steps` prop, and emits the **same** `task-context-step-<stepId>` testid as the
board card menu. This surface SHALL continue to show the workflow's **full** step set
regardless of the hidden set — it is out of scope for this feature, for the same reason as
the cross-workflow submenu above: the hidden set is a *display* filter for the kanban
board you are viewing, and the sidebar/mobile task switcher is a navigation surface, not
"this board". Build SHALL NOT filter it, and MUST NOT rely on the collapse to remove
hidden steps there (it does not derive from the collapsed array). Consequently, any
conformance/E2E assertion on the absence of a `task-context-step-<hiddenStepId>` entry is
scoped to the **board card menu** (and the bulk-move toolbar for `bulk-move-step-*`), NOT
to the sidebar/mobile task-switcher menu (see [Scenarios](#scenarios-acceptance-criteria)).

## Ordering & determinism
- **Steps section (new UI):** within a workflow group, steps are ordered by ascending
  `position`, ties broken by ascending step `id` (lexicographic), so the checkbox list
  is fully deterministic when two steps share a position.
- **The board is left exactly as today.** The board continues to order columns by
  ascending `position` via the existing stable sort in `WorkflowItemContent`
  (`swimlane-container.tsx`); this feature adds NO id tiebreak there and does not
  otherwise reorder columns. This is deliberate: adding a board tiebreak would reorder
  tied-position columns even when nothing is hidden, breaking the "identical to the
  pre-feature board" guarantee (see [Scenarios](#scenarios-acceptance-criteria)). On the
  rare tied-position workflow, the board's left-to-right column order and the Steps
  section's top-to-bottom order MAY therefore differ; that is accepted and the board side
  is unchanged from today.
- **Workflow-group order in the Steps section** follows the board's workflow order
  (the order of `selectWorkflowSwimlanes`, which derives from `workflows.items`).
  Moving that function to `lib/kanban/` (R2) does not change it.
- The predicate is a pure set-membership test; it introduces no ordering of its
  own and does not reorder surviving columns or tasks.
- **(R2) Step order is identical on both Display surfaces** — the shared component
  applies the same `position`-then-`id` rule on the dropdown and in the phone drawer.
- **(R2) Disclosure state** is a pure function of `(hiddenWorkflowStepIds, overrides)`;
  it is independent of render order and of the order in which snapshots arrive.
- **(R2) The mobile board navigator's step order is untouched.** `column-tab-*` indices
  are positional over the already-collapsed `steps` array, so hiding a step renumbers the
  remaining tabs. That is pre-existing behaviour, not something this feature introduces.

## Idempotency & concurrency
- Toggling a step is idempotent per `(workflowId, stepId)`: unticking adds the id
  to the workflow's hidden set (no-op if already present); re-ticking removes it
  (no-op if already absent).
- Persisting is idempotent: if the normalised `hiddenWorkflowStepIds` equals the
  current value (order-insensitive per workflow), `isSettingsUnchanged` short-
  circuits and no write is sent (matching the existing filters).
- Two concurrent writers (e.g. two tabs) resolve **last-write-wins on the
  `kanban_hidden_step_ids` field**. The backend update request uses per-field
  pointers, so a write that omits this field does not clobber it; a write that
  includes it replaces it wholesale. This matches how the Repository and Workflow
  filters already behave. No merge of individual step ids across concurrent
  writers is attempted.
- **(R2) Two Display surfaces cannot race**: at most one of `KanbanDisplayDropdown` /
  `MobileMenuSheet` renders the Steps section at any breakpoint, by construction.
- **(R2) Toggle during a viewport change:** IF the viewport crosses the 768px boundary
  while a toggle's persist request is in flight, THEN the request completes normally — it
  is owned by `useUserDisplaySettings`, not by either surface — and the newly mounted
  surface renders from the store. No debounce, no cancellation, no new state.
- **(R2) Disclosure overrides** are local state scoped to one visit to one Display surface
  (reset when that surface closes, and again when a 768px crossing hands the section to the
  other surface — see
  [Steps-section disclosure](#steps-section-disclosure--density)) and cannot race anything.
  This is the disclosure half of the viewport-change bullet above, and the two resolve
  oppositely on purpose: an **in-flight persist** survives the crossing because it is owned
  by `useUserDisplaySettings`, while **disclosure overrides** do not, because they are owned
  by the surface that is being replaced. Nothing is cancelled and nothing is migrated.

## Nil / empty / error / defaults / boundary
- **Default (fresh user, no map):** `hiddenWorkflowStepIds` is `{}`; the board is
  identical to today (every column and task shown).
- **Empty array for a workflow:** treated as "nothing hidden" and normalised away
  (key removed).
- **All steps of a workflow hidden** (assuming that workflow has **no orphan
  tasks** — for the orphan interaction see the next bullet):
  - In "All Workflows" view **while at least one other workflow still has visible
    tasks**, that workflow has zero visible tasks and is dropped from the swimlane
    list by the existing `getVisibleWorkflows` logic — the whole workflow
    disappears from the board (consistent with "a workflow with no visible tasks is
    not shown").
  - In single-workflow view, the swimlane renders with zero columns (an empty
    board region), not an error and not "No tasks yet".
- **Every eligible workflow filters to zero visible tasks in "All Workflows" view**
  (e.g. every step of every workflow hidden, or the last workflow with visible
  tasks is fully hidden): this is the existing empty-board fallback, unchanged by
  this feature. On desktop (`getVisibleWorkflows` sees `showEmptyBoard=false`) the
  board shows the existing **"No tasks yet"** empty-state message — the same result
  the Repository filter or search query already produce when they filter everything
  out; the hidden tasks are not gone, they are re-shown by re-ticking a step. On
  mobile kanban (`showEmptyBoard=true`) the board instead renders all workflows with
  zero columns. Neither is an error. This is the reason the "dropped by
  `getVisibleWorkflows`" claim above is scoped to the ≥1-visible-workflow case.
- **Orphan tasks interacting with the hidden set:** a task whose `workflowStepId`
  matches no step in its workflow's current snapshot is an *orphan* and is remapped
  by `remapOrphanTasks` into the synthetic "Needs Reassignment" column. Because task
  hiding uses `H ∩ liveStepIds` (see [dual-filter](#the-dual-filter-contract)), a
  hidden id that no longer exists does NOT hide such a task; the task orphan-remaps
  exactly as it would with an empty hidden set. Consequently, **if a workflow has
  orphan tasks, hiding all of its real steps does NOT yield zero columns — the
  "Needs Reassignment" column remains** (holding those orphans), and in "All
  Workflows" view the workflow is therefore still rendered (it has visible tasks).
  The "zero columns" and "identical to empty hidden set" statements elsewhere assume
  no orphan tasks are present.
- **Unknown / stale step id in the hidden set** (step deleted, or belongs to a
  workflow with no current snapshot): it matches no rendered step (column collapse
  is a no-op) and, per `H ∩ liveStepIds`, hides no task — so it is inert and never
  changes what renders, even for a task still pointing at the deleted id (that task
  orphan-remaps as it would with an empty set). Pruning stale ids is **not required**.
  See [Out of scope](#out-of-scope).
- **Newly added step** (created after the user last set the filter): absent from
  the hidden set, therefore shown by default.
- **Renamed step:** id is unchanged, so its hidden/shown state is preserved; the
  new title appears in the Steps section and (if shown) as the column header.
- **(R2) No eligible workflows:** the section renders nothing at all — no label, no
  separator, no header — on **both** surfaces.
- **(R2) Workflow whose snapshot has zero steps:** its group still renders (so the
  workflow is not silently absent from the filter surface) with no step rows. **Where a
  header is rendered — i.e. more than one eligible workflow — its shown-count summary
  reads "0 of 0 shown".** Where it is the *only* eligible workflow there is no header and
  therefore no summary; the section shows its label and description with no rows, per the
  single-workflow-wins rule in
  [Steps-section disclosure](#steps-section-disclosure--density).
- **(R2) `currentPage === "tasks"`:** no Steps section on either surface.
- **(R2) Default disclosure with an empty hidden set:** absent any explicit override, with
  more than one eligible workflow every group is collapsed; with exactly one, the section
  renders as R1 does.
- **(R2) Eligible-workflow count changing mid-visit** (a second snapshot arrives, or the
  user changes the Workflow filter — which lives on the same surface **on the dropdown
  surfaces only**: `MobileDisplayOptions` passes `showWorkflow: !isMobile || currentPage
  !== "kanban"`, so the Workflow select is **not rendered** in the phone drawer on the
  kanban page, which is exactly where the phone's Steps section lives. On a phone the
  count therefore only changes when a snapshot arrives. Build SHALL NOT add a Workflow
  select to the phone drawer to make this parenthetical true — that control's absence
  there is pre-existing, deliberate, and out of scope): the section
  re-renders under whichever branch now applies. Going 1 → many, groups gain headers and
  resolve through `overrides[id] ?? defaultExpanded(id)` like any other group — a workflow
  that was rendered inline has no override, so it takes the default. Going many → 1, the
  single group renders inline and fully expanded regardless of any override recorded for it;
  the override is not cleared, and if the count returns to many within the same visit that
  override applies again. The override map is keyed by workflow id, so it survives these
  transitions without being consulted while the single-workflow branch is active.
- **(R2) Tablet (768–1024px, coarse pointer):** already served by `KanbanDisplayDropdown`
  in `TabletHeader`. R2 changes nothing there beyond the shared rendering.

## Failure modes
- **Persist request fails** (WS and REST fallback both error): the UI keeps the
  user's in-memory selection for the session (the board reflects it immediately);
  the failure is swallowed/logged, exactly as the existing display-settings
  writes do. The selection may not survive the next cold load if every write
  failed — this is the same durability contract as the Workflow/Repository
  filters and is acceptable.
- **Snapshot for a workflow not yet loaded:** the workflow is **omitted** from the
  Steps section until its snapshot arrives (`selectWorkflowSwimlanes` returns only
  workflows with a loaded snapshot, so there is no group to show). No crash. Once the
  snapshot loads, the group appears and any persisted hidden ids for that workflow take
  effect. (This is the single chosen behaviour — not "show an empty group".)
- **Corrupt/missing persisted value:** hydration falls back to `{}` (all shown).

## Scenarios (acceptance criteria)

- **GIVEN** two workflows `A` and `B`, each with a step titled `Done` (different
  ids) that each holds a task, **WHEN** the user unticks `Done` in workflow `A`'s
  group, **THEN** workflow `A`'s `Done` column disappears and its task is hidden,
  **AND** workflow `B`'s `Done` column and task remain visible unchanged.

- **GIVEN** a workflow with a `Done` step that holds a task, **WHEN** the user
  unticks `Done`, **THEN** the `Done` column is removed from the board (not shown
  empty) **AND** the task does not reappear in a "Needs Reassignment" column.

- **GIVEN** a step has been unticked, **WHEN** the user re-ticks it, **THEN** the set
  of rendered column step-ids for that workflow, their left-to-right order, and the set
  of visible task-ids in each column all return to exactly what they were before the
  step was unticked (no column added or removed beyond the re-shown one, no task moved).

- **GIVEN** a step is unticked, **WHEN** the user reloads the page, **THEN** the
  step's column (`kanban-column-<stepId>`) and its task cards are still absent and the
  Steps-section checkbox for that step (`steps-filter-step-<stepId>`, see
  [Steps-section selectors](#steps-section-selectors)) is still unticked.

- **GIVEN** a step is unticked, **WHEN** the user opens that task's **board-card**
  "Move to" step menu for the task's own workflow, or the multi-select **bulk-move
  toolbar**, **THEN** no `task-context-step-<hiddenStepId>` / `bulk-move-step-<hiddenStepId>`
  entry is present. **AND** the cross-workflow "Send to workflow → step" submenu for a
  *different* workflow still lists that other workflow's steps unaffected by this
  workflow's hidden set. **AND** the **sidebar / mobile task-switcher** "Move to" step
  menu (a non-board surface) is out of scope: it MAY still list the hidden step and this
  AC does NOT assert its absence there (per [Move targets](#move-targets)). This
  absence assertion is therefore scoped strictly to the board card menu and the bulk-move
  toolbar, the two board surfaces whose `task-context-step-*` / `bulk-move-step-*` lists
  this feature controls.

- **GIVEN** no steps are hidden (`hiddenWorkflowStepIds` is `{}`), **WHEN** the board
  renders, **THEN** the set and order of rendered column step-ids and the set of visible
  task-ids per column are identical to a build without this feature, and every
  Steps-section checkbox is ticked. **(R2) How the checkbox clause is observed:** the board
  clause is observed with no interaction, but with more than one eligible workflow every
  group starts collapsed, so its checkboxes are not in the DOM (see
  [Steps-section disclosure](#steps-section-disclosure--density)). The checkbox clause is
  therefore observed **after expanding each group**, and it is NOT satisfied vacuously by a
  locator that matches zero elements — a conforming test SHALL assert the expected checkbox
  count as well as their checked state. With exactly one eligible workflow the rows are
  inline and no expansion is needed.

- **GIVEN** a Repository filter is also active, **WHEN** a step is hidden, **THEN**
  the visible tasks are those that pass **both** filters (AND semantics), and
  hiding/showing a step leaves `repositoryIds` unchanged.

- **GIVEN** a workflow whose every step is unticked in "All Workflows" view (with at
  least one other workflow having visible tasks), **WHEN** the board renders, **THEN**
  that workflow's swimlane is not present in the DOM, while every other workflow renders
  the same columns and tasks it did before.

- **GIVEN** a single workflow (with no orphan tasks) is selected in the Workflow filter
  and every one of its steps is unticked, **WHEN** the board renders, **THEN** the
  swimlane renders zero columns (no `kanban-column-*` for that workflow) and shows neither
  an error nor the "No tasks yet" empty-state message, **AND** the Steps section still
  lists all of that workflow's steps unticked so they can be re-ticked.

- **GIVEN** "All Workflows" view and every eligible workflow filtered to zero visible
  tasks (all their steps hidden, and no orphan tasks), **WHEN** the board renders, **THEN**
  the board shows the existing "No tasks yet" empty-state on desktop (and renders all
  workflows with zero columns on mobile kanban) — an empty board, not an error — and every
  step re-appears once its Steps-section checkbox is re-ticked.

- **GIVEN** the hidden set for a workflow contains a step id that no longer exists in
  that workflow's snapshot, **WHEN** the board renders, **THEN** the set of rendered
  column step-ids and visible task-ids for that workflow is identical to rendering with
  that workflow's hidden set empty — including a task still pointing at the stale id, which
  orphan-remaps into "Needs Reassignment" exactly as it would with an empty hidden set (the
  stale id hides nothing because task hiding uses `H ∩ liveStepIds`).

### R2 scenarios

- **GIVEN** a phone viewport and a workflow with two steps each holding a task, **WHEN**
  the user opens the topbar menu, finds the Steps section in Display options and unticks
  one step, **THEN** that step's column is absent from the phone board
  (`kanban-column-<stepId>` count 0), its task is absent, the mobile board navigator no
  longer lists that step by title, the other step and task remain, and no
  `kanban-column-__kandev_orphan__` appears. **AND** re-ticking restores the column and
  its task.

- **GIVEN** a phone viewport, **WHEN** the user unticks a step through the drawer and
  reloads the page, **THEN** the step is still hidden and its checkbox still reads
  unticked.

- **GIVEN** `hiddenWorkflowStepIds` already contains a step of the board's workflow
  (persisted from any surface), **WHEN** a phone user opens the menu, **THEN** that step's
  row is present and unticked, and re-ticking it restores its column and tasks on the
  phone board. (This is the trapdoor R2 exists to close.)

- **GIVEN** a phone viewport with the Steps section rendered and **more than one eligible
  workflow** (so both kinds of interactive row exist), **WHEN** the drawer is measured,
  **THEN** every `steps-filter-group-toggle-<workflowId>` in the DOM is ≥ 44 CSS px tall,
  **AND** after expanding a group every `steps-filter-step-row-<stepId>` it reveals is
  ≥ 44 CSS px tall, **AND** `document.documentElement.scrollWidth` does not exceed its
  `clientWidth`. The assertion is made against those two ids and not against
  `steps-filter-step-<stepId>` (see
  [Steps-section selectors](#steps-section-selectors)), and each measurement is paired with
  an expected-count assertion so an empty locator cannot pass it.

- **(R2) Overrides do not survive a breakpoint crossing — the surface stays open.**
  **GIVEN** a phone viewport (< 768px), more than one eligible workflow, an empty hidden set
  (so every group's `defaultExpanded` is `false`), and the mobile menu drawer **open** on
  the kanban page with the user having explicitly toggled workflow `A`'s
  `steps-filter-group-toggle-<A>` to expanded (`aria-expanded` `true`, `A`'s
  `steps-filter-step-<stepId>` checkboxes in the DOM, `overrides["A"] === true` against a
  collapsed default), **WHEN** the viewport widens into the **tablet** range and then
  narrows back below 768px **with the drawer never closing**, **THEN** on return `A`
  resolves from `defaultExpanded`: `steps-filter-group-toggle-<A>` reads `aria-expanded`
  `false` and `A`'s checkboxes are absent from the DOM, while
  `steps-filter-group-<A>` and its shown-count summary are present; **AND**
  `hiddenWorkflowStepIds` is unchanged and no settings write is issued.
  **(R2) This is the ONLY criterion that distinguishes reset trigger 2 from trigger 1, and
  three details are load-bearing:**
  1. **The drawer must never close.** If it closes at any point, trigger 1 fires and a build
     that never implemented trigger 2 passes anyway. Any variant that closes and re-opens the
     surface does NOT observe trigger 2 and SHALL NOT be substituted for this criterion.
  2. **The widened viewport must be `isTablet`, not `compactDesktop`.** Only the tablet
     branch of `kanban-header.tsx` renders `TabletHeader` **and** `MobileMenuSheet` together,
     which is what keeps the drawer mounted and open across the crossing. `compactDesktop`
     (768–1024px with a **fine** pointer) renders `DesktopHeader`, which renders no
     `MobileMenuSheet` at all — the drawer would unmount, closing it and collapsing this
     criterion back into trigger 1.
  3. **Where it is observed: Vitest, not Playwright** — for the same reason as the
     tablet-exclusivity criterion below. No configured Playwright project can produce
     `isTablet`. It SHALL be observed by a component test that drives a mocked
     `useResponsiveBreakpoint` through `mobile → tablet → mobile` while holding the drawer's
     `open` prop `true` throughout. The `aria-expanded` assertion SHALL be paired with an
     expected-count assertion on `steps-filter-group-toggle-<workflowId>` so an empty locator
     cannot pass it.

- **(R2) A crossing migrates no override in the other direction either — but this does NOT
  distinguish the triggers.** **GIVEN** more than one eligible workflow, an empty hidden set,
  and a tablet viewport where the user has opened the Display **dropdown** and explicitly
  expanded workflow `A`'s group, **WHEN** the viewport crosses below 768px, **THEN** the
  crossing itself opens nothing — the phone drawer is **closed**, exactly as it was, because
  a crossing changes which surface owns the section and never opens or closes a surface (see
  [Steps-section disclosure → Lifetime](#steps-section-disclosure--density)) — **AND** when
  the user then opens the drawer, `A` resolves from `defaultExpanded` with `aria-expanded`
  `false`, and `hiddenWorkflowStepIds` is unchanged with no settings write issued. **AND**
  crossing back does not restore the expansion. **This criterion is stated for completeness
  and is explicitly NOT sufficient coverage of trigger 2:** the user's `open` of the drawer
  is itself a fresh visit that clears overrides under trigger 1, so a build implementing only
  trigger 1 satisfies it. It SHALL NOT be written in place of the held-open criterion above.

- **(R2) Sole eligible workflow with zero steps** — **GIVEN** exactly one eligible workflow
  whose snapshot has zero steps, **WHEN** the Steps section renders, **THEN** the section's
  label and description are present, **AND** there is no
  `steps-filter-group-toggle-<workflowId>`, no shown-count summary, and zero
  `steps-filter-step-<stepId>` rows — the single-workflow inline rule wins over the
  "0 of 0 shown" reading, which applies only where a header exists.

- **(R2) Placement of the section within the phone drawer** — **GIVEN** a phone viewport
  with `currentPage === "kanban"` and the mobile menu drawer open, **WHEN** the Display
  options block renders, **THEN** the Steps section appears **after** the Repository field
  and **before** the Preview-panel field in document order within that block. Observed by
  comparing DOM positions (e.g. `compareDocumentPosition`, or index within the block's
  children), not by a screenshot. Without this criterion the placement SHALL requirement in
  [Display surfaces](#display-surfaces-desktop-tablet-mobile) has no observer at all and
  Build could render the section last with every other test still green.

- **(R2) The section is absent on the tasks page, on both surfaces** — **GIVEN**
  `currentPage === "tasks"`, **WHEN** the mobile menu drawer is opened on a phone viewport,
  **THEN** its subtree contains **zero** elements whose testid begins `steps-filter-`;
  **AND** on a desktop viewport with the same `currentPage`, opening `display-button`
  yields zero such elements too. Asserted on each surface's own subtree, not on the
  document, for the same reason the tablet-exclusivity criterion is.

- **(R2) Stale hidden ids are excluded from the shown-count summary** — **GIVEN** more than
  one eligible workflow, and workflow `A` whose snapshot has `N` live steps and whose hidden
  set contains **one live step id plus one stale id** that matches no step in `A`'s current
  snapshot, **WHEN** the Steps section renders, **THEN** `A`'s shown-count summary reads
  "`N-1` of `N` shown": the stale id is excluded from both the hidden count and the total,
  consistent with `H ∩ liveStepIds`. **AND** `A`'s group is expanded by default, because the
  *live* hidden id still satisfies the default-disclosure rule. A summary that counted raw
  `H` would read "`N-2` of `N`" and is non-conforming. This is the one derivation rule in
  the summary that a naive implementation gets wrong silently, which is why it has its own
  criterion rather than only a unit-test line.

- **GIVEN** a tablet viewport (768–1024px, coarse pointer) where `kanban-header.tsx`
  renders `TabletHeader` **and** `MobileMenuSheet` together, **WHEN** the mobile menu
  drawer is opened, **THEN** its subtree contains **zero** elements whose testid begins
  `steps-filter-` — no group container, no toggle, no step row, no step control. **AND**
  separately, **WHEN** the Display dropdown (`display-button`) is opened and, if more than
  one workflow is eligible, the group under test is expanded, **THEN** exactly one
  `steps-filter-step-<stepId>` element exists for each step of that group.
  **(R2) Both preconditions are load-bearing and neither clause may be observed without
  them.** The first clause is the real exclusivity claim and is asserted on the drawer
  subtree, not on the document, so it cannot be satisfied by the dropdown merely being
  closed. The second clause is a duplication check, and a bare document-wide "exactly one
  per step" would pass vacuously in three different ways — dropdown closed, group collapsed
  by the default rule (see
  [Steps-section disclosure](#steps-section-disclosure--density)), or both — so it is
  stated with the open-and-expanded precondition and SHALL be paired with an expected-count
  assertion, exactly as the "every checkbox is ticked" AC above requires.
  **(R2) Where this criterion is observed: Vitest, NOT Playwright.** It is written in
  browser terms, but no configured Playwright project can produce `isTablet`, which requires
  a 768–1024px viewport **and** a coarse pointer. `apps/web/e2e/playwright.config.ts`
  defines exactly five projects — `routing`, `auth`, `chromium` (Desktop Chrome: a **fine**
  pointer, so resizing it to 800px yields `compactDesktop`, which renders `DesktopHeader`
  and no `MobileMenuSheet`), `mobile-chrome` (Pixel 5 → `mobile`), and `containers` — and
  none of them lands on the tablet branch. This criterion SHALL therefore be observed by a
  component test that mocks `useResponsiveBreakpoint` to return `isTablet: true` and asserts
  both clauses against the rendered tree. **Build SHALL NOT add a tablet Playwright project,
  and SHALL NOT reach for a per-test `test.use({ hasTouch })` override, to make this a
  browser test** — the gate this criterion protects is a render-time branch, and a component
  test observes it exactly.

- **GIVEN** more than one eligible workflow and an empty hidden set, **WHEN** the Steps
  section renders, **THEN** each group shows its disclosure header with a shown-count
  summary and no `steps-filter-step-<stepId>` checkbox is in the DOM, **AND** expanding a
  group reveals exactly that workflow's steps in `position` order.

- **GIVEN** more than one eligible workflow and a hidden step in workflow `A`, **WHEN** the
  Steps section renders, **THEN** `A`'s group is expanded by default (so the hidden step is
  discoverable) while a workflow with nothing hidden is collapsed.

- **GIVEN** exactly one eligible workflow, **WHEN** the Steps section renders, **THEN**
  there is no disclosure header and every step row is inline — identical to R1.

- **GIVEN** any group, **WHEN** the user expands or collapses it, **THEN**
  `hiddenWorkflowStepIds` is unchanged, no settings write is issued, and the board renders
  identically.

- **GIVEN** more than one eligible workflow, an empty hidden set, and the user has expanded
  workflow `A`'s group (so `A` is expanded against a collapsed default), **WHEN** the user
  unticks a step inside `A`, **THEN** `A` is **still expanded** — `aria-expanded` is `true`
  and `A`'s `steps-filter-step-<stepId>` checkboxes are still in the DOM, including the one
  just unticked — and `B`'s group is unaffected. (This is the case a set-typed override
  would get wrong: the untick flips `A`'s default to expanded, and an inverting override
  would collapse the group on the very click that produced it.)

- **(R2) Re-ticking the last hidden step, NO override** — **GIVEN** more than one eligible
  workflow and exactly one hidden step in workflow `A`, and the user has **not** touched
  `A`'s disclosure header this visit (so `A` is expanded by the default rule with
  `overrides["A"]` absent), **WHEN** the user re-ticks that step, **THEN** `A` **collapses**:
  `aria-expanded` on `steps-filter-group-toggle-A` reads `false` and `A`'s
  `steps-filter-step-<stepId>` checkboxes leave the DOM, while
  `steps-filter-group-<A>` and its shown-count summary (now reading "N of N shown") remain
  present. No group other than `A` changes disclosure, and no settings write beyond the step
  toggle is issued. This is the recomputed default reasserting itself — the group closes on
  the very click that produced it — and it is required behaviour, not a defect to design
  around.

- **(R2) Re-ticking the last hidden step, WITH an override** — **GIVEN** the same starting
  state except that the user has explicitly toggled `A`'s disclosure header to expanded this
  visit (so `overrides["A"] === true`), **WHEN** the user re-ticks that step, **THEN** `A`
  **stays expanded**: `aria-expanded` reads `true` and `A`'s checkboxes are still in the
  DOM, including the one just re-ticked. No group other than `A` changes disclosure, and no
  settings write beyond the step toggle is issued. These two criteria differ only in whether
  an override exists, and that is the whole point: the override is what makes the user's
  intent survive a moving default.

- **GIVEN** more than one eligible workflow and a hidden step in workflow `A` (so `A` is
  expanded by default), **WHEN** the user explicitly collapses `A`, **THEN** the collapse is
  honoured — `aria-expanded` is `false` and `A`'s checkboxes leave the DOM — `A`'s
  `steps-filter-group-<workflowId>` container and its shown-count summary remain present and
  still report the hidden step, and nothing auto-reopens the group.

- **GIVEN** the user has explicitly collapsed workflow `A`'s hidden-bearing group, **WHEN**
  the user closes the Display surface and opens it again, **THEN** the override is gone and
  `A` is expanded again by the default rule, with no persisted value having changed.

- **GIVEN** the selector move, **WHEN** the board renders, **THEN** the set and order of
  rendered column step-ids, the visible task-ids per column, the Steps-section group order,
  and the mobile board navigator's workflow list are all identical to before the move.

## E2E requirement
This feature changes rendered UI under `apps/web/` and is **not** on the E2E
exemption allowlist, so a Playwright spec under `apps/web/e2e/` is **required**.
It SHALL assert real behaviour, seeded via the existing `apiClient` helpers
(`createWorkflow`, `listWorkflowSteps`, `createWorkflowStep`, `createTask`,
`saveUserSettings`) used by `apps/web/e2e/tests/kanban/workflow-filter.spec.ts`,
and drive the Display dropdown (`display-button`) the way that spec does. It SHALL
cover at minimum:

1. **Per-workflow isolation:** two workflows each with a same-titled step holding
   a task; unticking that step in workflow A hides A's column
   (`kanban-column-<stepId>` absent) and A's task, while B's same-titled step
   column and task remain visible. Uses `KanbanPage.columnByStepId` /
   `taskCardByTitle`.
2. **Persistence across reload:** untick a step, reload, and assert the column and
   its tasks remain hidden and the checkbox (`steps-filter-step-<stepId>`, see
   [Steps-section selectors](#steps-section-selectors)) is still unticked.

(The spec author recommends also asserting no orphan/"Needs Reassignment" column
appears for the hidden step, since that is the dual-filter trap.)

### R2 — mobile spec (required)
R2 adds a rendered phone surface, so desktop-only coverage is no longer sufficient.

- A spec SHALL be added at
  `apps/web/e2e/tests/kanban/mobile-step-visibility-filter.spec.ts`. The `mobile-*.spec.ts`
  name routes it to the `mobile-chrome` (Pixel-5) project per `apps/web/e2e/README.md`; it
  SHALL NOT set a per-test device override. Nested under `tests/kanban/`, it imports
  `../../fixtures/test-base` and `../../pages/mobile-kanban-page`.
- It SHALL seed through the same `apiClient` helpers as the desktop spec
  (`createWorkflow`, `listWorkflowSteps`, `createWorkflowStep`, `createTask`,
  `saveUserSettings`) and reset `kanban_hidden_step_ids` to `{}` in `afterEach`.
- It SHALL seed a **second** step holding a **second** task in the board's workflow, for
  the same reason the desktop spec does: otherwise hiding the only populated step zeroes
  the workflow's visible tasks and the pre-existing "drop workflows with no visible tasks"
  path masks whether per-step column collapse actually worked.
- **(R2) It SHALL seed a SECOND WORKFLOW. This is mandatory, not conditional.** With one
  workflow the section renders inline under the single-workflow rule, no
  `steps-filter-group-toggle-<workflowId>` ever enters the DOM, and the ≥ 44 px requirement
  on disclosure headers goes **entirely unmeasured** — so a fully green required suite would
  let a sub-44 px header ship on the very surface R2 exists to add. The second workflow is
  what puts the header on screen, and it also makes the collapsed-by-default path the
  spec's normal case rather than an optional extra.
- Minimum scenarios, matching the R2 scenarios above, **all six required**: (1)
  reachability, toggle, board result and restore — noting that with two workflows seeded the
  group starts collapsed, so this scenario expands it first; (2) persistence across reload
  driven through the **real UI**, not seeded via `saveUserSettings`, so the outbound write
  path is exercised; (3) touch-target geometry ≥ 44 px measured on **both**
  `steps-filter-group-toggle-<workflowId>` (collapsed state) and
  `steps-filter-step-row-<stepId>` (after expanding), each paired with an expected-count
  assertion; (4) no document-level horizontal overflow; (5) the collapsed-group path —
  checkboxes absent from the DOM, container and toggle present, expanding reveals exactly
  that workflow's steps in `position` order; **(6) the zero-column phone path** — hide
  **every** step of one seeded workflow through the drawer, then assert that workflow
  renders **zero** `kanban-column-*` on the phone board without an exception, an error
  boundary, or the "No tasks yet" empty-state, and that re-ticking a step restores its
  column. This is the mobile half of the all-hidden acceptance criterion in
  [Scenarios](#scenarios-acceptance-criteria) — "renders all workflows with zero columns on
  mobile kanban" (`showEmptyBoard=true`) — which no desktop spec can observe. It is required
  because R2 makes the phone a first-class surface and a zero-length `steps` array reaching
  `SwipeableColumns` / `mobile-column-tabs` is the one crash-class risk that only the phone
  can hit. Seed the second workflow with a task so the board is not globally empty.
- The existing desktop spec's **multi-workflow** tests seed a second workflow and therefore
  now start collapsed; they SHALL gain an expand step before clicking **or asserting on** a
  step checkbox. A missing expand step turns a checked-state assertion into a locator that
  matches nothing, which passes vacuously — so every such assertion SHALL be paired with an
  expected-count assertion. Their assertions SHALL NOT be weakened — only the interaction
  sequence changes.
- Build SHALL `rg` over `apps/web/e2e/tests/**/mobile-*.spec.ts` for controls this change
  hides or replaces and record the result. R2 only *adds* a control, so no removal sweep is
  expected, but the check is not optional.

**(R2) Unit/component coverage (Vitest) is REQUIRED, not merely expected**, for each item
below. Several acceptance criteria in [R2 scenarios](#r2-scenarios) name Vitest as their
only observation surface because no configured Playwright project can reach the state they
describe; for those, this list is the whole of their coverage and dropping an item silently
un-observes a criterion.

1. The shared Steps component's group/step derivation and ordering (`position`, tiebreak by
   ascending step `id`), asserted identically for both surfaces.
2. **The surface-exclusivity gate**, including the **tablet** case — mock
   `useResponsiveBreakpoint` to return `isTablet: true` and assert the drawer subtree holds
   zero `steps-filter-*` testids while the dropdown holds exactly one
   `steps-filter-step-<stepId>` per step of the expanded group. This is the sole observer of
   the tablet-exclusivity criterion; see the note there for why it cannot be a browser test.
3. The disclosure-default function.
4. The **override-resolution function** (`effectiveExpanded = overrides[id] ?? defaultExpanded(id)`)
   — a pure, separately testable function covering at minimum that an explicit `true`
   survives a hidden set going from empty to non-empty for that workflow, that an explicit
   `false` survives the reverse, and that an absent key falls through to the recomputed
   default.
5. **The toggle-write rule** `overrides[id] = !effectiveExpanded(id)`, covering both
   directions: two toggles on a collapsed-default group leave `false`, and two toggles on a
   **hidden-bearing (expanded-default)** group leave `true`. Asserting a fixed `false` for
   both is the specific defect this item exists to catch.
6. **The override-reset rule** — the map is empty after the surface closes, **and** after the
   section changes surface across the 768px boundary with the surface held open (drive a
   mocked breakpoint `mobile → tablet → mobile` with the drawer's `open` prop `true`
   throughout). This is the sole observer of the held-open crossing criterion.
7. **(R2) Override survival across an eligible-count change** — many → 1 → many within one
   visit: the single-workflow branch renders inline and fully expanded and does **not**
   consult or clear the override, and when the count returns to many the recorded override
   applies again. A build that clears overrides on the branch change violates
   [Nil/empty](#nil--empty--error--defaults--boundary) invisibly without this test.
8. **(R2) The shown-count derivation function** — stale hidden ids excluded per
   `H ∩ liveStepIds`, and a zero-step workflow **that renders a header** reading
   "0 of 0 shown".
9. **(R2) Long-title truncation** — a step title and a workflow name each render with the
   single-line ellipsis classes and carry the full untruncated text in the `title`
   attribute, per [Display surfaces](#display-surfaces-desktop-tablet-mobile).
10. The relocated selectors in their new `apps/web/lib/kanban/workflow-swimlanes.test.ts`
    home.

## i18n
All new copy goes through `t()` with keys in the `kanban` namespace
(`apps/web/src/locales/en/kanban.json`); `pnpm run i18n:ratchet` fails on
hardcoded strings. Recommended keys (Build may adjust the values, not the
through-`t()` requirement): `"steps": "Steps"` (section label) and a short
description string for the section. Do not call `t()` at module scope. Step
titles and workflow names are user/domain data and are rendered verbatim, never
translated.

**(R2)** The phone surface reuses `kanban:steps` and `kanban:stepsSectionDescription`
verbatim — the helper sentence is not rewritten for phones. The disclosure summary needs
one new key taking `{{shown}}` and `{{total}}` as interpolated values (e.g.
`"{{shown}} of {{total}} shown"`); it SHALL NOT be assembled by concatenation and SHALL NOT
pass an English plural ending as a value. `apps/web/components/kanban/` paths are added to
`i18nGuardFiles` in the same change if not already listed.

## Out of scope
- Any semantic/phase classification of steps (no reading or branching on
  `stage_type`).
- Any cross-workflow "status category" concept or matching steps by title.
- Any `task.state`-based hide shortcut.
- Changes to the plugin task-filter API (`registerTaskFilter`); this is the
  first-party equivalent, built with direct store access, not as a plugin.
- Pruning stale step ids from the persisted hidden set (inert; may be added later
  but is not required for correctness).
- A "hide/show all" bulk toggle for a workflow's steps (per-step toggles only in
  this iteration).
- **(R2) Relocating `useMultiSelectDerived`** out of `apps/web/components/kanban-board.tsx`.
  Raised in review; **declined**, with evidence. That file is 613 raw lines, of which 35 are
  blank and ~7 comment-only, giving ~571 lines countable under
  `apps/web/eslint.config.mjs`'s `"max-lines": ["warn", { max: 600, skipBlankLines: true,
  skipComments: true }]` — ~29 lines of headroom against a rule that is a **warning**, not
  an error. Against that, moving it would invalidate the file references in
  [The dual-filter contract](#the-dual-filter-contract) and [Move targets](#move-targets) —
  the exact passages explaining *why* the bulk-move list needs an explicit filter — and
  churn the largest file of a PR that has already survived five review rounds, for no
  correctness, lint, or test benefit. **Reopen condition:** IF `kanban-board.tsx` reaches
  600 countable lines, THEN relocating `useMultiSelectDerived` (with `SnapEntry` and its
  `kanban-board.test.ts` coverage) into `apps/web/hooks/domains/kanban/` becomes the
  preferred remedy over any other extraction, and this spec's file references are corrected
  in that same change.
- **(R2) Moving `filterTasks`** out of `swimlane-container.tsx` — it is this spec's pinned
  dual-filter seam and has no cross-layer importer.
- **(R2) Persisting disclosure state.** Group expansion is ephemeral UI state scoped to one
  visit to the Display surface.
  No DB column, no wire field, no store field, no new concurrency surface.
- **(R2) Scoping the Steps section to the mobile navigator's focused workflow.** The group
  set stays identical on mobile and desktop; a focused-only list would make an
  **all-steps-hidden** workflow unrecoverable. ("All-steps-hidden" — every step in the
  workflow's hidden set — is a different condition from `workflow.hidden`, the store flag
  that keeps system workflows off the board. The two are unrelated; see the next bullet.)
- **(R2) Restoring a hidden step that belongs to a `workflow.hidden` workflow, from
  "All Workflows" or from a phone.** Named and **accepted**, not overlooked. The path
  exists: the Display dropdown's Workflow select is built from the unfiltered workflow
  list, so a desktop or tablet user can select a hidden workflow, get its group via
  `selectWorkflowSwimlanes(<id>, …)` — whose explicit-filter branch returns it hidden or
  not — and untick one of its steps. From then on that step is re-tickable **only** by
  selecting that same hidden workflow in the Workflow filter again, which is reachable on
  desktop and tablet and **not** on a phone, because `MobileDisplayOptions` passes
  `showWorkflow: !isMobile || currentPage !== "kanban"` and so renders no Workflow select
  on the phone kanban page. Meanwhile `selectMobileNavigatorWorkflows` **does** surface a
  hidden workflow's board on the phone whenever it has visible tasks, and the hidden-set
  predicate applies to it — so the phone can display the consequence of a filter it cannot
  offer a control for. Hiding that workflow's last populated step additionally drops it
  from the navigator, because the navigator's task list is computed after the predicate.
  **Accepted for this iteration**, on these grounds: hidden workflows are system/office
  workflows deliberately kept off the board, reaching this state takes a deliberate
  desktop-side act on such a workflow, nothing is lost (the hidden set is a display filter
  and the tasks are untouched), and a full recovery path exists on desktop and tablet.
  Build SHALL NOT widen phone eligibility, add a Workflow select to the phone drawer, or
  union extra groups into the Steps section to close this. Doing so would break the frozen
  "the group set is the same on mobile as on desktop" rule and would require its own
  ordering, snapshot-availability and board-rendering rules — a second feature, not a fix.
  **Reopen condition:** IF hidden workflows ever become routinely user-facing on the phone
  — i.e. the phone gains a Workflow filter, or `selectWorkflowSwimlanes`'s "All Workflows"
  branch stops filtering on `!workflow.hidden` — THEN this exclusion is void and the Steps
  section's eligibility SHALL be revisited in the same change.
- **(R2) Changing the Display dropdown's width or max-height**, or adding a nested scroll
  container to the Steps section. The shared `DropdownMenuContent` cap and the drawer's
  existing scroll region remain the only scroll owners.
- **(R2) A tablet-specific Steps surface** — tablet already renders the dropdown.

## Pinned decisions (locked with the user — do not re-open)
1. Hiding a step also collapses its now-empty column, not just its cards.
2. The selection persists in backend user display settings, like the
   Workflow/Repository filters — not session-only.
3. The same predicate applies to both the single-workflow board and the
   multi-workflow swimlane.
4. Manual, per-workflow, keyed on `workflowStepId`. No phase grouping, no
   name-matching across workflows, no `task.state` shortcut. Store the hidden set
   (unticked steps), keyed by workflow id, tracking step id not title.
5. **(R2)** The review follow-ups are folded into **this** spec and shipped inside PR
   #2467 — one spec, one PR. No separate follow-up spec and no second PR.
6. **(R2)** The phone gets the Steps filter as a first-class surface, not a deferred
   nice-to-have: the predicate already applies to the phone board, so shipping without it
   would leave a state the phone cannot undo. **Scope of that claim:** it covers every
   workflow the Steps section is eligible to offer — i.e. non-hidden workflows with a
   loaded snapshot, which is every workflow the board renders in "All Workflows". It does
   **not** cover steps of `workflow.hidden` workflows, which remain restorable only from
   the desktop/tablet Workflow filter; that residue is named and accepted under
   [Out of scope](#out-of-scope) rather than silently claimed as closed.
