---
status: current
system: ui
requirements:
  - REQ-UI-BUTTON-STATE-COLORS-001
---

# Button State Colors

## Ownership and mapping

The shared `@kandev/ui` Button owns default action appearance, including
disabled state. Callers retain business predicates and semantic status.
Criteria `.1`, `.3`, and `.6` map to shared disabled treatment; `.2` maps to
foreground inheritance; `.4` covers explicit success-status exceptions; `.5` maps
to the Canvas sidebar trigger variant.

## Shared disabled treatment

In `apps/packages/ui/src/button.tsx`, override the base `disabled:opacity-45`
only for the default variant. Use `disabled:opacity-100`, `disabled:bg-muted`,
and `disabled:text-muted-foreground`, plus the corresponding `dark:disabled:`
background and foreground tokens. Dark-specific disabled selectors override
feature-specific dark colors without `!important`. Keep the existing base treatment for
outline, secondary, ghost, destructive, and link variants. Use existing theme
tokens; no new global CSS selectors or palette changes are needed.

Existing feature-specific enabled background/foreground classes remain.
Disabled selectors intentionally outrank ordinary enabled color classes.
Check dark-prefixed custom classes in the rendered cascade. Do not solve a
specificity conflict with a blanket `!important` rule.

Granted notification permission and the completed settings Saved state are
explicit exceptions: keep their green status
surface through state-specific disabled classes using the existing success
background/foreground token pair in both ordinary and `dark:disabled:` variants. Unsupported permission and pending/invalid settings Save use the ordinary
neutral disabled state. Do not introduce a generic public opt-out API for these
existing status cases.

## Foreground ownership

Markdown, Monaco, and CodeMirror Save shortcuts inherit the button foreground.
Remove their local `text-muted-foreground`; retain sizing and translated labels.
The chat Send spinner also inherits the parent foreground instead of forcing
`text-primary-foreground`. Other decorative SVGs already inherit current color.

The Canvas creation trigger selects `ghost` for sidebar presentation and
`default` for settings. Retain existing navigation, callbacks, dimensions, and
localized copy. This fixes a distinct enabled-state token mismatch found by
the same audit.

## Responsive behavior

Reuse `MobileFileActions` as the phone exemplar: mode selector, overflow,
and fixed Save action remain in their existing row. Only colors change.
Desktop editor toolbars retain compact density. Test desktop, 767px/768px
boundaries, and phone/coarse-pointer surfaces without changing their layout.

## Data, security, and recovery

No stored state, API, permission, or backend contract changes. Native disabled
semantics and caller predicates remain authoritative. Loading/error recovery,
focus handling, save operations, and draft retention remain with their owners.
No new telemetry or UI copy is required.

## Verification

Use rendered computed styles for the shared color cascade. Check real Save
states, inherited shortcut/spinner foreground, representative default actions,
custom-color actions, and the notification exception in both themes. Include
normal, unavailable, and busy states. Keep measurements separate from source
inventory matches; a caller list alone does not prove a rendered defect.

The [repair package](../../../plans/button-state-colors/plan.md) owns the
inventory, regression sequence, exact commands, and evidence.
