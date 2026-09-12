# ADR-2026-09-10-mobile-confirmation-surfaces: Focus mobile confirmation in one surface

**Status:** accepted
**Date:** 2026-09-10
**Area:** frontend

## Context

Archive and other phone confirmations currently expand selected rows or replace
their trailing actions. The decision competes with surrounding list content,
and longer warnings move neighboring rows. Kandev already uses inset bottom
drawers for temporary mobile choices. Many confirmation origins are inside
those drawers, so another drawer would create competing modal/focus boundaries.

## Decision

Phone archive and existing inline confirmations use a focused confirmation
step. Reuse an open drawer as the step host; from a page, dismiss any transient
menu and open a compact bottom drawer. Keep one active surface and preserve
origin state for Cancel. An inline decision inside a centered form/dialog uses
a step in that existing boundary. Existing full task-delete, discard-consent,
type-to-confirm, and system maintenance alerts retain their current surfaces.

Share presentation components and keep domain state, eligibility, callbacks,
feedback, and persistence with current owners. The rule is phone-specific;
desktop and tablet retain their current compositions. The UI contract is in
[Mobile action confirmations](../specs/ui/requirements/mobile-action-confirmations.md).

## Consequences

Phone users get readable consequences and full-width actions while list rows
stay stable. Hosts must preserve hidden origin content, restore focus/scroll,
and cancel stale requests. Menu owners must survive their portaled content
closing. This adds explicit hosting at several existing surfaces, but does not
require a global overlay manager or change mutation/recovery semantics.

## Alternatives Considered

- Keep enlarged inline rows: less overlay coordination, but warnings still
  interrupt list scanning and move neighboring items.
- Open a new drawer for every action: simpler call sites, but nested origins
  gain a second modal layer and ambiguous dismissal/focus behavior.
- Use centered alerts everywhere: useful for existing high-consequence full
  dialogs, but less comfortable for frequent phone actions and duplicates the
  existing drawer context.
- Replace archive confirmation with Undo: appealing for reversible state, but
  runtime cleanup and session teardown require a separate recovery contract.
