---
status: current
system: costs
requirements:
  - REQ-COSTS-PROVIDER-USAGE-SNAPSHOT-001
  - REQ-COSTS-PROVIDER-TURN-ALLOWANCE-002
---

# Provider usage snapshot design

## Composition and feature boundary

`features.providerUsageLimits` / `KANDEV_FEATURES_PROVIDER_USAGE_LIMITS` uses the
typed runtime registry and remains off in prod/dev/e2e. Changes require restart.
Only enabled startup injects `ProfileUsageProvider` into agent settings.
`GET /api/v1/agent-profiles/usage` returns 404 without this provider before
enumerating profiles or accessing credentials (AC-001.4).

The existing Office adapter remains intact. The snapshot lister shares its
`UsageService` and account cache keys but does not depend on Office being enabled.
No scheduler, model execution, account hold or persisted setting changes.

## Binding and reading

Enumerate live global concrete profiles using agent row names, not UUIDs as
provider names. Use existing Claude/Codex clients only for supported host
credential bindings, the existing explicitly selected TeamClaude proxy resolver,
and a new OpenCode Go client. Go paid models share a credential cache entry;
free Go and Zen models have no provider quota API. Unknown prefixes, remote
executor preferences and credential-changing profile environment overrides do
not borrow the host credential (AC-001.1, .2, .5).

Snapshot DTOs carry profile/model/provider/account identity, billing class,
source, bounded state/reason and optional timestamps/windows. The account ID is
a public provider family identifier for these single host credential bindings;
raw credential locations stay internal. Missing or invalid reset/window data
is omitted or marked unavailable, never converted into zero or unlimited quota.
`binding_scope: host_default` and the localized default-local-account label make
the observation scope explicit: a profile can later run with an executor or task
override, so this snapshot does not claim to measure that execution's account.
Explicit loopback proxies use `local_proxy`; unresolved bindings stay unknown.
Go windows retain their paid-model scope and explicit provider exhaustion.
Existing provider fetch behavior, fetch coalescing and cache TTL remain unchanged.
A preserved reading carries the existing shared cache's stale flag through the
snapshot DTO. The profile row keeps its measured windows and original read time
visible alongside a localized failed-refresh notice; it never relabels an earlier
reading as a newly successful refresh.

Provider windows carry internal observation metadata, excluded from the existing
Office JSON. The snapshot accepts a percentage only when the provider reported
a valid number, including zero, and includes a reset only when reported by the
provider. Legacy Office scalar defaults and fallback resets are preserved.
Profile `OPENCODE_CONFIG_CONTENT` overrides also leave the host binding unknown.
Profile environment-key comparisons follow host OS rules: Windows ignores case;
Unix preserves case. The stored profile and launch environment are not rewritten.

## Settings display

The costs-owned display is embedded in ordinary profile rows. One store-scoped
shared request cache includes backend/auth identity and profile version. Initial
mount and explicit refresh read the API; no interval is installed. Late responses
from an old scope cannot publish into a new one. Disabled rows mount no reader.
Provider errors remain visible, with no raw error/body rendered. Text wraps on
phone, and refresh stays outside the whole-card link with a 44px target
(AC-001.3, .5).

## Shared turn allowance increment

The task SQLite/PostgreSQL repository owns two narrow additions: account policy
JSON (owner and learned stored separately) and durable turn claims. No second
usage cache, polling loop or queue is introduced. The existing snapshot binding
resolver and Office UsageService remain the shared observation source. Admission
uses internal bindings, never a round trip through the snapshot HTTP DTO.

Policy is canonical user material; its owner is the existing task repository
writer. Claims are operational state with the same writer and a delivery identity
revision. They have no automatic expiry: only known-unsent release or confirmed
turn/runtime termination changes their charge. Policy/claim locations are native
SQL tables; the policy JSON and claims recover them after restart. The settings
API is their observer. Neither disabling nor UI omission deletes them. Existing
unknown/learned JSON survives owner field patches. The existing task repository
remains the single SQL owner; no parallel state registry or usage cache is added.

Named turn-allowance settings routes retain the ordinary agent-settings global
profile boundary. Missing, deleted and Office workspace profiles return 404
before provider/account lookup. PATCH also retains the existing organization
configuration permission and settings interlock; the internal Office prompt
consumer keeps its separately verified runtime binding.

The first policy unit is `kandev_turn`, with independent optional concurrency and
fixed-window count limits. Zero disables that explicit dimension; absent means
unconfigured. An integer turn claim never asserts a provider HTTP request,
provider-wide usage, token estimate or monetary reservation. The API states that
provider spend remains unknown on this surface. Free and paid models can share
an explicit Kandev-turn policy without applying paid spend windows to free work.

At enabled startup composition wires a narrow lifecycle admission callback and
settings service. The common `callAgentctlPrompt` consults it after existing
admission and before I/O. Submission identity, or execution plus prompt generation,
owns one claim; steers reuse the current turn claim. Replay reuses it. Confirmed
pre-write rejection refunds it; response loss/write ambiguity retains it. Validated
completion returns concurrency without refunding period use. Confirmed stop also
ends concurrency; survivable detach does not. No expiry guesses that an uncertain
remote effect did not happen.
Local initial-prompt failures keep their existing lifecycle/UI effects but are
not remote terminal evidence for provider claims. Only the trusted stream path
settles a numbered completion. A later matching remote terminal frame may settle
its still-retained claim even when that local failure already finalized the UI
prompt; execution and generation must still match. This does not replay terminal
UI publication or refund period use. Confirmed-unsent refunds remain at dispatch
and cannot release a predecessor's ambiguous claim.

Actual execution locality and effective credential environment must match the
supported host binding. A potentially limited provider with unknown binding is
closed, not silently routed through another account. Credential/cache keys remain
private. No credential fingerprint or new identity hash is added.

The account row serializes policy changes and claims. SQLite uses its existing
immediate writer admission; PostgreSQL locks the account row. Provider reads happen
outside SQL transactions. Existing grade admission returns its reservation after
releasing its mutex, before this boundary. No allowance transaction invokes grade
admit/release or publishes callbacks, avoiding DB-to-grade lock inversion.

At the current admission authority, a Manager snapshot also supplies active
incumbents from before enablement or policy creation. Account-local concurrency
unions these with durable claims by submission/execution generation. An incumbent
is not fabricated as a newly charged period turn. Unknown active binding or
generation closes configured admission. An atomic execution marker covers an
unmetered turn between its no-policy decision and agentctl acknowledgment; the
settings/admission mutex orders that marker with policy changes. No snapshot lock
is held over provider/SQL work. Settlement never acquires this admission mutex.

Other units/scopes, learning from natural errors, actual remote account readers,
the editor and aggregate reset/learning UI remain original scope. No general
provider-request enforcement or whole-T1 completion is claimed.

Unresolved binding only closes potentially applicable provider limits. Verified
native Claude/Codex families and the supported actual OpenCode model prefixes
can establish that another provider's policy is disjoint. Same-provider unknown
accounts and unmapped/custom-wrapper provider origins remain closed. An unknown
incumbent from a provably different provider does not block this account.

Claude provider-selection variables are passed through profile/runtime env to
the SDK adapter. The snapshot and allowance binding reject an unverified nonempty
selector instead of labeling it as the host Anthropic subscription. This covers
the five selectors documented in the [official environment reference](https://code.claude.com/docs/en/env-vars).
That reference describes empty provider selectors as unset and generally describes
false Boolean values as off; the [official action](https://github.com/anthropics/claude-code-action/blob/main/base-action/action.yml)
instead treats nonempty provider-selector values as truthy. Without proof for the
installed CLI version, a nonempty value, including `false`, means binding unknown,
not a definite third-party provider. Empty/absent host defaults remain intact.
Native profile env resolution drops empty literals, so a profile's empty selector
does not prove an inherited process selector was unset. An actual runtime snapshot
with an explicit empty selector does establish that override. No CLI setting or
operating environment is changed by these guards.

Normal-task and Office prompt delivery are covered. Utility/review
`agentctl.InferencePrompt` uses another child/HTTP path; C2 child-owner integration
must attach it to this same claim seam after releasing the grade mutex. It remains
original work rather than covered behavior of this increment.

## Related contracts

- [Subscription usage](../requirements/subscription-usage.md): existing Office behavior.
- [Runtime flags](../../../decisions/0007-runtime-feature-flags.md).
- [Implementation and evidence](../../../plans/provider-usage-snapshot/plan.md).

## Startup and verified stale-runtime settlement

Composition installs the existing shared usage/allowance adapter after Office
construction and before required-store readiness, event consumers, deferred
recovery and scheduling. Admission is immutable during running startup work,
so the enabled path cannot dispatch through an unwired nil admission pointer.

Stale-execution cleanup also settles claims once the runtime backend confirms
stop, or an already-retired local owner's termination has been proven. Missing
backend identity alone is not proof. Settlement is bounded independently of
caller cancellation; failure is logged without changing the existing runtime
cleanup result. Graceful survivor detach, restart/map absence and ambiguous
remote outcomes retain claims. No expiry or absence-based reconciliation is added.
