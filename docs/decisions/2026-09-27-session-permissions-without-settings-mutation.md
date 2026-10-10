# ADR-2026-09-27-session-permissions-without-settings-mutation: Prefer session permission controls

**Status:** accepted
**Date:** 2026-09-27
**Area:** protocol

## Context

The user requires task modes to leave shared Claude user settings unchanged.
PR #3886 currently installs a session-owned settings overlay on isolated executors.
It does not directly edit the host user file. However, it redirects settings
paths, complicates authentication, and treats file delivery as necessary.

The reviewed Claude ACP release supports the SDK permission-mode control through
ACP. Kandev already applies session layers before its first prompt. An unchanged
process argument does not establish that the runtime permission mode stayed unchanged.
The evidence and exact source version are in the
[system design](../specs/agents/system-design/agent-permission-control-integrity.md#evidence-and-correction).

## Decision

Prefer advertised ACP session controls. Apply and confirm the selected mode
before the first prompt. Use provider-specific process controls only when their
contract is verified and the effect stays within that process.

Do not automatically write agent settings files or redirect their directories
to apply permission modes. Preserve separately selected authentication and
portable-settings transfers.

No shared-settings fallback ships now. If later evidence requires one, it needs
a separate agent-profile option, disabled for both new and existing profiles.
The user must enable it manually after the interface explains its shared scope.
Mode selection and automatic approval never grant that consent.

This decision supersedes
[ADR-2026-09-25-session-mode-configuration-boundary](2026-09-25-session-mode-configuration-boundary.md).
The earlier prohibition on implicit host-settings or credential transfer remains.

## Consequences

- Permission modes use the same session boundary across executors.
- Mode changes do not alter authentication discovery or unrelated sessions.
- Confirmation must accept authoritative session settings responses.
- An unmet explicit start mode holds the prompt and exposes the reason.
- Real-provider behavior still needs evidence. Mock tests prove Kandev routing only.
- A future shared-settings fallback needs a separate design for concurrency,
  user edits, rollback, and profile consent. This ADR does not authorize its implementation.

## Alternatives Considered

- Keep session-owned overlays as the default. Rejected because ACP provides the
  required control without settings-path and credential coupling.
- Edit global settings then restore them. Rejected because another session can
  read the temporary value, and restoration can overwrite a user edit.
- Pass arbitrary CLI flags to the ACP bridge. Rejected because its ACP entry
  point does not forward those flags to the wrapped CLI.
- Pass `permissionMode` in creation metadata. Rejected for the reviewed release
  because the bridge overwrites that field before it creates the SDK query.
