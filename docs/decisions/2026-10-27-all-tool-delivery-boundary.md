# ADR-2026-10-27-all-tool-delivery-boundary: Bound all tool reports and observe writer failure independently

**Status:** accepted
**Date:** 2026-10-27
**Area:** protocol, backend

## Context

The generic-only guard leaves typed tool reports exposed to the same oversized-event failure.
A search completion contained 1,048,607 text bytes and exceeded the 1 MiB journal cap after JSON encoding.
The failed writer stopped forwarding and cancelled the native turn.
The backend then showed an active session for about eleven hours without its real terminal outcome.

The user selected a long-term fix with a modest limit increase, shared preparation, aligned budgets, and reliable recovery.
The earlier generic-only scope no longer defines the full repair boundary.
The user then required a simple repair with the existing interface unchanged.

## Decision

Use one shared preparation boundary for every tool report before queue admission and journal sequencing.
Preserve classification, identities, actual status, permission boundaries, and trusted provenance.
Reduce only display data and keep the native result unchanged.

Raise the default encoded event limit to 2 MiB.
Keep aggregate display data within 1536 KiB and reserve space for delivery metadata.
Keep the existing 1 MiB prompt limit independent of event capacity.
Align live queues, byte-bounded replay, response reads, and disk headroom with that contract.

Reuse existing typed truncation fields, generic preview values, and rich-content text markers.
Add no common marker namespace, canonical metadata projection, output widget, or preview control.
Keep existing recovery copy, layout, and actions.
Do not change accepted hashes, rewrite replayed events, add full-output storage, or require a format migration.

Expose writer failure through the existing authenticated status endpoint.
Observe that state independently of output delivery and browser focus.
A failed ordinary append cannot remove the sole native-event consumer before bounded cancellation drain.
Actual committed terminals and existing native recovery proof remain the only settlement authority.

## Consequences

Ordinary near-limit reports gain useful headroom.
Arbitrarily larger display results become explicit bounded previews.
A storage failure activates the existing recovery surface promptly while retaining uncertainty and Stop access.
The repair adds no UI component, new label, or localization work.

Byte-bounded replay must preserve progress after short pages.
Mixed-version fixtures must prove old-reader hashes and retained-producer behavior.
The new event limit does not widen prompts or require replacement of live agents.

The [2026-10-22 decision](2026-10-22-bounded-tool-display-data.md) remains valid for generic representation.
This decision extends its category scope and replaces its assumption that the event cap remains 1 MiB.
The existing [recovery decision](2026-10-12-total-session-recovery.md) retains authority over uncertainty and admission.

## Alternatives considered

- Raise the cap alone: handles this example but leaves the next oversized result and writer failure exposed.
- Keep 1 MiB: preserves an unnecessarily tight boundary for ordinary provider output plus encoding overhead.
- Add only a search exception: leaves other typed and future tool reports exposed.
- Store or fragment every original result: adds retrieval, retention, and cleanup contracts without a current product requirement.
- Publish uncommitted output or success after a failed append: violates durable acceptance and outcome authority.
- Wait for another event or browser action: leaves a silent connected writer failure undiscovered.
- Clear all recovery fences after Stop: can override live owners, independent failures, or uncertain external effects.

## Related contracts

- [Tool requirements](../specs/platform/requirements/bounded-tool-delivery.md).
- [Tool design](../specs/platform/system-design/bounded-tool-delivery.md).
- [Failure design](../specs/platform/system-design/durable-delivery-failure.md).
- [Fix package](../plans/resilient-agent-output-delivery/plan.md).
