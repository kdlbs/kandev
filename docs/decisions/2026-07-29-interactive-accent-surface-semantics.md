# ADR-2026-07-29-interactive-accent-surface-semantics: Separate Brand Accent from Interactive Surface Fills

**Status:** accepted
**Date:** 2026-07-29
**Area:** frontend

## Context

Kandev's `accent` token is a bright brand color, while several inherited UI
patterns treated it as a neutral background for hover, focus, open, and selected
states. That combination reduced text contrast and obscured semantic status
colors on icons. The problem appeared across shared menu primitives and
application-specific selectors, so local fixes alone would keep drifting.

## Decision

- Transient interactive states such as hover, focus, and open use a neutral
  `muted`, `input`, or card-derived surface with the normal foreground.
- Persistent non-semantic selection uses a neutral or card fill plus a
  `primary` accent border or inset ring. Components reserve border width or use
  an inset ring so selection does not change layout.
- True primary actions use the matched `primary` and `primary-foreground` token
  pair. Semantic status fills keep their domain-specific colors.
- Shared menu, context-menu, menubar, select, and table primitives preserve
  explicit semantic icon colors instead of forcing descendants to an accent
  foreground.
- The bright brand `accent` token remains available for intentional brand
  emphasis, but not as the default interactive surface fill.

No product spec changes are needed because this standard changes visual token
usage without changing product capabilities, navigation, or workflows.

## Consequences

Selection remains recognizable through the accent boundary without placing
text or colored icons on a saturated background. Hover and keyboard-focus
states remain visible in light and dark themes. New interactive components must
choose transient, selected, primary-action, or semantic-status styling
explicitly instead of relying on the generic accent-background convention.

## Alternatives Considered

- **Neutralize the global `accent` token.** Rejected because it would remove the
  existing bright brand color from intentional accent uses and silently alter
  unrelated surfaces.
- **Patch only the reported PR controls.** Rejected because the same contrast
  failure existed in shared primitives and other selectors.
- **Add another interaction-background token.** Rejected because existing
  `muted`, `input`, and card surfaces already express the needed neutral states.
