# ADR-2026-10-29-provider-owned-repository-url-discovery: Provider-owned repository URL discovery

**Status:** accepted
**Date:** 2026-10-29
**Area:** backend, plugins, tasks

## Context

Office projects store repository source strings without provider hints.
The current task preflight recognizes public built-in hosts or a configured GitHub Enterprise origin.
It cannot identify self-managed GitLab or choose a plugin inspector from a bare URL.
The user wants an installed VCS plugin to make its supported URLs usable in this flow.

Plugins already own workspace-scoped `repositories.inspect` actions and return validated repository descriptors.
Browser discovery already rejects multiple successful inspectors instead of choosing registration order.
The server currently requires a provider ID before it can use the same authority.

## Decision

Resolve unhinted repository URLs through provider-owned candidates before persistence.
Public built-in URLs keep their existing route. Explicit provider selections remain pinned to that provider.
For other bare URLs, collect verified workspace GitHub/GitLab candidates and inspected candidates from active repository plugins.
Accept exactly one successful candidate. Reject multiple successful candidates and unknown URLs.
The task service owns final admission. The plugin service owns plugin eligibility, inspection, and descriptor validation.

Plugin participation requires active manifest ownership and the existing workspace inspection capability.
Installation makes that capability available, while the plugin retains its connection, authentication, and URL-ownership rules.
The host does not add Bitbucket, Forgejo, or future plugin hostnames to its built-in parser.
Project sources remain strings. The server resolves their provider at admission time.

## Consequences

Compatible VCS plugins participate without a host release for each provider.
Task creation, direct remote registration, and other shared-preflight callers use the same discovery boundary.
Explicit hints and saved repository IDs retain deterministic selection when several connections can recognize one host.
Ambiguous unhinted selections now fail instead of inheriting GitHub-first precedence.

Discovery adds bounded plugin calls only for unhinted URLs outside the public built-in path.
The plugin keeps credentials and provider-specific URL rules.
The host keeps action bounds, workspace authorization, generation checks, exact descriptors, and failure before repository/task writes.
No schema, project-source migration, wire action, or plugin SDK change is required.

## Alternatives Considered

- Infer every unknown host as GitLab. Rejected because plugin and Enterprise origins overlap.
- Hard-code supported VCS hostnames. Rejected because self-hosted origins and future plugins require provider-owned recognition.
- Preserve provider hints in project storage. Deferred because server discovery serves every bare-URL caller without a schema migration.
- Require prior repository registration. Rejected because the existing plugin inspection contract supports first-use resolution.
- Select the first successful provider. Rejected because registration order and response speed do not establish URL ownership.

## References

- [Server-owned plugin repository resolution](2026-08-26-server-owned-plugin-repository-task-resolution.md)
- [Plugin provider extensions](2026-07-31-plugin-repository-provider-extensions.md)
- [Plugin discovery requirements](../specs/plugins/requirements/repository-url-discovery.md)
- [Task admission design](../specs/tasks/system-design/project-repository-context.md#workspace-verified-remote-admission)
