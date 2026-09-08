# Use Explicit Roots for Desktop Repository Discovery

- Status: proposed
- Date: 2026-08-27
- Area: desktop, backend, frontend, security, operations

## Context

The repository discovery endpoint scans the configured roots. When no root is
configured, it scans the process user's home to depth five. Several product
surfaces call discovery when they open.

That behavior is useful for a server or local web launch. In a macOS desktop
application, an automatic home scan can touch Desktop, Documents, Downloads, or
another protected location. macOS can then display a permission dialog without
a visible user action.

The current desktop application is unsigned. Apple explains that unsigned code
has no stable designated requirement. Ad hoc signatures identify only one code
version. Therefore macOS cannot reliably retain privacy choices across updates.

## Decision

Server-launched backends retain configured-root discovery and the Home fallback.

Desktop-launched backends retain operator-configured roots. They also use roots
that a user selects through the native folder picker or HTTP folder browser.

A desktop backend without an effective root does not scan Home. An existing
implicit-Home installation receives a confirmation action during upgrade.

If macOS Home is a root, discovery skips direct Desktop, Documents, and
Downloads children. If one protected folder is a root, discovery scans it.

The desktop bridge exposes one origin-checked folder-selection command. It does
not expose a generic filesystem plugin. The Tauri WebView does not use the HTTP
directory-listing API.

An ordinary browser can use the HTTP folder browser on a desktop backend. The
backend marker selects policy, while the client capability selects the picker.

Repository-selection surfaces show cached results first. They request a refresh
only while visible and only when the cache is at least 30 minutes old. No
repository-discovery timer runs while those surfaces are closed.

If access fails, Kandev stops automatic refresh for that root and offers a
Reconnect action. The application does not promise permanent consent while it
remains unsigned.

## Consequences

- A new desktop user performs one explicit selection before automatic local
  repository discovery begins.
- The server experience remains convenient and compatible with configured roots.
- Selecting Home does not scan its direct protected children by default.
- Configured roots remain effective after a desktop upgrade.
- Existing implicit-Home users receive a visible migration action.
- Desktop updates can still require reconnection until releases have a stable
  Developer ID signature.
- All repository-selection surfaces need one shared discovery-state contract.
- Desktop root records become a separate preference from saved repositories and
  operator-managed server configuration.
- Logs can identify the operation and target that caused an access denial.

## Alternatives considered

### Continue automatic home scans

Rejected because the application can request protected-folder access while it
is idle or while an unrelated dialog opens.

### Request Full Disk Access

Rejected because the application cannot grant it programmatically, and broad
access is not necessary for repository discovery.

### Add every protected-folder usage description

Rejected as a fix for prompt frequency because descriptions do not remove
prompts. The selected design still uses them to improve prompt text.

### Expose the Tauri filesystem or dialog plugin directly to the SPA

Rejected because direct exposure broadens the loopback WebView's native
authority. A narrow application command is sufficient.

### Run a fixed background refresh timer after consent

Rejected because a stale or lost grant can produce a prompt while the user is
away. A visible-surface freshness check provides current results without idle
filesystem access.

### Keep protected children in a Home scan

Rejected because one Home selection can cause three macOS dialogs. Users can
select a protected folder directly when it contains repositories.

### Remove configured roots from desktop policy

Rejected because configured roots already represent an operator choice.
Removing them breaks repository discovery during an upgrade.
