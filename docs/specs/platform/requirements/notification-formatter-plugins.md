---
status: draft
system: platform
created: 2026-11-06
owners:
  - kandev
---

# Notification formatter plugins requirements

## Overview

Allow independently released plugins to customize notification content while
Kandev retains delivery policy and transports. Platform owns this contract
because notification providers, recipients, subscriptions, and delivery admission
belong here. Plugin packaging and lifecycle retain their existing owners.
This extends [semantic notifications](notifications.md).

The first consumer separates the operational Apprise template feature from its
fork-only host implementation. The host contract remains product-neutral. The
plugin's implementation and permanent tests belong in a dedicated repository.

## Terminology

- **Formatter:** an explicitly selected plugin implementation that can replace
  the admitted notification's title/body, but cannot send it.
- **Binding:** one provider owner's selection of one installed formatter.
- **Defaults:** the complete host-built title/body pair for that occurrence.
- **Legacy import:** one-time transfer of fork template configuration, distinct
  from runtime access to Kandev storage.

## Requirements

### REQ-PLATFORM-NOTIFICATION-FORMATTER-001: Policy-preserving formatting

**Intent:** Keep one authoritative delivery path while content evolves separately.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATION-FORMATTER-001.1:** With no formatter selected,
  recipients, copy, enabled/subscription behavior, claims, and transport failure
  handling shall remain unchanged. Installing a plugin shall not select it.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-001.2:** Formatting shall run only for
  an authorized recipient's enabled, subscribed provider, after any existing
  occurrence/member claims. It shall not alter provider identity, recipients,
  event/occurrence/session identity, claimed runtime members, or transport.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-001.3:** The formatter shall receive only
  bounded, authorized semantic context and defaults. It shall receive no provider
  config, receiver URL, credential, transcript, clarification text, or arbitrary
  internal event object through this contract.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-001.4:** An unavailable, disabled,
  incompatible, cancelled, timed-out, or invalid formatter result shall preserve
  the whole default pair. Formatting shall have a maximum one-second deadline
  shared across an occurrence's provider/recipient fanout within the caller's
  remaining budget and shall cause no second send. After that formatting budget
  expires, later providers shall use defaults without shortening the delivery
  context. Existing
  caller cancellation shall still cancel delivery rather than restart it.

### REQ-PLATFORM-NOTIFICATION-FORMATTER-002: Owned management and preview

**Intent:** Preserve per-provider control without adding product logic to core.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.1:** Only the provider owner shall
  select, clear, inspect, or manage its formatter. Foreign and missing provider
  IDs shall return indistinguishable not-found results before plugin dispatch.
  Client-supplied actor or installation identifiers shall not grant authority.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.2:** The Apprise plugin shall offer
  per-event title/body editing, event-specific variable help, synthetic preview,
  revision-conflict reporting, restore defaults, and shared Save/Reset behavior.
  Preview and edits shall not enable, subscribe, or send through any provider.
  The first plugin version shall support `session.turn_finished`,
  `session.clarification_requested`, `session.permission_requested`,
  `session.failed`, `office.inbox_item`, and `system.update_available`.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.3:** Preview shall use synthetic data
  without task/transcript reads, transport invocation, or delivery-history writes.
  Missing values shall show whole-pair fallback. Stale responses shall not replace
  another draft; acknowledged saves shall preserve subsequent edits.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.4:** Desktop and phone shall expose
  the same operations, visible accessible loading/errors, keyboard controls,
  localized labels, and no horizontal page scrolling. Coarse-pointer controls
  shall have at least 44px targets. Unsupported-host status shall prevent binding.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.5:** Before initial activation, the
  owner shall prepare and validate configuration through the editor or import without connecting it
  to delivery. Activation shall select only the exact validated revision.
  Failed preparation or activation shall retain the previous delivery selection.
  A pending selection shall not prevent shared Save from saving independent
  notification settings. Until activation or Reset, it shall remain dirty and
  prevent save-and-leave from silently discarding the selection.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-002.6:** For an active formatter, one
  explicit Save shall persist a complete new revision and attempt its activation
  against the observed binding revision. A conflict or failed activation shall
  retain the old active revision and distinguish saved from applied state. Each
  admitted notification shall use one immutable configuration revision for its
  whole title/body pair. Restore defaults shall follow the same Save semantics;
  Reset shall discard only unsubmitted edits. An uncertain activation result
  shall require authoritative readback before retry.

### REQ-PLATFORM-NOTIFICATION-FORMATTER-003: Lossless legacy transition

**Intent:** Retain working configuration and prove recovery before cutover.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.1:** Import shall preserve legacy
  title/body bytes, grammar version, provider/event associations, and source
  revision provenance. It shall not alter provider credentials, enabled state,
  subscriptions, history, tasks, or sessions. Identical reimport shall be a no-op;
  conflicting existing edits or ownership shall prevent partial import.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.2:** Old and new renderers shall
  produce identical title/body/fallback outcomes for the four legacy event keys,
  conditional update variants, missing task/session/origin, invalid values, and
  URL-encoding fixtures. New host event keys shall default unless supported
  explicitly; import shall not create subscriptions for them.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.3:** Cutover shall require a private
  consistent backup, verified import/readback, rendering parity, and a successful
  isolated rollback drill. Legacy records shall remain recoverable. Production
  activation and real-recipient sends shall require separate explicit direction.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.4:** Restart, host/plugin upgrade,
  and documented backup/restore shall preserve bindings and templates. Disable
  shall retain plugin data; uninstall shall explain its existing removal
  behavior and require export before recovery data is lost.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.5:** A recoverable backup set shall
  include the host database/configuration and plugin data with a manifest of
  versions, content digests, bindings, and referenced revisions. Template writes
  and binding changes shall be quiesced while capturing the set, without requiring
  task agents to stop solely for configuration capture. Restore shall reject a
  missing, mismatched, or unsupported referenced configuration.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-003.6:** Rollback shall export edits made
  after cutover before replacing data. It shall verify recovery of the four
  legacy event configurations where supported and preserve additional event
  configurations in export without claiming the older host can use them.
  Unsupported schema downgrade shall stop rather than discard changes.

### REQ-PLATFORM-NOTIFICATION-FORMATTER-004: Explicit compatibility evidence

**Intent:** Avoid replacing a custom host dependency with an undocumented one.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATION-FORMATTER-004.1:** Existing plugins shall remain
  source/wire compatible with additive formatter contracts. A new formatter
  plugin shall detect unsupported hosts and declare the published SDK-wide host
  minimum, without bypasses through private APIs, live SQL, or shelling into core.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-004.2:** Compatibility evidence shall
  identify exact host/plugin/SDK revisions, package digests, platform, persisted
  data versions, and results for old-host rejection, supported-host operation,
  restart, upgrade, and restore. An unchanged official v0.99.0 shall not be
  described as supporting this proposed extension.
- **AC-PLATFORM-NOTIFICATION-FORMATTER-004.3:** Diagnostics shall use bounded
  reason codes without template source, rendered content, config, credentials,
  receiver addresses, or subprocess output. Published results shall distinguish
  planned, passed, skipped, blocked, and uncertain checks.

## Out of scope

- A plugin-owned sender, new event source, new retry queue, or exactly-once mail.
- Templates for Local, System, Web Push, or Webhook in the first plugin.
- Unrequested production deployment, merge, release, or legacy-data deletion.
- Treating previous approval of core templates as approval of a new protocol.

## Related artifacts

- [System design](../system-design/notification-formatter-plugins.md)
- [Delivery plan](../../../plans/apprise-template-plugin/plan.md)
