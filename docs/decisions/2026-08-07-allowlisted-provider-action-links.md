# ADR-2026-08-07-allowlisted-provider-action-links: Allowlisted Provider Action Links

**Status:** accepted
**Date:** 2026-08-07
**Area:** backend, frontend, protocol, security

## Context

The OpenCode TUI can render a provider failure with a workspace-specific URL,
while OpenCode's ACP service-failure response currently carries only a short,
safe message. Kandev's existing stderr diagnostic boundary intentionally strips
URLs and workspace identifiers, so the recovery card cannot offer the same
actionable destination even when a managed structured diagnostic contains it.

The existing terminal-diagnostics decision correctly rejects raw stderr and
private OpenCode log files. A narrowly scoped, validated link is useful, but it
changes the boundary that currently excludes provider workspace identifiers and
must not turn arbitrary provider text into browser navigation.

## Decision

Kandev may carry an optional `remediation_url` alongside the normalized
`ProviderError`, the persisted `LastAgentError`, and recovery-message metadata
only when an adapter-specific validator accepts the exact OpenCode route
`https://opencode.ai/workspace/<safe-workspace-id>/go`.

The validator requires HTTPS, the exact host, the exact path shape, a bounded
workspace identifier, no userinfo, query, or fragment, and a bounded complete
URL. The URL is extracted before message sanitization, while the provider
message continues to remove all URLs and identifier-bearing content. Raw stderr,
raw ACP error objects, private log files, and arbitrary URLs never cross the
agentctl boundary or reach the browser.

The adapter accepts the link from a structured ACP error field when OpenCode
provides one and from a correlated structured OpenCode stderr record. Kandev
does not reconstruct a link from a TUI-only error or read OpenCode's private log
directory. OpenCode 1.18.5 therefore keeps the existing short-error fallback
until an upstream ACP or managed-stderr contract emits the optional field.

Kanban recovery messages, the persistent last-agent-error notice, and Office
failed-session entries render the validated destination as a localized external
link. They share the same safe field and preserve the existing recovery actions
and sanitized technical details.

## Consequences

Users can open the provider's actionable recovery page when the agent transport
actually supplied it, including after a reload and on the Office surface. The
new field is deliberately useful rather than a general log-exposure mechanism.

The URL contains a provider workspace identifier, so the allowlist, bounds, and
frontend defense-in-depth check become compatibility and security contracts.
The live fix for the reported OpenCode 1.18.5 case depends on an upstream
change; without that change Kandev cannot recover information absent from ACP,
stdout, and managed stderr.

## Alternatives Considered

- **Expose raw stderr or tail OpenCode's private log:** rejected because those
  sources contain unrelated sessions, paths, URLs, and potentially sensitive
  provider data, and violate the existing diagnostic boundary.
- **Copy the entire ACP error object into the chat:** rejected because ACP
  errors are not a stable safe UI contract and may contain raw request or
  provider data.
- **Allow any HTTPS URL emitted by a provider:** rejected because a provider
  message is untrusted input and arbitrary navigation creates a phishing and
  data-disclosure surface.
- **Keep stripping every URL:** rejected because it needlessly loses a
  validated, user-actionable destination when the transport provides one.
- **Wait for OpenCode alone:** rejected as the only Kandev change; Kandev can
  prepare a safe forward-compatible contract and surface structured stderr
  links while upstream conformance is pending.
