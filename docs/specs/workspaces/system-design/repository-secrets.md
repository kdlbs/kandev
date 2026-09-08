---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
created: 2026-09-08
owners:
  - kandev
---

# Secret reference protection

## Scope and requirement mapping

This design covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9 through .11.
The existing repository-secrets requirement retains the other runtime and storage contracts during specification migration.

## Deletion boundary

`secrets.Service` authorizes the secret, checks references, and delegates deletion to the existing store.
An injected reference checker keeps the secrets package independent of profile and task repositories.
`backendapp` wires the checker from the existing agent-settings and task repositories.
It reads active agent profiles, all executor profiles, and active repositories across workspaces.
Workspace-scoped profile and repository metadata is disclosed only after workspace access succeeds.
Inaccessible references still block deletion, with their metadata omitted.

HTTP deletion returns `409` with `error`, `code: secret_in_use`, and `references`.
Each reference contains `kind`, `id`, `name`, and `key`. Hidden references contain only `kind`.
WebSocket deletion returns `CONFLICT` with equivalent details.
HTTP `?force=true` and the WebSocket boolean `force` bypass the reference check after authorization.
Missing or unauthorized secrets retain `404` behavior. Unexpected errors return sanitized `500` or `INTERNAL_ERROR` responses.

The store remains available to internal credential cleanup and workspace cascades.
No schema or foreign-key migration is required. Existing broken references remain available for manual repair.
`UserVisibleStore.DeleteForWorkspace` preserves the authorized workspace scope through the final deletion boundary.
Its default `Delete` continues to accept Global secrets only.
The check protects references present during lookup. It does not serialize concurrent profile saves with deletion across repository owners.

## Resolution and recovery

`environment.SecretError` retains redacted errors and adds a repair instruction for the source environment.
Lifecycle error wrapping includes the selected agent profile name for agent-profile failures.
Origin classification and precedence remain unchanged. Secret IDs and underlying reveal errors stay out of rendered errors.

Automatic name matching is excluded because a replacement name does not establish the original credential identity or authority.
Existing scope-transfer operations remain unchanged in this repair.

## Settings feedback and mobile parity

The existing deletion toast maps `secret_in_use` references to localized labels.
Unknown responses retain the generic localized error. Raw server error text is never rendered by this path.
The existing `SecretListItemRow` confirmation and toast are the desktop and mobile exemplars.
No layout, navigation, touch target, or scroll behavior changes. The shared action retains the row after rejection.
Component and formatter tests cover this response normalization in the existing surface.

## Evidence

HTTP and WebSocket tests exercise the real service and encrypted SQLite store.
Reference-collector tests exercise profile and repository selection, redaction, and lookup failures.
Runtime tests cover actionable errors, profile identity, redaction, and all-or-nothing resolution.
