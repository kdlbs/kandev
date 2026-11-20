# ADR-2026-11-08-validated-repository-locator-updates: Validate locator changes against saved repository identity

**Status:** accepted
**Date:** 2026-11-08
**Area:** backend, protocol

## Context

Repository registrations retain clone URLs independently of their Git origins.
HTTP and WS updates omit that field. Compact MCP settings already write it through the internal repository service.
The current internal write trims the URL without validating repository identity.
Re-registration deliberately preserves a nonempty stored locator.

The existing provider-origin and plugin-descriptor contracts prohibit reconstruction from an arbitrary host or browser-supplied identity.
A supported repair must preserve those guarantees across provider inspection and the later database write.

## Proposed decision

Treat a supplied clone URL as a validated change to one saved registration, not a repository selection or re-import.
Pin validation to the saved provider and complete identity. Require the active owner to inspect plugin-backed candidates.
Compare the complete result to the saved identity, including scope and immutable ID.
Refuse missing identity rather than inferring a different repository.

Expose read-only preview and validated update through the existing repository and compact settings surfaces.
Preview supplies information, not authority. Every update repeats validation.
Fence the database write against identity changes during inspection.
Preserve omitted URLs at the ordinary settings write boundary so stale saves cannot reverse repairs.

Keep Git origins, materialized checkouts, task associations, and credential policy outside this mutation.
Keep exact-administration fences and deliberate complete store writes intact.

## Consequences

Callers gain a supported repair without losing repository identity or attached work.
The service owns validation across all transports. No plugin SDK or storage migration is required.
Unknown legacy identity and provider renames require separate deliberate recovery.
Correcting registration data does not promise that an existing checkout passes exact-origin reuse checks.

## Alternatives considered

- Raw `remote_url` passthrough: rejected because it permits unrelated repositories and credential-bearing locators.
- Delete and re-register: rejected because it changes identity or preserves the old nonempty locator without repairing it.
- Reconstruct all URLs from host/namespace/name: rejected because plugin context roots and immutable scope can differ.
- Require a stored preview token: not selected because update can repeat bounded validation and fence identity at commit.
- Retarget every existing Git origin: rejected because it changes user work and bypasses checkout ownership and reuse policy.
- Add a separate MCP repair tool: not selected because compact settings already owns the field and supports domain options.

## References

- [Provider-origin identity](2026-07-20-repository-provider-origin-identity.md)
- [Provider-owned discovery](2026-10-29-provider-owned-repository-url-discovery.md)
- [Registration requirements](../specs/workspaces/requirements/remote-repository-registration.md#clone-url-repair)
- [Registration design](../specs/workspaces/system-design/remote-repository-registration.md#clone-url-repair)
- [Implementation package](../plans/git-repository-locator-repair/plan.md)
