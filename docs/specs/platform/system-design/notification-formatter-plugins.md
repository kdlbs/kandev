---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-001
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
---

# Notification formatter plugins system design

## Status and source evidence

This checkout implements the optional Go `pluginsdk.NotificationFormatter`
interface, the `FormatNotification` and `InspectNotificationConfiguration` RPCs,
and provider-scoped formatter management. Published SDK `0.1.0` does not include
these additions; coordinated private builds use the explicitly pinned checkout.
The historical research observations below describe the original baseline,
not absence of the API implemented in this contribution.
The maintainer approved the split direction in the
[follow-up comment](https://github.com/kdlbs/kandev/issues/5011#issuecomment-6683963471),
including two coordinated contributions and desktop/mobile screenshots. Detailed
SDK/API review belongs in the contributions, not another direction-approval gate.
The operator subsequently confirmed grilling Q1-Q8; the plan records those choices.
The [boundary ADR](../../../decisions/2026-11-06-notification-formatter-plugin-boundary.md)
records alternatives and release consequences.

Research baseline is official v0.99.0, commit
`f0e2e774cfe2df8a0d15e4c21dea7b48580e38fb`. Upstream main was fetched as
`08a36b3fe0d49e1dd23007f30fc82ac426a2c1c8` on 2026-11-06; an exact proto/plugin
source search also found no notification/formatter entry point there. Reconcile
the entire implementation boundary against freshly fetched main before coding.
This design worktree remains pinned to the release research baseline.
During grilling, latest stable was verified as v0.99.1 at
`813a741b21108ea8ed10ab58954d85c2e5205999` and main as
`5264536ba9845f966966c2a371c7e8136ccdf119`. Public SDK latest was 0.1.0;
main's 0.2.0 metadata had no assigned host release. These are dated observations,
not permanent version selections. Refresh exact inputs before implementation and
again before operational upgrade.

| Source | Observation |
| --- | --- |
| `pluginsdk/proto/kandev/plugin/v1/plugin.proto`, service `Plugin` | Optional extension pattern exists; no notification formatter RPC. |
| `internal/notifications/service/service.go`, `deliverOccurrence` | Provider enabled/subscribed filtering and delivery claim precede `dispatchProvider`. |
| Same file, `HandleInboxItem`/`dispatchGenericNotification` | Inbox has a separate path; source workspace/type must survive until formatting. |
| `internal/notifications/service/runtime_updates.go` | Runtime batch claims are per member; format only the already-claimed message. |
| `internal/notifications/providers/webhook.go`, `newWebhookPayload` | Version 1.0 projects default copy and task/session/occurrence IDs only. |
| `internal/plugins/manifest/manifest.go`, `Action` | Existing action scopes are workspace/task/repository, not notification provider. |
| `docs/public/plugins-authoring.md`, backend matrix | OnEvent is best-effort bus delivery, not an admitted-send callback. |

Backend paths above are relative to `apps/backend/`. Legacy implementation
evidence is fork feature commit `63b8e4841019a3624fc2c3812c95716560f3ca44`,
deployed source `15495808e383c0af926a617b54cd8b2adacd9db3`, and merged fork
`production/mac` source `a381b768095350a712fcd3ec3de12afaa8327738`.
Read `templates/renderer.go`, `templates/urls.go`, service `templates.go`,
`template_values.go`, `template_catalog.go`, and store `templates.go` at those
revisions. Do not import legacy `internal/...` packages into the plugin.

## Ownership and requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-NOTIFICATION-FORMATTER-001` | Delivery boundary; semantic DTO; failures |
| `REQ-PLATFORM-NOTIFICATION-FORMATTER-002` | Provider selection; revision activation; native UI; plugin data |
| `REQ-PLATFORM-NOTIFICATION-FORMATTER-003` | Plugin parity; legacy migration; recovery |
| `REQ-PLATFORM-NOTIFICATION-FORMATTER-004` | Compatibility; verification and diagnostics |

The Kandev PR owns the generic contract, SDK/wire glue, service adapter, binding
storage/authorization, native UI mount, focused fixture, and author docs. The
independent plugin PR owns product logic, SQLite template schema, UI bundle,
tests, import utility, packaging, and release metadata. Proposed community
repository name: `illuwa-soft/kandev-plugin-apprise-templates`; creation and
bootstrap from `kdlbs/kandev-plugin-template` occur in its work order. Do not
claim it exists or publish under `kdlbs` without organization authorization.

## Delivery boundary

```text
semantic occurrence -> recipient -> enabled/subscribed provider -> existing claim
  -> immutable default/context DTO -> selected formatter (bounded RPC)
  -> validated replacement OR original pair -> existing provider.Send
```

The optional SDK `NotificationFormatter` interface and additive Plugin RPC
`FormatNotification` preserve the existing base `Plugin` interface;
old implementations return Unimplemented through SDK glue. Add manifest
`notification_formatters` declarations with a unique key, contract version,
supported provider types, and declared bounded management action keys. Host
selection requires a declared, active, compatible installed implementation.
Do not add a formatter-specific sender or event subscription.

The service depends on a narrow formatter interface wired by backend composition;
notification service code does not import plugin product code. Both ordinary
`dispatchProvider` and inbox generic dispatch use the adapter. Inbox retains
workspace/type/title context in an immutable payload before dispatch. Runtime
summary membership is constructed before formatting, as today. The seam neither
adds inbox deduplication nor changes the current transport retry contract.

Only title/body are copied into a per-provider message after validation. Never
mutate a shared message or map. Fixed-copy provider test sends bypass formatters.

## Semantic DTO and result

`NotificationFormatRequest` carries contract version 1, a host-issued provider
reference, owner identity, formatter key, selected configuration revision,
immutable default pair, event type,
occurrence ID when one exists, and a typed event context union:

- Session: exact task ID/title and verified task/session binding. A missing or
  mismatched session remains absent. No latest/primary session lookup.
- Inbox: source workspace reference, inbox type, and title; no invented item ID.
- Update: Kandev release, individual runtime notice, or already-claimed summary;
  retain applicable version/name/status/count/release URL facts only.

The DTO has no provider Config or arbitrary Payload field. Host validates
ownership before projecting facts; the plugin needs no task/session Host reads.
Bound the serialized request to 128 KiB. If required context cannot fit or be
resolved safely, skip formatting and use defaults rather than truncate identities
or expose raw objects. No invented occurrence is required for inbox.

`NotificationFormatResponse` uses `custom=false` for defaults and `custom=true`
for a complete replacement pair. Custom output is valid UTF-8 with no NUL, title at most 2,048
bytes without CR/LF and body at most 65,536 bytes. Bound the serialized response
to 512 KiB to allow JSON/protobuf escaping overhead. Empty, partial, or over-limit
custom output is invalid. No response can override
destinations, headers, config, recipients, or delivery identity.

Share one one-second formatter budget across an occurrence's provider/recipient
fanout, bounded by caller cancellation/deadline. Later providers use defaults
when it expires; their sender still receives the original delivery context.
Formatting is side-effect-free; no automatic retry is needed. On unavailable,
Unimplemented, timeout, invalid result, missing binding, or plugin-local error,
send defaults through the same admitted transport exactly once. If the caller
itself is cancelled, propagate cancellation to the transport; do not create a
fresh send context. Retain existing transport-error claim release behavior.
Late RPC responses cannot replace the selected result or trigger a send.

## Provider selection and management authorization

Propose generic `notification_formatter_bindings` keyed by provider ID with
plugin installation identity, formatter key, contract version, active immutable
configuration revision, binding revision,
and UTC timestamps. The provider relation cascades on deletion. Selection uses
an expected revision and owner/type checks in one transaction. Do not put
templates, receiver credentials, or grammar fields in this table or provider Config.
Changing provider type clears an incompatible binding atomically. Plugin upgrade
retains installation identity; uninstall invalidates it, and reinstall does not
silently acquire old bindings.

Binding selection/listing and management must run through notification-provider
ownership checks. Add a generic notification-provider resource scope to the
authenticated action model rather than treating a global settings route as a
workspace. Proposed scope `notification_provider` and verified context field
`notification_provider_id` are additive public-contract changes. The host derives
the provider owner and selected installation, forbids browser identity overrides,
and relays only declared formatter management actions through `HandleAction`.
Selection remains a core-owned operation; product CRUD/preview/import is plugin-owned.

An active management action must match the selected binding and installation
generation, including on response. Initial preparation instead uses an explicit
owner-authorized candidate selection: core validates provider ownership, declared
formatter/type, and the active installation, but does not bind delivery. Only
declared prepare/import/catalog/preview actions are admitted for that candidate;
the plugin cannot activate itself. Use indistinguishable 404s for foreign/missing
providers, 409 for revision conflicts, bounded generic errors otherwise, a
128-KiB request/512-KiB response cap, cancellation, and a five-second management
deadline. No public webhook is used. Initial import/binding requires verified
provider ownership and explicit selection; it cannot self-select from plugin code.
Recheck deleted/type-changed providers on subsequent access; inaccessible plugin
rows may remain until explicit export/cleanup, but must never be served to a new owner.

## Immutable revisions and activation

Persist plugin configuration as immutable provider-scoped snapshots. Each revision
contains the complete event override set and origin needed to render it. This
prevents an origin edit or staged deletion from changing already-admitted work.
Prepare returns an opaque revision and content digest tied to verified owner,
provider, formatter, and installation identity. Browser claims are not evidence:
before activation core redeems/verifies the candidate through the selected plugin.

First import stages and validates a snapshot with no delivery binding. Explicit
activation performs a core transaction comparing the observed binding revision
and selecting that exact prepared revision. Ordinary editing exposes one Save:
prepare the new snapshot, then compare-and-swap the active pointer. There is no
distributed transaction across host and plugin databases. If pointer selection
fails, the prepared revision remains inactive and the old pointer stays active.
If the response is lost, read the canonical binding and revision before retrying.

Capture binding, selected revision, and authorized semantic context once at
notification admission. Existing in-flight requests keep the old revision;
subsequent requests use the new one. The plugin renders the requested revision,
never its latest row. Do not hold a core SQL transaction during plugin RPC.
Provider deletion/type/owner and plugin lifecycle authorization still apply;
capturing a revision cannot preserve revoked authority. Missing/corrupt captured
revision uses the original default pair and a safe diagnostic, never another
custom revision. Cancellation prevents late responses or activation acknowledgements
from silently changing the page baseline.

Keep active and prepared revisions recoverable in the initial release; no automatic
revision garbage collector is introduced. Uninstall retains its documented data
removal behavior and therefore requires an export before recovery data is lost.
Restore defaults creates a snapshot without the selected event override and uses
the same pointer-selection path. UI shows saved-but-not-applied separately from
active, including conflicts and uncertain outcomes. Reset does not deactivate an
active formatter or destroy persisted prepared data.

## Plugin parity and data

Keep version-1 literal substitution behavior from the legacy renderer: title
source at most 1,024 bytes; body source at most 16,384 bytes; at most 64
placeholders per field; literal brace/backslash escaping; one-pass substitution;
no functions/evaluation. Missing referenced context or invalid rendering falls
back as a whole pair. Unknown fields/variables/versions are rejected at save.

Preserve `event.type`, `event.url`, `default.title`, `default.body`; task/session
variables for the two legacy session events; inbox type/title; conditional
update kind/version/previous_version/agent_name/runtime_id/status/count and
release URL. The plugin maps typed semantic DTOs to these names. It owns public
origin configuration and encoded destinations, with the legacy origin-only
HTTPS/HTTP-loopback validation. Do not add template vocabulary to the host DTO.
The first plugin supports permission-request and session-failure events as well
as the four legacy keys, using the same exact task/session rules. Legacy import
does not invent overrides or subscriptions for the additional keys. Future
unknown events use host defaults until explicitly supported.

Use plugin-owned SQLite in `KANDEV_PLUGIN_DATA_DIR` for immutable configuration
snapshots keyed by verified owner/provider/revision, their event entries, and
import receipts. One transaction commits a complete prepared snapshot/import
batch; retained content never changes under an existing revision.
Do not use Host state for pretend compare-and-swap: its API has no transactions.
Use a portable SQLite implementation compatible with the template's packaging
profile, selected and pinned during bootstrap; prove cross-builds before adding
platforms to the manifest. A checksum plus source tuple identifies an import;
identical content is a no-op, while changed target data conflicts.

## Native UI

Extend the generic Notifications settings surface with a formatter selector and
owner-bound formatter editor mount. The registry receives provider ID, binding
revision, and host-supported status as data, not authority. The independent UI
bundle registers its formatter key and uses host-owned React/UI, i18n, and shared
Save contributor APIs. No Apprise parser/editor is copied into core.

An unactivated selection defers its own save with `SettingsSaveDeferredError`.
The coordinator continues independent contributors while retaining dirty status
and refusing save-and-leave. Explicit cancellation still stops the save loop.
The editor prepares first; only explicit activation selects the prepared revision.

The plugin editor has separate subscription status, event selection, title/body,
variable help, synthetic sample selection, plain-text preview, staged restore,
and conflict/loading/error state. Save advances only the submitted baseline;
Reset discards drafts. Request generations fence stale previews. The same
stacked composition applies on phone with wrapping labels and 44px targets.
An incompatible/missing plugin keeps defaults, shows degraded status, and does
not discard drafts. See the plan's UI-01 preview.

## Legacy migration and recovery

1. Freeze only template edits for the chosen provider during operator cutover;
   do not stop running task agents merely to export configuration. Take a
   consistent private SQLite backup using the supported backup mechanism.
2. A one-time operator tool reads the offline snapshot's legacy
   `notification_templates` rows and the minimal owner/provider/type references.
   Export version, exact title/body, event, source revision/provenance, and
   separately verified public origin. Exclude provider Config and secrets.
   Write private files (0600, parent 0700), never repository fixtures.
3. Import through the owner-authorized preparation path in an isolated candidate
   first. Validate every record and commit the whole batch or none. Duplicate
   source receipts are no-ops; changed revisions/owners are conflicts, never
   overwritten silently. Revisions after import are fresh plugin tokens; retain
   legacy revision only as provenance, not as a live CAS token.
4. Compare old/new synthetic rendering and private count/content digests. The
   earlier operational snapshot had two overrides; refresh the count from the
   consistent source, rather than hardcoding two as a migration assumption.
5. Select the formatter on a candidate without the legacy render path. Never
   apply old and new rendering sequentially. The deployed custom host remains
   unchanged until explicit production direction. A future patched-legacy bridge
   must select exactly one renderer, not double-format defaults.
6. Prove restart, plugin replacement, host upgrade, and private restore. Hold the
   shared configuration-capture gate across template preparation/activation and
   binding edits while capturing host DB/config AND the plugin's SQLite snapshot.
   Ordinary task-agent activity does not require stopping solely for this capture.
   Use supported consistent database snapshot APIs, not a live file copy.
   The backup-set manifest records source versions, file digests, installation,
   provider/binding identities, and every referenced immutable revision. Reopen
   copies and verify pointer-to-snapshot resolution before declaring recovery
   complete. A host full-DB backup alone does not cover `KANDEV_PLUGIN_DATA_DIR`.

Rollback before live adoption discards only candidate state. After an authorized
cutover, quiesce template edits and binding changes and export plugin changes.
Before clearing bindings or replacing any host database, account for every host
change since the proposed restore point: task/session transitions, messages,
provider settings, subscriptions, claims, and other domain writes. Plugin exports
alone cannot preserve those changes. Prefer a verified compatible in-place
runtime rollback; otherwise require a tested lossless preservation/replay
strategy for all later host changes before restoring a version-compatible
snapshot. If later host changes cannot be verified as preserved, stop rollback
and retain the current host/plugin state and recovery evidence. A task completed
after a snapshot must never be reverted by restoring that older snapshot.
Arbitrary database downgrade is not promised. Do not replay
test notifications or restore delivery claims into a running host. Preserve
legacy rows; removal is a separate approved migration after a retention decision.
The plugin export preserves all six event configurations. A legacy restoration
tool imports only compatible four-event data with revision/owner checks; the
permission/failure entries remain in the private export. Reject unsupported
schema downgrade rather than silently replacing new edits with an older backup.

## Authorized operational upgrade

After Tasks 01-04 pass, Task 05 upgrades the operator's Kandev using the latest
stable selected at execution time, not the earlier research version. Use an
official artifact if it contains the required contract. Otherwise the operator
explicitly permits a latest-stable build with this minimal host patch. Do not
substitute an arbitrary main/nightly build or describe patched output as official.
Record stable commit, patch commit, SDK/plugin versions, artifact digest, and
service launcher before transition. A development SDK source pin is admissible
for this explicitly private patched installation, not a public stable release.

Stop before activation if the patch cannot be applied compatibly, package/restore
checks fail, or the candidate changes unrelated provider/session configuration.
Verify prior custom functionality and active-session recovery before replacement;
do not assume a newer release includes every fork-local feature. Preserve the old
runtime and coherent snapshot set. User approval covers the guarded upgrade and
plugin activation, not upstream merge, public tagging, or destructive cleanup.

## Compatibility and verification

Follow [SDK distribution](../../../decisions/2026-10-09-published-plugin-sdk.md).
Pin the first published SDK pair supporting this contract and use its SDK-wide
host minimum. Before publication, cross-repository integration pins an exact
development host/SDK source and marks the artifact non-release; remove local
SDK replacements/file links before the release check. Pin packaging tooling
independently. Never invent a future version to satisfy installation validation.

The [plan](../../../plans/apprise-template-plugin/plan.md) owns targeted tests,
rendered checks, synthetic old/new fixtures, and the host/plugin version matrix.
Existing plugins, unsupported-host rejection, cancellation/late replies,
ownership forgery, one-send fallback, and batch-claim parity are mandatory.
Backups contain personal template literals; reports expose counts/digests only.
Logs use closed reason codes and never include source/rendered/config data.
