---
status: current
system: agents
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback: Policy and Persistence

## Scope and mapping

This design amends the strict-by-default implementation in PR #3473. The agents
system owns profile policy; lifecycle applies it using executor evidence.
[Part 2](no-silent-model-fallback-02.md) covers recovery and rendered surfaces.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001 | Persistence, policy, warnings, boundaries |
| REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002 | Compatible selection order |

## Persistence and API

Add `RequireExactModel bool` to `settings/models.AgentProfile`, represented as
`require_exact_model` in JSON/storage and `requireExactModel` in normalized web
profile data. Use an additive `agent_profiles.require_exact_model INTEGER NOT
NULL DEFAULT 0` migration through the existing dialect-aware migration path in
`settings/store/sqlite.go`. Cover SQLite and Postgres. Do not rewrite existing
`model`, `fallback_model`, or `auto_fallback` columns or reinterpret their values.
New profiles and seeds also default false. Reopening the store is idempotent.

Wire create/read/update, scans/inserts, clone/duplicate, reconciler preservation,
profile export/import, and the canonical settings discovery contract. Use pointer
presence on partial updates, including the enabled-only fast path. An omitted
field preserves an existing value; explicit false clears strictness. Create and
legacy import without the field use false. Old clients must not accidentally
clear an existing true value when editing another field. The response includes
an explicit boolean. Use the existing profile contract generator for snapshots.

Validation uses the merged profile on updates: strictness requires a non-empty
model on a concrete ACP profile. A dynamic selector or terminal-passthrough
profile cannot provide this guarantee and rejects an explicit strict request;
existing such profiles remain valid with false. Dynamic selection passes the
chosen concrete candidate's policy through existing execution resolution.
Do not redesign dynamic eligibility or Office routing.

Saved fallback choices may coexist with strictness as dormant values. Runtime
precedence is `require_exact_model` first, then existing `auto_fallback`, then
explicit fallback/compatible selection. This preserves values across toggling
and avoids destructive normalization. UI prevents simultaneous active modes;
API precedence makes combinations deterministic without extra booleans.

## Profile transport

Propagate the field through `settings/dto`, handlers/controller mappings,
`lifecycle.AgentProfileInfo`, `profile_resolver.go`, both policy constructors in
`manager_profile.go`, and `StartModelPolicy`. Propagate it through the internal
`orchestrator/executor.AgentProfileInfo` projection and `backendapp/adapters.go`.
Include it in the existing profile snapshot produced by
`executor.resolveAgentProfileSnapshot`; missing legacy snapshot fields mean
false. A snapshot is evidence, not a new competing source of profile policy.
Keep current profile-resolution timing: saving affects the next launch/reset/
rebind or cross-step decision, not an already-running turn. Preserve existing
model override precedence within a session.

Frontend mappings include `lib/types/agent-profile.ts`, API normalization and
serialization, settings store option conversion, editor drafts, dirty checks,
reconciliation, and settings discovery. Trace every existing `auto_fallback`
producer/consumer to check whether it also carries the new profile field.
Do not add strictness to provider wire protocols: the host owns this policy.

## Shared policy

`applyStartModelPolicy` remains the single owner of `SetModel` decisions.
Never send an ID absent from the current executor catalog. Later config layers
must not repeat a handled selection. Exactness compares full IDs, including
provider prefixes and bracketed suffixes.

For strict profiles, ignore dormant fallback choices. Require a non-empty
requested ID, advertise it, and apply it before ready/initial prompt. Empty
catalog, absent requested ID, unsupported selection, or selection error fails
with sanitized evidence. Runtime guards also reject strict+empty if storage
validation was bypassed. An intentional session override supplies the effective
requested ID under the existing override contract; this setting is not a lock
against explicit user model changes.

### Compatible selection order

Use merge base `ba960f973205854733e0dcd9afa355bdda3dddf6` as the compatibility
oracle for selection, not the PR's new strict tests:

1. Empty requested model means no explicit selection and no mismatch warning.
2. Apply the requested model if advertised.
3. With automatic fallback on and the requested model absent, continue on the
   executor current/default model. Ignore saved explicit fallback and variations.
4. Otherwise apply an advertised explicit fallback when available.
5. Otherwise resolve one distinct advertised bracketed variation of a bare ID.
   Restore the removed `uniqueAdvertisedModelVariation` runtime behavior only
   here. Deduplicate matching IDs; reject empty/nested brackets, ambiguous
   matches, and already-bracketed requests. Do not rank suffixes or catalog order.
6. Otherwise keep the executor current/default model and warn.

An empty catalog follows step 6. Unsupported model selection retains the
provider default and warns for compatible profiles. Other advertised apply
errors still fail unless existing automatic fallback permits continuation.
Do not catch initialization failure, transport disconnect, or cancellation as a
catalog mismatch. The setting does not make all failures recoverable.

| Runtime case | Strict on | Strict off, auto off | Strict off, auto on |
| --- | --- | --- | --- |
| Requested advertised and applies | Requested | Requested | Requested |
| Requested absent, fallback advertised | Error | Explicit fallback | Executor default |
| No fallback, unique variation | Error | Variation | Executor default |
| Missing/empty catalog or no match | Error | Executor default | Executor default |
| Selection method unsupported | Error | Executor default | Executor default |
| Ordinary advertised apply error | Error | Error | Executor default |
| Initialization fails/cancelled | Error | Error | Error |

The stricter PR behavior remains valuable when selected explicitly. Its tests
must set the new field instead of relying on `auto_fallback=false`.

## Warnings and existing boundaries

Reuse `ModelSelectionDecision`, the lifecycle warning event, and the persisted
`status` message with `metadata.kind=model_selection_warning`. Restore the
`unique_variation` decision outcome and existing effective-model warning path
for compatible variation selection. Preserve one warning per selection decision,
requested/effective model attribution, unknown-effective-model text, and reload
idempotency. Failed strict attempts emit errors, not successful fallback notices.
No credentials or provider configuration content belongs in metadata.

Keep gone saved models visible and unselectable in editors without rewriting
profile values. Host probes remain advisory. Do not restore editor-side ID
replacement to achieve runtime compatibility. Office post-start routing remains
owned by workspace policy; never repurpose `auto_fallback` or strictness as a
new workspace routing gate. Existing authorization and secret sanitization apply.

Preserve existing provider-specific launch preparation, including cold Claude
model exposure and request/executor/profile environment precedence. Dormant
fallback choices must not authorize strict selection. This package adds no
provider-specific environment contract, cache copying, or unadvertised selection.

## Decision

[Explicit per-profile exact model policy](../../../decisions/2026-09-15-explicit-profile-model-strictness.md)
supersedes the implicit strict default and records why a global setting and a
bulk automatic-fallback migration are not used.
