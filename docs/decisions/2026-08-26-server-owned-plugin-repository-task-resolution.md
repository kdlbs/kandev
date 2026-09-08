# ADR-2026-08-26-server-owned-plugin-repository-task-resolution: Resolve First-Use Plugin Repositories on the Server

**Status:** accepted
**Date:** 2026-08-26
**Area:** backend, frontend, protocol, security, plugins

## Context

The native task dialog can list a repository from a plugin before Kandev has a
repository row for it. The browser currently submits a complete provider
descriptor. Normal REST task creation correctly refuses to trust that
descriptor, then sends the URL through the built-in parser. A Bitbucket URL is
unsupported there. The failure happens after the task row is inserted, so the
user sees an error and an empty task remains.

The browser cannot become repository authority. If a request could choose both
the provider host and clone URL, a later credential lookup could send managed
credentials to an attacker-controlled host. Requiring a separate registration
step would preserve the trust boundary but would break the native first-use
workflow already presented by the plugin picker.

The active code-host plugin already owns the external connection and can
inspect a repository URL within verified workspace context.

## Decision

Kandev will resolve a first-use plugin repository on the backend through a
standard workspace-scoped `repositories.inspect` action owned by the active
plugin manifest.

Task-create request fields are selection hints only. Kandev uses the provider
ID to find the active manifest owner and forwards the untrusted URL to that
plugin under server-verified workspace context. The plugin proves the URL
belongs to its configured connection and returns a complete, credential-free
repository descriptor. Kandev validates provider ownership, immutable
repository identity, provider scope, HTTPS clone origin, and response bounds.
Only this validated server result can enter the existing trusted-descriptor
repository resolver.

Kandev resolves and validates every first-use plugin selection before it
creates a task or repository row. An inactive provider, unavailable action,
timeout, no-match result, or invalid response returns a typed, bounded error
without partial persistence.

Existing repository IDs and built-in provider URL parsing remain unchanged.
The authenticated plugin Host `Tasks.Create` path also remains valid because
the plugin server, not the browser, supplies its descriptor. REST, WebSocket,
and MCP callers cannot set the internal trusted-descriptor marker.

## Consequences

Users can create a task directly from a repository listed by a connected
Bitbucket or future code-host plugin. The normal native task dialog remains the
single workflow on desktop and phone.

Plugin authors that support first-use task creation must implement the
workspace-scoped inspect action. Already-persisted plugin repositories continue
to work without it. Kandev gains one backend adapter between task creation and
the plugin service, but keeps credentials and provider-specific API logic out
of the task service.

Task creation now waits for one bounded provider inspection per first-use
plugin repository. Multi-repository inspection is sequential initially so
error order is deterministic and one plugin connection is not flooded.

This decision removes the known provider-resolution partial-write case. It
does not make every later task-create write transactional.

## Alternatives Considered

- Trust the complete repository descriptor from the browser. Rejected because
  a forged provider host and clone URL can redirect managed credentials.
- Require users to register the repository before opening the task dialog.
  Rejected because the picker already offers a direct first-use workflow and
  the extra step is unnecessary when the connected plugin can resolve it.
- Require each plugin to replace normal REST task creation with an
  authenticated plugin action. Rejected because it duplicates the host's
  native creation transport and makes provider behavior depend on which screen
  opened the same task dialog.
- Teach the built-in URL parser about Bitbucket and every future plugin host.
  Rejected because host-specific parsing and connection authority belong to
  the plugin, including self-managed origins and immutable provider IDs.

## References

- [Issue #3066](https://github.com/kdlbs/kandev/issues/3066)
- [Plugin Repository Task Creation Requirements](../specs/plugins/requirements/repository-provider-task-creation.md)
- [Plugin Repository Task Creation System Design](../specs/plugins/system-design/repository-provider-task-creation.md)
