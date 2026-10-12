---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-002
created: 2026-09-08
owners:
  - kandev
---

# Secret metadata lists and reference protection

## Scope and requirement mapping

This design covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9 through .14 and
REQ-WORKSPACES-REPOSITORY-SECRETS-002 (all four acceptance criteria).
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
same-list read/mutation ordering, and late mutation callbacks after navigation were outside
the independent-Workspace correction. The bounded Global success-publication contract below
extends that design without changing the Workspace effect. No cross-instance Workspace cache, secret-value request, authorization,
API, or profile-binding change is introduced.

Targeted real-provider/store hook tests cover read lifetimes and Global sharing/filtering.
Rendered `SecretsSettings` tests use its real form/save/delete acknowledgment paths with
only transport mocked. The state/data-only mobile-parity exception applies: presentation,
touch behavior, scrolling, navigation, and breakpoint behavior do not change.

## Global initial metadata publication

REQ-WORKSPACES-REPOSITORY-SECRETS-002 is owned here because the workspaces contract
already owns secret metadata scope, Global profile consumption, and list lifetime.
The Global list remains in the current `StateProvider` store. No cache owner, persisted
state, backend contract, WebSocket handler, or global revision/timestamp is introduced.

### Admission and publication

`useSecrets` uses the existing `useAppStoreApi` to read the current owning store when
its Global effect admits an initial read. Recheck that store's `secrets.loaded` and
`secrets.loading` synchronously, capture the immutable `secrets.items` array reference,
and set loading before invoking the existing default `listSecrets({ cache: "no-store" })`.
The live gate prevents two consumers with pre-effect render snapshots from each admitting
a request. Keep the effect's scope/readiness dependencies and the separate Workspace
effect; do not add metadata items as an effect reset dependency.

In the existing success callback, read `store.getState().secrets.items` synchronously.
Call the unchanged `setSecrets` action with `response ?? []` if the current array is the
captured reference, otherwise with that exact current array. `setSecrets` already assigns
items and sets `loaded` true; writing the current immutable array back preserves its
identity, membership, order, and values while settling readiness. The read, comparison,
and setter run in one callback with no await or external call between them, so there is
no interleaving gap. The existing `finally` clears `loading`. No store action, declaration,
Immer import, serial, global version, timestamp, journal, framework, or helper module
changes are needed. The hook is the only production edit.

`addSecret` deduplicates by ID and appends; `updateSecret` updates a present row in place
within the immutable next snapshot; `removeSecret` filters membership. Those actions,
and an authoritative `setSecrets` replacement, invalidate the captured array reference
when they change accepted metadata. Both HTTP acknowledgments from `SecretsSettings`
and `registerSecretsHandlers` events use these existing actions. No handler rewrite is
needed. An update to an absent row retains the existing no-insertion policy.

This is a whole-list publication fence, not a merge: after any intervening accepted
change, snapshot-only rows from that older read are not inserted. Current values and
order remain authoritative, even after removal leaves an empty list or metadata returns
to a previously equal value. Readiness still settles without a trailing read. Uncontested
reads publish all rows normally. `filterGlobalSecrets` remains unchanged, including legacy
rows with omitted scope; it is not applied to the stored response itself.

### Failure and compatibility boundary

Retain the current unguarded `catch(() => setSecrets([]))` and its loaded/loading/error
policy. There is qualified evidence for stale successful publication only. Guarding
failure settlement requires an independently authored companion causal RED proving loss
of current accepted metadata; absent that evidence it is outside this package.
Workspace identity, supplied-list replacement, abort/cancellation, profile selection,
secret-value operations, transfer, and auth/lifecycle behavior are unchanged.

### Evidence and surfaces

Real-provider/store/hook tests must exercise admission and successful publication
through the unchanged store actions under actual Immer, including mixed create/update/removal events and repeated creation.
A rendered `SecretsSettings` test must exercise the real Add secret form, shared Save,
HTTP `createSecret` adapter, registered `secrets.created` handler, and exact row presence
before and after releasing the older HTTP list response. Isolate only external transport;
do not mock the hook, store, providers, API adapters, handlers, form, save, or row widgets.
Use synthetic metadata and secret input; never reveal or inspect plaintext storage.

The state/data-only mobile-parity exception applies. Existing Settings composition,
controls, scrolling, navigation, copy, and breakpoint behavior remain unchanged. The same
store publication serves desktop and phone. Targeted component evidence proves the real
rendered workflow; no new browser, build, Playwright, ASCII redesign, operator step, or
localized copy is required for this correction.

## Implementation plans

- [Workspace secret list isolation](../../../plans/workspace-secret-list-isolation/plan.md).
- [Preserve saved Global secrets](../../../plans/preserve-saved-global-secrets/plan.md).

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
