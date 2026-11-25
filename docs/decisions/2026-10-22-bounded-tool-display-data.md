# ADR-2026-10-22-bounded-tool-display-data: Bound generic tool data before acceptance

**Status:** accepted
**Date:** 2026-10-22
**Area:** protocol

## Context

Generic GitHub MCP results exceeded the durable event limit, stopped forwarding
and interrupted the turn. The user selected a marked bounded Kandev copy and
then explicitly requested a focused implementation without surrounding machinery.

## Decision

Bound generic JSON input/output and duplicate content before queue admission
and journal sequencing. Retain `truncated: true` and a bounded `preview` inside
the existing generic JSON value. Preserve native agent results, actual status,
identity and trusted provenance. Apply the same helper at both adapter emission
and the process persistence boundary. Do not rewrite accepted events on replay.

Using the known arbitrary-JSON field preserves current reader/hash contracts.
The earlier draft's new typed marker, protocol/journal versions and SQL/hash
migration are unnecessary and are excluded from the narrowed package.

## Consequences

An oversized generic result no longer prevents later events or completion from
crossing delivery. The existing generic JSON renderer exposes the omitted-data
flag on desktop and phone. The retained Kandev copy is intentionally incomplete;
there is no full-output side store or download. Preparation bounds its own work
and allocation after source decoding; native/provider memory is outside scope.

## Alternatives considered

- Raising finite record limits moves the failure to another result size.
- Lossless fragmentation adds storage/retrieval work the user did not select.
- A typed envelope marker adds reader/hash migration work this fix does not need.
- Reducing already accepted evidence changes replay hashes and is forbidden.
- Limiting GitHub requests leaves other valid tools exposed to the same failure.

## Related contract

[Requirements](../specs/platform/requirements/bounded-tool-delivery.md),
[design](../specs/platform/system-design/bounded-tool-delivery.md) and
[focused plan](../plans/bounded-tool-delivery/plan.md).

The [all-tool boundary decision](2026-10-27-all-tool-delivery-boundary.md) extends this policy to typed reports and increases event capacity.
