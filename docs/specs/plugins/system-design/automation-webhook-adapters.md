---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-001
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-002
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-003
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-004
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-005
---

# Plugin Automation Conditions and Webhook Adapters System Design

## Purpose and boundaries

The plugin system owns registration and verification contracts. The host owns
binding authority and automation admission. Bitbucket code remains in its
[dedicated plugin repository](https://github.com/kdlbs/kandev-plugin-bitbucket).
This draft proposes new contracts; names explicitly described as proposed do not
identify existing APIs.

Existing grounding:

- `internal/automation/trigger_registry.go`: `triggerTypeRegistry` and
  `GetTriggerTypes` currently provide a fixed list.
- `components/automations/trigger-picker.tsx`: `CATEGORY_META` currently hides
  categories other than GitHub and webhook. Metadata alone cannot extend the UI.
- `internal/automation/webhook.go`: `WebhookHandler.Handle` checks
  `X-Webhook-Secret` and calls `Service.FireTrigger` with an empty dedup key.
- `pkg/pluginsdk.Plugin.HandleWebhook` and `proto/kandev/plugin/v1/plugin.proto`
  already carry raw bodies and headers but return arbitrary HTTP responses.
- `internal/plugins/manifest/manifest.go`: `Webhook.EffectiveAccess` and
  `Webhook.EffectiveMaxBodyBytes` provide existing access and size conventions.
- `internal/automation/service.go`: `Service.FireTrigger` serializes admission
  through `admitTrigger`, then publishes an event after saving the run. That
  post-insert publish boundary is not a durable delivery acknowledgement.

## Requirement mapping

| Requirement                          | Design sections                       |
| ------------------------------------ | ------------------------------------- |
| `REQ-PLUGINS-AUTOMATION-WEBHOOK-001` | Registration and editor               |
| `REQ-PLUGINS-AUTOMATION-WEBHOOK-002` | Bindings and verification; Security   |
| `REQ-PLUGINS-AUTOMATION-WEBHOOK-003` | Admission and recovery; Observability |
| `REQ-PLUGINS-AUTOMATION-WEBHOOK-004` | Lifecycle and compatibility           |
| `REQ-PLUGINS-AUTOMATION-WEBHOOK-005` | Bitbucket adapter                     |

## Registration and editor

Add proposed manifest contribution `automation_conditions`, with stable local key,
configuration version, localized label/description, provider grouping, event kind,
configuration schema and payload placeholder metadata.
Identity is `(plugin_id, condition_key)`; labels never serve as identifiers.
The RPC routes within the contributing plugin by condition key; no separate
adapter-key metadata is required. Declare external access
explicitly and advertise this optional capability through protocol negotiation.
Missing RPC support or invalid/duplicate declarations makes the contribution
unavailable rather than falling back to `HandleWebhook`.

Introduce an optional SDK interface and additive gRPC operations for condition
availability/configuration validation and webhook verification. Existing required
`Plugin` methods remain unchanged. Registration is scoped to the enabled plugin
installation and generation; workspace projections also check connection
eligibility. No plugin-specific names or event switches enter core.

Extend `automation.trigger_types` with a workspace-scoped projection merging
built-ins and eligible plugin conditions. Introduce proposed internal trigger type
`plugin_event`, with plugin/condition identity and configuration version persisted
in its configuration. Update validation, interpolation, export/import, and type
handling end to end; do not cast arbitrary plugin identities into built-in enums.

The host renders fields from a bounded declarative schema: strings, booleans,
enums, string lists, and workspace-authorized connection/repository selectors.
Unknown field types or invalid saved schema versions block editing with an
explanation. Both host and plugin validate before save. Plugin validation receives
only the binding's verified workspace/connection context. Availability checks must
not create connections or invoke automation work.

Use declared provider labels and the established plugin localization contract in
the picker instead of a fixed category allowlist. Use a searchable desktop picker
and a mobile sheet with full-width fields and reachable actions, preserving the
same host-owned form and validation state. Loading failure keeps existing values.
Expose URL copy and explicit secret reveal after save, never as hidden form defaults.

## Bindings and verification

Proposed host-owned binding model:

| Field                                    | Meaning                                                          |
| ---------------------------------------- | ---------------------------------------------------------------- |
| binding_id                               | Opaque public routing identity, not an authentication credential |
| workspace_id, automation_id, trigger_id  | Server-resolved destination                                      |
| plugin_id, condition_key                 | Exact contributing capability                                    |
| connection_id, connection_revision       | Exact provider connection and freshness                          |
| config_version, config, binding_revision | Validated filters and authority revision                         |
| secret_reference, secret_revision        | Encrypted signing secret and rotation fence                      |
| enabled                                  | Explicit user activation, separate from derived availability     |

Creation, update, reveal, rotation, and deletion use authenticated host management
operations authorized against the automation's workspace. The management response
provides a host-generated URL; payloads never select destinations. The secret is
host-generated and encrypted using existing secret-storage facilities, without
claiming the existing plaintext automation secret column is sufficient.

Add a proposed dedicated POST route
`/api/v1/automations/webhook-bindings/:binding_id`. The route is an explicit
external callback boundary, not a blanket exception for plugin endpoints. Resolve
an active binding and explicit public adapter declaration before dispatch. Share
the plugin generation lease, size enforcement, and credential-stripping policy
with existing plugin ingress. Unknown, revoked, or unauthenticated bindings return
non-enumerating authentication failures. Operational unavailability of a verified
binding is exposed to authorized users, while callback errors remain sanitized.

Read at most the configured maximum plus one byte; default to 1 MiB for this new
route. Reject over-limit bodies with 413. Preserve bytes without JSON decoding,
re-serialization, or decompression. Reject unsupported content encoding. Enforce
one HTTP method, a bounded verification timeout, and bounded adapter output.

Proposed verification request contains the raw bytes, sanitized headers, condition
identity, immutable binding configuration, exact connection context, and transient
signing secret. It does not contain ambient Kandev credentials. The plugin must
verify before interpreting event fields and returns a typed result:

- `rejected`: authentication failure, no normalized event.
- `ignored`: verified event does not match the bound condition.
- `accepted`: one normalized event and dedup identity.
- RPC error: transient verification failure; never an accepted event.

Accepted events include event kind, namespaced provider metadata, original JSON
payload, normalized JSON data, and dedup identity provenance. They cannot contain
an effective workspace, automation, task destination, or execution instruction.
Reject unknown kinds, invalid output, and results inconsistent with the bound
condition. Host supplies `data.*`, `webhook.*`, and declared provider placeholders
without silently overwriting host-owned template context.

## Admission and recovery

Persist a proposed delivery receipt uniquely by `(binding_id, dedup_identity)`.
Dedup identity must be derived from authenticated content or combined with a body
digest when the provider's delivery header is unsigned. Never trust an unsigned
request ID alone. If the provider lacks a stable event identity, use the exact-body
SHA256 digest and document that identical bodies intentionally collapse during
retention. Store the verification/binding/secret revisions, normalized payload,
receipt state, and eventual run ID. Retain dedup tombstones for seven days after
terminal handling; pending receipts are not expired. Run-history deletion does not
remove these tombstones. This retention is a proposed default for review.

After verification, recheck binding and plugin generation under the lifecycle
fence. Insert the receipt and pending admission record atomically, then return 202.
Return 200 for a duplicate or verified ignored event, 401 for bad authentication,
400 for authenticated malformed input, 413 for excess size, and 503 for transient
failures before persistence. HTTP acceptance is not a claim that an agent started.

A restart-safe worker reserves a persisted attempt before processing a receipt. Recheck
workspace, connection revision, enabled automation/trigger, binding revision, and
plugin availability immediately before admission. Revoked or obsolete receipts
become cancelled and never resume automatically. A concurrency-policy skip is a
terminal skipped receipt, matching ordinary automation policy rather than creating
an unbounded waiting queue.

Extend the existing admission boundary to atomically link the receipt to its run
and persist dispatch intent in the automation database transaction. Do not call
`FireTrigger` and subsequently save a receipt link: that would leave a crash gap.
Refactor shared admission so plugin events preserve existing policy while a durable
outbox drives publication. Redelivery uses the same run ID and the downstream
consumer's durable claim; it must not launch another task for that run. Confirm
that claim at the task-creation boundary during implementation, rather than
assuming the in-memory event bus guarantees it. This is scoped to plugin delivery
admission; existing trigger behavior must retain compatibility.

Crash validation covers receipt commit before response, receipt claim before
admission, run/outbox commit before publication, and publication before worker
acknowledgement. Deduplication guarantees one admitted run per recognized delivery,
not exactly-once external effects performed by an agent.

## Lifecycle and compatibility

Pin verification to a plugin generation lease and fence persistence/admission with
binding revisions. Disable, uninstall, upgrade, connection change, secret rotation,
and binding edits cannot authorize work through stale verification results.
Already admitted runs continue under ordinary automation lifecycle policy. Cancel
pending receipts when their authority revision is invalidated; do not reverify old
payloads using a new secret or reinterpret them under changed filters.

Keep generic webhook endpoints, their secrets, and built-in trigger identifiers
unchanged. Plugins without the optional extension load normally. The updated
Bitbucket package declares the first released compatible host version, determined
at release time rather than guessed in this draft.

Persist unavailable conditions for repair. Restore availability only for the same
plugin identity and compatible configuration schema with a valid existing binding.
Export only portable condition identity/version/configuration; omit endpoint IDs,
receipts, and secrets. Imports require explicit workspace connection rebinding and
fresh secret provisioning. Register new tables with workspace/automation deletion
cleanup; removing an automation invalidates its URL and removes scoped receipts.

## Security

This is a privileged plugin verification boundary. Host structural validation is
not independent cryptographic verification of a malicious plugin's assertion.
Installation grants alone do not authorize an automation: user-created bindings
supply narrowly scoped authority. Do not grant adapters a general `FireAutomation`
API or route through loopback HTTP with an injected `X-Webhook-Secret`.

Use the existing plugin ingress rules to strip Kandev credentials. Permit only
verification-relevant provider headers; reject ambiguous duplicate signature
headers. Never place the signing secret in events, plugin configuration projections,
logs, exports, or retained receipt payloads. The plugin receives it transiently and
must not persist it. Provider account credentials remain plugin-owned and separate
from the binding's signing secret. Payloads remain untrusted task context after
signature verification; signatures establish origin, not permission to expand an
automation's configured authority.

## Bitbucket adapter

Extend the Bitbucket plugin's manifest and optional SDK implementation; preserve
its existing OAuth `HandleWebhook` callback. Keep Cloud/Data Center parsing and
capability probes within the plugin, following the existing
[Bitbucket design](../../integrations/system-design/bitbucket-plugin-01.md).

For HMAC-SHA256, require exactly one `X-Hub-Signature` with `sha256=` followed by
a valid 32-byte hex digest. Compute HMAC over the untouched body with the binding
secret and compare with constant-time `hmac.Equal`. No generic-header fallback or
algorithm negotiation based only on untrusted input is permitted.

[Atlassian's published vector](https://support.atlassian.com/bitbucket-cloud/docs/manage-webhooks/)
provides a verification fixture. Add Cloud and supported Data Center fixtures for
new PR, merged PR, branch push, and CI/build-result events. Bind each advertised
kind to the actual product/version payload schema; a generic event header alone
is not proof of its meaning. Derive repository identity and branch matching from
the authenticated body, using target branch for PR events, pushed branch for push,
and documented commit-to-branch semantics for CI. Where a CI payload cannot prove
a requested branch match, use an authorized provider lookup or fail closed with a
clear unsupported/filter outcome; never guess from the default branch.

Product-specific delivery IDs and version support must be verified against official
documentation and captured fixtures before advertising the corresponding condition.
Unsigned metadata is used only as a hint, with body-derived deduplication. This
keeps the host contract independent of Bitbucket header spelling and product quirks.

## Observability

Expose host-owned receipt outcomes to automation-authorized users, with timestamp,
condition, safe reason code, plugin availability, and linked run. Keep receipt
acceptance distinct from run success. Record counters for verification failures,
ignored events, accepted deliveries, duplicates, cancellations, admission failures,
and dispatch recovery. Avoid high-cardinality body values and secrets in metric
labels. Retain normalized payload only as long as pending processing or existing
run-data policy requires; terminal tombstones need only identity and outcome.

## Verification approach

Use contract tests for manifest/RPC compatibility and authority fencing; handler
tests for raw-body verification, body limits, status codes, and credential stripping;
transactional crash/concurrency tests for receipts and outbox recovery; and desktop
and mobile editor tests for registration, configuration, localization, unavailable
plugins, secret reveal, and import repair. Run packaged Bitbucket against the exact
compatible host for signed acceptance and duplicate delivery. Implementation tests live beside the host and plugin code.

## Related decisions

- [Plugin automation adapter ownership](../../../decisions/2026-09-15-plugin-automation-webhook-adapters.md).
- [Webhook access gate](../../../decisions/2026-08-12-plugin-webhook-auth-gate.md).
- [Contribution lifecycle authority](../../../decisions/2026-08-04-plugin-contribution-lifecycle-authority.md).
- [Plugin localization](../../../decisions/2026-08-12-plugin-localization-contract.md).

## Implementation notes (2026-09-15)

The implementation uses additive `DescribeAutomationCondition` and
`VerifyAutomationWebhook` RPCs in the optional Go SDK `AutomationAdapter`
interface. A missing RPC makes the contribution unavailable. `config_options`
supplies bounded workspace-authorized repository suggestions; users can enter a
repository outside the first 100 suggestions, with provider access checked on save.
The Bitbucket plugin selects its single existing workspace connection rather than
creating another connection choice in the automation form.

The dedicated route is `/api/v1/automations/webhook-bindings/:binding_id` and the
management action is `automation.webhook_binding`. Bindings, receipts, and durable
secret cleanup jobs use the automation database. Receipt state `dispatch` is the
transactional outbox; the consumer atomically changes it to `processing` before
task creation. On startup, an unfinished `processing` claim becomes `failed`
with an indeterminate-outcome reason and is never executed again. There is no
claim of exactly-once external effects or replay of uncertain task creation.

The host derives identity from the exact original body digest, independently of
provider response metadata. Terminal retention starts at `finished_at`. The
host-owned payload envelope keeps original `webhook` and normalized `data` objects
separate. The `malformed` response covers authenticated invalid JSON input.

Bitbucket Cloud supports PR creation, PR merge, branch pushes, and terminal
commit-status results. Its documented CI shape carries a `links.commit.href`;
parsing that link does not fetch it. CI branch filters and Data Center CI remain
explicitly unavailable. Data Center PR and branch-push parsing uses its signed
body `eventKey`. Neither product's unsigned delivery ID expands replay authority.

The first released compatible host version remains a release-time decision.
Until that pin is set, build both sibling checkouts for webhook development.
See [the manifest contract](../../../public/plugins-manifest.md) and the Bitbucket
plugin README for the implemented setup and operational behavior.

## Review corrections and delivery policy

The [host work order](../../../plans/automation-webhook-adapters/task-01-host-adapters.md)
covers AC-001.6, AC-001.7, AC-003.6, and AC-004.5 with the original host contract.

- Hash plugin version plus installed-at timestamp into the binding revision.
  Reconfiguration after an installation change creates a fresh signing secret;
  it cancels pending/unclaimed work before replacing authority. Host restarts and
  compatible disable/re-enable retain the installation identity. Existing draft
  bindings with the old digest require reconfiguration; they fail closed.
- Persist `attempt_count` and `next_attempt_at`, migrating older receipt tables.
  Reserve at most eight processing/publish attempts, with delays of 5, 10, 20,
  40, 80, 160, 300, and 300 seconds. A still-unclaimed receipt becomes failed
  when eligible again after its last attempt. Select at most 100 eligible rows
  ordered by next-attempt time, creation time, and ID. New receipts start at
  zero and therefore cannot be permanently excluded by repeatedly failing rows.
- Complete cancellation/failure of the receipt and its unclaimed run in one
  transaction. Before deleting a trigger or binding, cancel pending/dispatch
  receipts while holding the automation lock. Already claimed execution retains
  ordinary task lifecycle semantics; never dispatch it a second time.
- Reject schedule/plugin-event combinations during creation and trigger addition,
  regardless of enabled flags. Legacy mixed records remain fail-closed; exclude
  their schedules in the scheduler query before hydration. Switching conditions
  in the editor deletes the old schedule before adding the plugin trigger.
- Validate supplied manifest defaults through the same value validator used for
  saved settings, without requiring all required fields at installation time.
- Store condition metadata by workspace in the automation state slice. Hooks
  load it once per concurrent enumeration and supply the same snapshot to the
  picker and expanded form. Give enumeration one shared five-second deadline.
- Disable binding operations during the initial lookup and fence results across
  changed triggers. Build URLs with the configured backend origin; a remotely
  reachable ingress address still depends on deployment. Use the application
  phone breakpoint (768px), desktop-density selectors, and coarse-pointer touch
  sizing. Preserve the existing localized copy and error surface.

These rules replace any earlier draft reference to an independent adapter key or
an indefinitely retried pending receipt.
