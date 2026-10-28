---
created: 2026-10-27
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Executor profile card keyboard navigation

## Outcome and evidence

An existing plugin executor profile can be opened from the hub with a keyboard,
just as with a pointer or phone touch. The shared card also serves the legacy
executor detail page. ROOT qualified current main `46eed0e90c72acbd107a7c3a9a673eb3ec9e2638`
with a real managed Chromium, packaged disposable fixture plugin, actual hub,
router and controls: after creating a profile and returning to the hub, Tab
from provider Add reached Delete Profile and left the card without an edit
stop; the URL stayed at the hub. A row click opened the matching editor.
Run 19861 actually joined: two collected, one causal behavioral failure and
one pointer pass. The earlier component probe 41125 collected zero due to a
missing dependency and is not RED. The protected qualification source is not
an implementation or test input.

The row is a clickable `div`, with only its delete button focusable. Change its
primary area to a native button and keep delete as a sibling. The existing
`executorProfileSettingsPath(profile.id)` and `onOpen` behavior remain the
source of routing. Do not change provider policy, state, refresh, APIs, save
behavior, other executor cards, dependencies, or shared styling. Existing name
and badge supply the accessible name; no new copy is expected. Any unavoidable
copy requires the full seven-locale contract.

## Work order and dependency

One sequential [work order](task-01-profile-card-navigation.md) implements the
shared card and its desktop and phone regressions. ROOT reviewed the four-artifact
package and released implementation under its exclusive local heavy lease.

## ASCII UI previews

UI-01, executor profile card, existing profile (AC .28). Before: the row is
pointer-clickable but keyboard Tab skips its edit action. After: the primary
name/badge area is one native button, delete is its sibling, and the decorative
chevron stays on the right in the open row target. Spacing is illustrative.

```text
Desktop, before                         Desktop, after
Provider                         [Add] Provider                         [Add]
| Profile name     [Delete]  > |       | [Profile name       ] [Delete]  > |
  Tab: Add -> Delete -> outside          Tab: Add -> profile -> Delete
                                         Enter / Space -> profile editor

Phone, after (direct navigation)
Provider                        [Add]
| [Profile name              ] [Delete]  > |
      tap -> profile editor
```

The row remains in the existing vertically scrolling settings content, with no
new overlay or scroll owner. On phone, primary and delete are separate 44px
minimum touch targets without overlap, as is the row-owned chevron target;
preserve safe-area behavior and avoid
horizontal overflow. Desktop ordinary delete remains 28px; the primary row is
content-sized. Use shared state and routing on both viewports, following the
existing hub and `components/kanban-with-preview.tsx` direct-navigation
precedent. Long names truncate. Compare the rendered phone view to this
structure; dimensions are not a pixel layout specification.

## Verification and delivery

The work order gives exact commands and independent browser oracles. A nonzero
Playwright discovery and a causal own-test failure on the unmodified card are
required before production changes; setup errors or zero tests do not count.
An assertion timeout on the blocked focus or URL after loaded controls and a
native Tab trace is behavioral evidence when the pointer control passes; a
fixture, selector, discovery or infrastructure timeout is not. Never require a
future button selector before observing the current focus chain.
After correction run the bounded desktop and mobile scenarios, existing card
navigation tests in their `browser` Vitest project and scoped static checks.
The order pins Node 24.21.0 and pnpm 9.15.9 including hooks, stages actual owned
inputs before i18n and E2E sleep ratchets, and requires nonzero collection.
Each heavy command requires ROOT's
serialized lease; record actual handle, join, exit and outcomes. Determine a
resource cap for canonical typecheck from current available memory and prior
4 GiB OOM evidence; do not run a blind default or repeat an unchanged OOM.
Phone GREEN can reuse only freshly verified desktop GREEN assets with unchanged
source and artifact fingerprints; otherwise build fresh. RED assets are ineligible.
Run normal staged ratchets, active hooks and delivery skills only on later
release and under the appropriate lease. Public executor documentation describes
unchanged user steps, so no public edit is needed. No new architecture decision
or external plugin implementation is in scope.

## Risks

Nested interactive elements, a focusable container without native activation,
or a delete event bubbling into navigation would recreate accessibility or
mutation defects. A test must drive real native Tab and activation, identify the
actual profile route and check both navigation and delete isolation, rather than
only inspecting markup. Confirm the phone control sizes on the actual rendered
surface. Maintain the existing unrelated catalogue-refresh behavior.

## Review correction: preserve the original row targets

CodeRabbit's [finding](https://github.com/kdlbs/kandev/pull/4868#discussion_r4394365465)
is valid: base `46eed0e90c72acbd107a7c3a9a673eb3ec9e2638` placed delete
before the far-right chevron; published `7b9cfd30a50f501f4d46160e8341c023d3d06b3e`
puts delete at that former open target. A familiar pointer or touch there can
now delete. Existing AC .28 already requires the pointer destination and
independent delete; no new product requirement is needed. ROOT reviewed the
bounded refinement and separately released implementation and a heavy lease.

Restore the old visual order and actionable row padding without nesting buttons:
profile primary button, sibling delete button, then decorative chevron. Let
the row's existing navigation handler receive primary-button activation and
pointer/touch on its padding and chevron; the existing delete handler stops
propagation. The primary remains a native focus-visible button. The phone
chevron's row-owned target is at least 44px square, separate from the delete
and primary hit areas. No confirmation, new copy, API or state change.

```text
Before original: | name and row padding    [delete]  > |
Published:       | [name               >]  [delete]   |  <- old > can delete
Correction:      | [name              ]    [delete]  > |  <- row/chevron open
Tab order:         Add -> name -> delete
Phone:             name / delete / right chevron open, separate touch areas
```

The work order defines an independent real-browser coordinate oracle: click or
tap the old chevron position and row padding, prove the selected editor opens
and no deletion occurs; separately prove delete still deletes only its target.
Capture causal RED at the frozen head before production correction, then run
only affected checks. Preserve the original running PR observer across the head
change. No retry, rebase, broad replay or optional polish is authorized.
