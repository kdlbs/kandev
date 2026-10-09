---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
created: 2026-09-08
owners:
  - kandev
---

# Secret metadata lists and reference protection

## Scope and requirement mapping

This design covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9 through .14.
The existing repository-secrets requirement retains the other runtime and storage contracts during specification migration.

## Metadata list lifetimes

`useSecrets` in `apps/web/hooks/domains/settings/use-secrets.ts` keeps Global metadata in the
app store's `secrets` slice and Workspace metadata in hook-local state. Separate guarded
effects own the two lifetimes. The Global effect depends on scope, Global loaded/loading
flags, and the existing store setters. It retains the existing shared loaded/loading gate,
default Global list request, empty-list failure settlement, and `filterGlobalSecrets` output.

The Workspace effect depends only on scope, workspace identity, `scopedKey`, and
`initialItems`. It returns immediately for Global scope. A committed Workspace scope or
initial-items reference change initializes the local list and flags as before; an undefined
initial list admits the existing abortable workspace read when a workspace ID exists.
Supplied initial items, including an empty array, need no read. An absent workspace ID
admits no read. Cleanup cancels publication and aborts the old scoped read on a relevant
change or unmount. `loadedScopedKey` continues to hide the prior workspace's rows before
the new effect commits. Global loaded/loading transitions cannot reset these rows or flags,
abort the Workspace request, or admit another Workspace request.

`SecretsSettings` continues to apply `createSecret`, `updateSecret`, and `deleteSecret`
acknowledgments through `addSecret`, `updateSecret`, and `removeSecret`. This satisfies
AC-WORKSPACES-REPOSITORY-SECRETS-001.14 without changing the scope contracts in .1/.2
or the Global-only profile selection in .4. The supplied-list path in
`app/settings/workspace/[id]/secrets/page.tsx` and the unsupplied-list SPA route in
`src/settings-routes.tsx` use the same lifetime boundary. Workspace identity and initial-items
reference changes remain intentional replacement boundaries. Existing read failure behavior,
same-list read/mutation ordering, and late mutation callbacks after navigation are outside
this correction. No cross-instance Workspace cache, secret-value request, authorization,
API, or profile-binding change is introduced.

Targeted real-provider/store hook tests cover read lifetimes and Global sharing/filtering.
Rendered `SecretsSettings` tests use its real form/save/delete acknowledgment paths with
only transport mocked. The state/data-only mobile-parity exception applies: presentation,
touch behavior, scrolling, navigation, and breakpoint behavior do not change.

## Implementation plans

- [Workspace secret list isolation](../../../plans/workspace-secret-list-isolation/plan.md).

## Deletion boundary

`secrets.Service` authorizes the secret, checks references, and delegates deletion to the existing store.
An injected reference checker keeps the secrets package independent of profile and task repositories.
`backendapp` wires the checker from the existing agent-settings and task repositories.
It reads active agent profiles, all executor profiles, and active repositories across workspaces.
Workspace-scoped profile and repository metadata is disclosed only after workspace access succeeds.
Inaccessible references still block deletion, with their metadata omitted.

HTTP deletion returns `409` with `error`, `code: secret_in_use`, and `references`.
Each reference contains `kind`, `id`, `name`, and `key`. Hidden references contain only `kind`.
`GET /api/v1/secrets/:id/references` returns the same authorized reference projection without changing the secret. Workspace secrets require the existing `workspace_id` query parameter.
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

`SecretsSettings` renders the scope-specific description in the existing page and group headers.
The Global description names the three supported binding locations and states that saving alone
does not inject a secret. The Workspace description names repository bindings and the profile
restriction. Both descriptions remain visible in empty and populated lists on desktop and phone.
Update the existing localized settings description keys in every supported locale. No reference
lookup or secret-value request is needed to render this guidance.

Opening a secret-delete confirmation starts the read-only reference request. While it is pending, the row-local desktop popover or mobile inline confirmation remains visible with its destructive action disabled.
An unreferenced secret keeps that local confirmation and requires a separate Delete action.
Existing references replace it with the same contained conflict-dialog pattern used by agent-profile deletion. The dialog presents each visible reference as a resource card with an icon, localized type badge, name, and environment key, and offers only Close. Only the resource list scrolls when it is long; the explanation and footer remain visible.
The dialog uses full-width touch actions below the small-screen breakpoint, remains within the dynamic viewport, and never exposes secret values or inaccessible resource metadata.
The final Delete request repeats the service reference check. A reference created after preflight reopens the conflict dialog from the structured `409`; unknown failures retain the generic localized toast.
Desktop and mobile Playwright coverage proves both the safe delete and preflight-conflict paths.

## Evidence

HTTP and WebSocket tests exercise the real service and encrypted SQLite store.
Reference-collector tests exercise profile and repository selection, redaction, and lookup failures.
Runtime tests cover actionable errors, profile identity, redaction, and all-or-nothing resolution.
