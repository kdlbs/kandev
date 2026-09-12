---
created: 2026-09-11
status: implemented
requirements:
  - REQ-UI-THREADS-DECK-004
  - REQ-UI-THREADS-DECK-005
  - REQ-UI-THREADS-SAVED-VIEWS-005
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/ui/system-design/threads-saved-views.md
legacy_specs: []
---

# Implementation Plan: Threads Layouts

## Overview

Add Columns and a two-row Grid, plus independent composer auto-hide, saved
with each Threads view. Keep the existing Open task action. Phones keep one
conversation at a time with swipe/picker navigation and the normal composer.

The user approved implementation on 2026-09-11. The initial five orders are done.
The scoped polish requested on 2026-09-12 is also complete in Task 06.
The feature is committed and open for review in PR #3626. Review remediation
retains the existing implementation boundaries and is recorded below.
Read the previews first, then the work order
for the implementation boundary being changed.

## Scope

### In scope

- Persist layout and composer preferences through existing view drafts,
  Save/Save as/Discard, synchronization, and recovery.
- Render a stable two-row desktop/tablet grid with viewport-owned live chats.
- Reveal composers by pointer dwell or keyboard, preserving drafts, reading
  position, and access to required actions. Touch keeps the normal composer.
- Add Display controls, translated descriptions, and Maximum chats wording.
- Prove the flows with focused tests and update the existing user guide.

### Out of scope

- New enlargement modes, arbitrary row counts, tile resizing, drag ordering,
  masonry, or more than one simultaneous phone transcript.
- New view tables/endpoints, browser storage for portable preferences, agent
  lifecycle changes, or changes to session-management/authorization rules.
- Automatically enabling Grid/auto-hide or changing the five-chat default.
- Native implementation delegation, publishing, commits, or runtime deployment.

## Technical approach

### Sources of truth

The UI system already owns the [deck requirements](../../specs/ui/requirements/threads-conversation-deck.md)
and [saved-view requirements](../../specs/ui/requirements/threads-saved-views.md).
Extend those pairs rather than creating another feature specification.

The existing [windowing package](../threads-multi-session-windowing/plan.md),
[saved-view package](../threads-saved-views/plan.md),
[phone package](../mobile-threads/plan.md), and
[task-action package](../threads-task-actions/plan.md) are implemented
foundations. Their recorded results apply to their original scopes, not this
package. Preserve their tests; add new cases without reopening or rewriting
historical completion evidence. No sibling branch is required.

The existing ADRs already establish
[surface-owned views](../../decisions/2026-08-31-surface-owned-saved-task-views.md),
[portable user settings](../../decisions/0041-backend-owned-portable-user-settings.md),
and [viewport activation](../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md).
This feature applies those boundaries and does not need another ADR.

### Persist presentation

Extend `ThreadView`/`ThreadViewDraft` in backend models and frontend types
with `layout` and `auto_hide_composer` (frontend `autoHideComposer`).
Defaults are Columns and false. Keep missing/unknown stored-field recovery
independent; reject unsupported new writes before saving. Extend every clone,
draft merge, and rollback snapshot. Keep `queryFingerprint` query-only.

### Compose the board

Pass `query.effectiveView` presentation to `ThreadsBoard`. Preserve direct
task children, keys, session choices, and the observer registry. Add the
`thread-layout.ts` effective-layout helper. Grid fills down each
pair, keeps the 360px minimum width, and uses two equal rows. A single task
fills the height. A grid needs two 300px tiles plus the 12px gap in the board
content box; below that, use Columns with a visible reason and no settings
write. Extend `useThreadSelectionRecovery` to anchor layout changes.

Refresh viewport geometry when effective layout changes even if task IDs do
not. Ignore stale observer callbacks. Both visible rows own live selected
sessions, while the existing one-adjacent-task preload and single-phone-detail
rules remain. Do not pin offscreen chats to retain a composer.

### Disclose the existing composer

Add an optional Threads-only context scoped to the task and selected session.
`use-composer-disclosure.ts` owns dwell/exit timers, explicit-open
state, and keep-open conditions. Reuse existing editor state through a narrow
interaction callback; audit native portaled model/mode/context/queue overlays.
Do not duplicate editor, draft, submit, cancel, or recovery ownership.

Collapsed routine content occupies no height and is inert. Only the existing
CI popover remains, when applicable; no Reply, Stop, or plugin row is added.
Required actions/recovery force the normal surface open. Keyboard focus on
the task tile reveals the editor. Manual Collapse preserves a draft and
returns focus to the tile; Enter or a new pointer entry reopens it. Phone and
coarse-pointer layouts keep the normal composer without changing preferences.
Editor and plugin instances stay mounted. Opaque plugin activity does not hold
the composer open, and collapse never cancels or revokes a plugin operation.
The native transcript scroll owner retains bottom-follow or the current
message/offset during footer resizing. A bounded expanded footer leaves an
80px transcript floor and keeps long input/action content reachable.

### Expose controls

Keep Display exclusively in the existing view editor, using its draft and
Save/Discard actions without a duplicate top-bar selector. Phone/tablet use the current
drawer. Relabel the existing limit Maximum chats, retaining `max_columns`
and five-task defaults. All copy is localized in five languages plus generated
pseudo output.

## ASCII UI preview

Spacing and labels are illustrative. The structural requirements are control
order, stable task identity, two desktop rows, independent transcript scrolling,
a CI-only collapsed footer, and one phone conversation. Use existing
primitives/tokens; do not hardcode copy or pixel-perfect drawings.

### UI-01: Columns, normal composers

Entry: Threads, existing/new view defaults. This preserves the current body.

```text
[All threads v] [View settings]  [Listing views]
+------------------+------------------+------------------+
| A        [Open]  | B        [Open]  | C        [Open]  |
| status / agent   | status / agent   | status / agent   |
|                  |                  |                  |
| transcript       | transcript       | transcript       |
| scrolls          | scrolls          | scrolls          |
|                  |                  |                  |
| status controls  | status controls  | status controls  |
| [Reply editor]   | [Reply editor]   | [Reply editor]   |
| [Composer tools] | [Composer tools] | [Composer tools] |
+------------------+------------------+------------------+
```

Topbar and tile headers are fixed. The board scrolls horizontally when needed.
Covers AC-UI-THREADS-DECK-004.1/.8 and AC-UI-THREADS-DECK-005.9.
Rendered check: existing `threads-view.spec.ts` and UI-02 comparison.

### UI-02: Grid, auto-hide enabled, no active draft

Entry: select Grid in Display and enable Auto-hide composer.

```text
[Monitoring v] [View settings]      [Listing views]
+------------------+------------------+------------------+
| A        [Open]  | C        [Open]  | E        [Open]  |
| status / agent   | status / agent   | status / agent   |
| live transcript  | live transcript  | live transcript  |
| [CI]*            | [CI]*            | [CI]*            |
+------------------+------------------+------------------+
| B        [Open]  | D        [Open]  | F        [Open]  |
| status / agent   | status / agent   | status / agent   |
| live transcript  | live transcript  | live transcript  |
| [CI]*            | [CI]*            | [CI]*            |
+------------------+------------------+------------------+
                     horizontal board scroll ->
```

`[CI]*` appears only when applicable; otherwise no footer height is reserved.
Stop is available in the revealed composer. Each transcript scrolls independently.
Task order remains A, B, C, D, E, F. Five admitted tasks leave the last lower
cell empty; one task fills the height. Both rows load visible selected chats.
An empty/initially loading deck uses its existing message and keeps the topbar.

Covers AC-UI-THREADS-DECK-004.1 through .5/.8 and
AC-UI-THREADS-SAVED-VIEWS-005.6.
Rendered check: `threads-layouts.spec.ts`, including lower-row deep links,
removal, bounds, and one versus two rows.

### UI-03: Grid tile, hovered or actively composing

Entry: hover one tile for 150ms, or focus the task tile with the keyboard.

```text
+-----------------------------------+
| B                         [Open]  |
| status / selected agent           |
| transcript (independent scroll)   |
|-----------------------------------|
| status / queue controls           |
| [Draft message                  ] |
| [model] [mode] [attach]     [Send] |
|                        [Collapse] |
+-----------------------------------+
```

Only this tile changes its internal allocation. Hover does not focus.
Its routine footer uses one interruptible 200ms reveal/160ms collapse with a
subtle fade/6px motion. CI stays outside that region. Reduced motion is instant.
A portaled picker holds the composer open. A draft stays open during automatic
hiding; explicit Collapse retains it and returns focus to the tile. A pending
question/permission/recovery also expands this region without hover.
Long footer content has bounded scrolling; required response actions stay
reachable. The sibling tile keeps identical bounds.

Covers AC-UI-THREADS-DECK-005.1 through .7/.9/.10.
Rendered check: `threads-composer-disclosure.spec.ts` and its mobile companion.

### UI-04: Display settings and recovery

Entry: View settings.

```text
+---------------------------------------------+
| View: Monitoring                            |
| Scope / filters / sort                      |
|                                             |
| Display                                     |
| Layout                  [Grid v]            |
| Two rows for monitoring more conversations. |
| Auto-hide composer                [On]      |
| Hover or focus a chat to reply.             |
| Touch screens keep the composer visible.   |
| Maximum chats               [5]             |
| Counts chats across both rows.              |
|                                             |
| [Discard] [Save as]                   [Save] |
+---------------------------------------------+
```

The existing editor owns one scroll region and Save/Discard. On short windows,
the Layout helper shows `Grid needs more height. Showing Columns.`; the
stored choice stays Grid. A failed sync uses the current error/retry surface
and rolls back all view fields together. Empty selected scope still disables
Save for its existing reason. Phone shows the same fields on the editor page
inside its one inset drawer and explains that layout applies on larger screens.

Covers AC-UI-THREADS-SAVED-VIEWS-005.1 through .7 and
AC-UI-THREADS-DECK-004.7.
Rendered check: `threads-display-settings.spec.ts` and its mobile companion.

### UI-05: Phone, visible composer and keyboard

Entry: Threads on a phone with auto-hide enabled.

```text
READING                       COMPOSING
+-------------------------+   +-------------------------+
| Threads / Monitoring 2/6|   | Threads / Monitoring 2/6|
| Task B v         [Open] |   | Task B v         [Open] |
| status        [Agent v] |   | status        [Agent v] |
|                         |   | transcript scrolls      |
| one live conversation   |   |                         |
| transcript scrolls      |   | [Draft message        ] |
| [Message editor       ] |   | [tools]          [Send] |
| [tools]          [Send] |   |                         |
+-------------------------+   +-------------------------+
                              | software keyboard       |
                              +-------------------------+
```

Swipe or the task-title picker changes the single conversation. Display stays
inside the existing view drawer. The normal composer stays visible even when
auto-hide is saved; no Reply or Collapse control is needed on touch screens.
Touch targets are at least 44px; fixed controls clear safe areas.
The available app height responds to the keyboard. Test keyboard-like height
reduction in emulation; a physical mobile-keyboard smoke check remains distinct
from emulation evidence.

Covers AC-UI-THREADS-DECK-004.6, AC-UI-THREADS-DECK-005.3/.7/.8/.10,
and AC-UI-THREADS-SAVED-VIEWS-005.5.
Rendered check: both new mobile specs plus existing swipe/picker tests.

## Tests

The table records the planned test contracts; each work order's Results
records the actual targets, coverage, and counts. Paths without a prefix
are relative to `apps/web`.

| Acceptance criteria | Test file and case |
| --- | --- |
| AC-UI-THREADS-SAVED-VIEWS-005.1/.7 | Backend `internal/user/store/thread_views_test.go`: `TestThreadPresentationStoredDefaults`; service `thread_views_test.go`: `TestThreadPresentationValidation`; DTO `thread_views_test.go`: null/omitted draft decoding. |
| AC-UI-THREADS-SAVED-VIEWS-005.1/.2/.7 | `lib/state/slices/ui/thread-view-wire.test.ts`: “normalizes presentation independently”; backend handlers/store tests: view/draft JSON round trip, restart read, existing settings payload delivery. |
| AC-UI-THREADS-SAVED-VIEWS-005.2/.3 | `lib/state/slices/ui/thread-view-actions.test.ts`: “retains presentation across every draft operation” and “rolls back rapid presentation edits”; `lib/threads/thread-view-query.test.ts`: “presentation changes preserve query fingerprint and admission”. |
| AC-UI-THREADS-SAVED-VIEWS-005.4/.5/.6 | `components/threads/threads-view-controls.test.tsx` and `threads-view-controls-recovery.test.tsx`: shared draft entry, default/limit labels, short-height and phone descriptions, recovery. |
| AC-UI-THREADS-DECK-004.1/.2/.6/.7 | New `components/threads/thread-layout.test.ts`: “derives rows for zero, one, odd and many tasks”, “preserves requested grid across phone and height fallbacks”. |
| AC-UI-THREADS-DECK-004.3/.4/.8 | `components/threads/threads-board.test.tsx`, new `use-thread-selection-recovery.test.tsx`, and `app/threads/threads-page-client.test.tsx`: “retains a lower-row reader across layout and membership changes”; preserve existing Open task/session/deep-link assertions. |
| AC-UI-THREADS-DECK-004.5/.6 | `components/threads/thread-column-activation.test.tsx`: “details both visible grid rows”, “ignores old-layout observer callbacks”, “keeps phone detail singular after a grid transition”; existing 30-shell budget. |
| AC-UI-THREADS-DECK-005.1/.2/.3/.4/.7/.10 | New `components/task/chat/use-composer-disclosure.test.ts`: dwell/exit, focus/draft/portal/host-operation holds, manual-collapse override, touch fallback, session/timer cleanup using fake timers. |
| AC-UI-THREADS-DECK-005.4/.5/.9/.10 | `components/task/chat/chat-input-container.test.tsx`, `use-chat-input-container.test.ts`, and new `composer-disclosure.test.tsx`: single mounted editor, interaction reporting, action/recovery force-open, cancellation pending, unchanged submission and session draft restoration. |
| AC-UI-THREADS-DECK-005.6/.8 | `components/task/chat/transcript-viewport-resize.test.ts`, `transcript-auto-scroll.test.ts`, and `clamped-scroll-restore.test.ts`: footer resize retains follow/reading state; browser geometry proves rendered anchoring and bounded footer reachability. |

## E2E tests

Use the existing primary-session creation pattern from `threads-view.spec.ts`
and traffic capture in `e2e/helpers/ws-traffic.ts`. Do not seed non-primary
sessions and assume they enter Threads. Factor shared setup into
`tests/task/threads-presentation-helpers.ts` only where reused.
Capture/restore user settings after each scenario, including failures.

| File / project | Flow and criteria |
| --- | --- |
| `threads-layouts.spec.ts` / chromium | Compare three columns with six visible grid tiles on a sufficiently wide/tall viewport; assert equal rows, minimum width, odd/single/empty cases, lower-row session/deep-link/removal, height fallback and restore, bounded selected subscriptions, and no reorder on a reply. AC-UI-THREADS-DECK-004.1 through .8; AC-UI-THREADS-SAVED-VIEWS-005.3/.6. Include a coarse-pointer tablet and 767/768px boundary cases. |
| `threads-composer-disclosure.spec.ts` / chromium | CI-only collapse; hover reveal/hide without focus or sends; keyboard tile reveal; nonempty/attachment drafts; portaled model/context/menu interaction; successful and failed send; required questions/permissions/recovery; revealed Stop; unchanged sibling bounds; history/bottom scroll with and without auto-follow; offscreen restore and normal task-page composer. AC-UI-THREADS-DECK-005.1 through .6/.9/.10. |
| `mobile-threads-composer-disclosure.spec.ts` / mobile-chrome | Normal composer remains visible with auto-hide saved; type/send, attachment/menu and required action; swipe/picker away and back; keep one detail stream, 44px hit targets, safe-area/short-height containment, and no document overflow. AC-UI-THREADS-DECK-004.6 and AC-UI-THREADS-DECK-005.3 through .10. |
| `threads-display-settings.spec.ts` / chromium | Change through UI, save/reload, switch views, duplicate/Save as/discard, observe a second client, reject a write/retry, keep ordering, and respect a five-chat cap in Grid. AC-UI-THREADS-SAVED-VIEWS-005.1 through .7. |
| `mobile-threads-display-settings.spec.ts` / mobile-chrome | Open the existing drawer, change/save both settings, keep one phone chat, restore Grid on desktop, contain long translated copy, and expose save recovery in the same drawer. AC-UI-THREADS-SAVED-VIEWS-005.2/.4/.5/.6. |

Use explicit causal waits and finite-animation settling; the intentional
150/300ms disclosure timing can use categorized `dwell` for negative browser
assertions and fake timers in unit tests. Seed a transcript longer than the
viewport for anchoring proof. A DOM-visible message is not evidence that its
screen position stayed fixed.

Do not add a new runtime instance, demo, or screenshots to this design turn.
Implementation captures test screenshots for comparing UI-01 through UI-05.
A physical-device keyboard check should be recorded if available; do not label
a resized emulated viewport as a hardware keyboard test.

## Work orders

- [x] [Task 01: Persist presentation preferences](task-01-persist-presentation.md)
- [x] [Task 02: Render the conversation grid](task-02-render-grid.md)
- [x] [Task 03: Disclose the existing composer](task-03-composer-disclosure.md)
- [x] [Task 04: Expose Display settings](task-04-display-settings.md)
- [x] [Task 05: Document Threads presentation](task-05-document-presentation.md)
- [x] [Task 06: Polish Threads presentation](task-06-presentation-polish.md)

Execute 01 -> 02 -> 03 -> 04 -> 05 sequentially in the primary session after
the user requests implementation. Work orders share state types, tile
composition, locales, and browser fixtures. No wave authorizes delegation.
Each work order owns RED/GREEN evidence and its exact verification commands;
new source/test paths are explicitly identified as planned.

## Verification results

Design validation (2026-09-11):

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/threads-layouts`:
  passed. The status inventory includes this manifest and all five new orders.
- Read-only package audit: five pending orders, 42 valid criterion references,
  local links/anchors, dependency/design paths, and 38 verification commands
  checked. Command targets resolve to existing files or explicitly planned
  additions; pnpm scripts and working directories exist.

The design turn changed only requirements, designs, and plans. Implementation
began after the user's later approval; current product evidence is recorded in
each work order below. No commit or publication is requested.

Implementation completed on 2026-09-11. Every work order passed its checks.
Task 01 passed user-settings Go packages, race-enabled persistence conformance,
SQL guard, 160 unique targeted frontend tests, typecheck, targeted lint, and
whitespace checks. Detailed RED/GREEN and commands are in its Results section.
Task 02 passed 62 focused unit tests, typecheck, targeted lint, localization,
and its final rebuilt browser suites: 20 desktop and 10 phone/tablet tests
with retries disabled. Its Results include the native boot correction,
responsive reader/deep-link regressions, and rendered checks.
Task 03 passed its scoped composer/scroll tests and 6 desktop/2 mobile browser
cases. Task 04 passed 102 focused unit tests, typecheck, scoped lint,
localization, and final rebuilt integration: 23 desktop and 19 mobile browser
tests, with retries disabled. These integrated suites overlap earlier cases;
their counts are not added to earlier totals. Task 05 updated the public
how-to and scoped ownership notes; public-doc validators (46 pages), 36 spec
linter tests, complete specification lint, and diff checks passed.

The paired requirements/designs are active/current and this plan is
implemented. Work-order Results hold the command and RED/GREEN details.
No physical-device keyboard verification was available. No commit, push,
publication, deployment, or delegated work occurred.

## Implementation checkpoint

The user resolved the plugin question: the whole composer hides, leaving only
the CI popover. This supersedes the earlier compact Reply/Stop proposal. Plugin
instances and their existing capability stay mounted while hidden; opaque
plugin activity alone has no keep-open guarantee. No plugin activity API or
external repository change is needed. See [Task 03](task-03-composer-disclosure.md#results).

To retain keyboard and touch access without another persistent control, the
task tile receives keyboard focus to reveal, and phone/coarse-pointer layouts
keep the normal composer. The latter is an explicitly communicated implementation
assumption, not a separate user answer. Composer disclosure passed its scoped
unit, type, lint, localization, and rebuilt desktop/phone browser checks.
Display controls passed 102 focused unit tests, typecheck, targeted lint,
localization, and the final integrated browser runs: 23 desktop and 19 mobile
tests, with retries disabled. Final public documentation and its validators
are complete. The package is ready for user review of the local changes.

## Polish verification (2026-09-12)

Task 06 passed 137 scoped unit tests, typecheck, scoped ESLint, localization,
and final rebuilt browser runs: 10 desktop and 4 mobile tests, with retries
disabled and strict WebSocket accounting. Screenshots were inspected for the
desktop grid and phone configurator; the phone remains the same single-chat
composition. Public-doc validation (46 pages), its test, 36 spec-linter tests,
complete spec lint, and whitespace checks passed. Detailed RED/GREEN evidence
and test-harness corrections are in Task 06 Results.

The existing isolated playground on :48431 now serves the same verified web
build. Only its frontend assets changed: no backend restart, reseed, view/draft
reset, or operation against :9998. Old hashed assets remain available for
already-open tabs; refresh loads the new interface. No commit or publication
was requested.

## PR review remediation (2026-09-12)

PR #3626 review fixes preserve the existing feature boundary:

- Saved-view selection cannot discard an unresolved draft. Desktop and touch
  lists explain that Save or Discard in View settings is required first.
- Replacement viewport observers measure current geometry before their first
  delivery, retaining visible chats and retiring offscreen subscriptions.
- The Grid height explanation stays in Display settings so it cannot consume
  the allocation being measured. Wheel interaction records the reader anchor.
- Deep-link scrolling honors reduced motion. Chinese hidden-chat wording and
  mobile locator/overflow assertions now describe the actual chat unit/target.
- Base-branch integration preserves native in-drawer deletion confirmation.
- Recovery skips zero-offset scroll assignments, which otherwise cancel an
  in-flight phone deep-link scroll during the first size measurement.

Regression evidence: six initial draft/observer unit failures, two board
unit failures, two touch draft browser failures, and the near-threshold Grid
browser failure reproduced before their fixes. Final focused unit coverage is
123 passing tests across these ten files:

```bash
cd apps/web
pnpm exec vitest run components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx components/threads/use-thread-selection-recovery.test.tsx components/threads/threads-view-controls.test.tsx components/threads/threads-view-controls-recovery.test.tsx components/threads/threads-view-editor-actions.test.tsx components/threads/threads-view-editor-utils.test.ts lib/state/slices/ui/thread-view-actions.test.ts lib/threads/thread-view-query.test.ts app/threads/threads-page-client.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

Typecheck, localization, and scoped ESLint passed. Browser verification uses
one worker, strict WebSocket accounting, and retries disabled:

```bash
cd apps/web
pnpm e2e:run --project chromium tests/task/threads-display-settings.spec.ts tests/task/threads-layouts.spec.ts tests/task/threads-composer-disclosure.spec.ts tests/task/threads-view.spec.ts -- --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-threads-display-settings.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-threads-task-actions.spec.ts -- --retries=0
```

The original mobile sequence reproduced an initial deep-link failure twice.
Three instrumented reproductions confirmed a zero-offset recovery assignment
interrupting the browser scroll. A focused regression test failed before the
guard and passed afterward. All three rebuilt browser reproductions passed
(56.3 seconds), then the original 21-test mobile sequence passed in 3.8 minutes
with diagnostics removed. The desktop command passed 24 tests before that final
no-op-scroll correction; the focused recovery tests and complete mobile rerun
cover the correction. Fresh post-commit desktop/capture and remote CI evidence
will be recorded in the external task plan, without another code change.

Documentation validation from the repository root passed:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
```

This validated 46 public pages, 36 specification-linter tests, and 19 harness
linter tests. Public guidance and the saved-view/deck contracts reflect these
fixes. Review/CI completion is tracked externally for the pushed head, not
inferred from local results. Physical-device checks remain unavailable.

## Risks

- A full composer can dominate a half-height tile. The grid-height fallback,
  bounded footer, scroll anchoring, and existing Open task action address this.
- Portaled controls, pending actions, uploads, and delayed sends need explicit
  interaction ownership; hover CSS alone is insufficient.
- Extra visible chats increase legitimate live rendering. Offscreen cleanup
  and both-row subscription assertions must remain intact.
- Old view/draft constructors can silently drop new fields. Test every clone,
  update, recovery path, and query fingerprint.
- Phone keyboard behavior varies across browsers. Emulation proves geometry
  and touch paths; record any physical-device coverage separately.
