# ADR-2026-09-10-canvas-creation-authority: Authorize the owner's initial canvas release

**Status:** accepted
**Date:** 2026-09-10
**Area:** backend, protocol, security

## Context

A user asks their task agent to create a canvas, then must approve its first
data permissions before opening it. The current release policy treats that
locally requested application like an externally obtained package. Source user
metadata exists, but does not distinguish delegated creation from ownership of
an import. Future marketplace packages need explicit review.

## Decision

Treat owner-authorized local creation as a single-use delegation for the first
validated release. The host records that authority at creation and consumes it
atomically with release activation and exact declared grants. It never trusts
an agent or manifest field that claims ownership or local provenance.

Initial grants cover supported task-scoped Kandev data operations, events,
instance state, and declared exact HTTPS origins. They do not permit remote
scripts, native plugin execution, or workspace scope. The runtime continues to
intersect declarations, grants, scope, and current resource authorization.

Subsequent permission increases, revoked access, and promotion require explicit
review. External package ownership does not authorize execution. Existing
drafts and releases do not receive authority through a migration or read path.

This amends the grant-approval interpretation of
[the plugin-backed canvas decision](2026-08-26-plugin-backed-web-app-canvases.md).
It does not change its sandbox, data ownership, or immutable artifact rules.

## Consequences

The requested local canvas opens after first publication. The host must persist
and transactionally consume creation authority, and keep import paths distinct.
Initial local code can exercise its declared supported writes and HTTPS network
access without another prompt; creating the canvas is the authorization point.
Later publication is not ongoing blanket approval. Current pending canvases
retain their existing manual review path.

## Alternatives Considered

- Review every first release: preserves the current repeated-consent problem.
- Trust every user-owned package: lets an imported package bypass review.
- Let the manifest request automatic approval: gives untrusted code authority
  over its own grants.
- Automatically approve every later local release: can restore revoked grants
  and hide permission expansion behind a normal edit.
- Approve only reads at creation: leaves locally requested interactive and
  network-backed canvases gated immediately after creation.

## Related records

- [Creation requirements](../specs/canvases/requirements/local-creation-authority.md)
- [Creation design](../specs/canvases/system-design/local-creation-authority.md)
- [Fix plan](../plans/canvas-runtime-permission-fixes/plan.md)
