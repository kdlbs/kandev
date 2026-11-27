---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-SECRET-CATALOGUE-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
created: 2026-10-10
owners:
  - kandev
---

# Workspace Secret Catalogue System Design

## Purpose and boundaries

This design extends the workspace-local publication lifetime in
[repository secrets](repository-secrets.md#metadata-list-lifetimes). Its earlier
same-list ordering exclusion describes that correction's scope; this pair is
the authority for workspace initial-success publication after local mutations.
The original scope and independent-list contracts remain authoritative.

Workspace state stays local to `useSecrets` in
`apps/web/hooks/domains/settings/use-secrets.ts`. Global metadata stays in the
existing `StateProvider` store. No store action, cache owner, backend contract,
persisted revision, timestamp, event handler, or reconciliation service changes.

Sibling PR 4398 at `a589325537d2b70ff66f02f289c1d1ad62f88127` independently
defines REQ-WORKSPACES-REPOSITORY-SECRETS-002 and the Global initial publication
section in `repository-secrets.md`. Retain those reviewed definitions and the
Global effect unchanged when that dependency lands. This design neither
allocates that ID nor copies its Global rule. The local lifecycle and the
shared Global lifecycle are independently verifiable.

## Requirement mapping

| Criterion | Design section |
| --- | --- |
| AC-WORKSPACES-SECRET-CATALOGUE-001.1 | [Components and real workflow](#components-and-real-workflow), [Success publication fence](#success-publication-fence) |
| AC-WORKSPACES-SECRET-CATALOGUE-001.2 | [Success publication fence](#success-publication-fence) |
| AC-WORKSPACES-SECRET-CATALOGUE-001.3 | [Admission and readiness](#admission-and-readiness) |
| AC-WORKSPACES-SECRET-CATALOGUE-001.4 | [Scope and replacement boundaries](#scope-and-replacement-boundaries) |
| AC-WORKSPACES-REPOSITORY-SECRETS-001.1, .2, .14 | [Scope and replacement boundaries](#scope-and-replacement-boundaries), [Failure and compatibility](#failure-and-compatibility) |

## Components and real workflow

The Vite route in `apps/web/src/settings-routes.tsx` mounts `SecretsSettings`
with `scope="workspace"` and `workspaceId`, without `initialItems`. Its plural
workspace route and legacy singular redirect reach the same surface. The
co-located `app/settings/workspace/[id]/secrets/page.tsx` supplies initial items;
that separate supplied path is a compatibility control, not the qualified
defect's entry point.

`SecretsSettings` composes the real `SecretForm`, shared settings Save, and
metadata rows. `useSecretRequests` applies successful `createSecret`,
`updateSecret`, and `deleteSecret` acknowledgments through the hook's existing
`addSecret`, `updateSecret`, and `removeSecret` callbacks. No mutation is
published before its existing acknowledgment boundary. `listSecrets` and
`createSecret` retain their current `fetchJson` adapters, metadata shapes,
workspace query, and create payload. No plaintext read is required.

## Admission and readiness

Keep the workspace effect dependencies: `initialItems`, `scope`, `scopedKey`,
and `workspaceId`. Initialization resets local items and flags for that lifetime
exactly as before. Undefined initial items plus a workspace ID admit one
abortable scoped read. Supplied items, including `[]`, or a missing workspace ID
admit no read. Neither scoped items nor the new mutation fence belongs in the
effect dependency list.

A current successful response always sets `scopedLoaded` true; the existing
current `finally` sets `scopedLoading` false. Suppressing stale contents must
not leave readiness pending. Local callbacks retain their existing readiness
behavior until the read settles. No trailing refresh, abort on mutation, or
re-admission is introduced.

## Success publication fence

Use one hook-local monotonic mutation generation ref. Capture its current value
when the workspace effect admits the initial request. Every workspace-local
metadata callback advances the ref synchronously before scheduling its existing
functional `setScopedItems` update. Global branches return through their
existing store actions without touching this local ref.

In the workspace success callback, first retain the existing `cancelled` check.
Publish `response ?? []` only if the captured generation still equals the
current generation. Always settle current success readiness. Keep the check
and state scheduling synchronous, without an await or external call between
them. Advancing the ref before React schedules the local update prevents
publication from depending on a completed render or on a state updater's timing.

This is a whole-list fence. If a mutation intervenes, keep the exact functional
update result; do not merge snapshot-only rows, compare array length, compare
field equality, or infer freshness from timestamps. Add followed by remove,
and rename followed by restoration, remain mutations even when current contents
are empty or value-equal again. Preserve the existing local append, map, and
filter semantics, including update of an absent ID and existing deduplication
behavior; do not import the Global store's separate mutation policy.

## Scope and replacement boundaries

Retain `loadedScopedKey` hiding of former workspace rows and the existing
effect-local `cancelled` flag plus `AbortController`. Scope, workspace identity,
initial-items reference changes, and unmount still cancel publication and abort
the abandoned request. A new lifetime captures its own current generation; an
earlier lifetime's generation does not fence an uncontested new workspace read.
The monotonic ref need not be reset or exported. Cancellation, rather than
generation equality alone, rejects a late former-workspace result.

Explicit replacement `initialItems`, including a new empty array, remains
authoritative over local mutations. Stable initial-array identity preserves the
current lifetime. Missing workspace ID admits no request. Late mutation callback
navigation hardening remains outside scope; the new tests use callbacks owned
by the current committed lifetime.

## Failure and compatibility

Retain the existing workspace catch branch: while current, it writes an empty
list and sets loaded true; current finalization clears loading. Do not extend
the generation fence to rejection without a separate causal RED and reconciled
specification. Existing failure/cancellation controls can run unchanged.

Unrelated Global load transitions retain their independent lifetime. The
reviewed sibling Global effect and its tests/spec edits are outside this
production ownership. Re-read both publication contracts on a moved or merged
base before implementation and delivery. This is a bounded local ordering fix
within the established architecture, so no new ADR or system boundary is needed.

## Evidence and desktop/phone surfaces

Independently author `use-secrets.workspace-publication.test.tsx` and
`secrets-settings.workspace-publication.test.tsx`. Use real StateProvider/store,
hook, ToastProvider, SettingsSaveProvider, form, shared Save, rows, and list/create
adapters; defer only external fetch. The principal component case must prove
GET admission, exact workspace POST payload and HTTP 200 acknowledgment,
rendered saved row, then release of the earlier HTTP 200 empty GET. Assert row
retention and no second GET. Ordinary GET and supplied-empty/no-GET creation
controls qualify the fixture independently. Hook cases exercise accepted local
callbacks, mixed changes, current-empty and value-restoration histories, flags,
scope switches, initialItems replacement, and unmount.

The nearest shipped surface is the existing Workspace Secrets settings page
and `SecretsSettingsBody`; its phone Settings composition remains intact.
Only shared state/data publication changes. The mobile-parity exception permits
meaningful hook/component evidence without a new Playwright test, browser run,
Vite build, ASCII redesign, or viewport sweep. Layout, touch, navigation,
scrolling, accessibility controls, and copy do not change.

## Implementation plans

- [Preserve saved Workspace secrets](../../../plans/preserve-saved-workspace-secrets/plan.md).
