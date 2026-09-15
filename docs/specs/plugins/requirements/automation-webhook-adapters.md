---
status: draft
system: plugins
created: 2026-09-15
owners:
  - kandev
---

# Plugin Automation Conditions and Webhook Adapters Requirements

## Overview

Users shall configure provider events in the existing automation editor without
building a separate webhook bridge. Bitbucket is the first integration: its
signatures are incompatible with the current generic secret-header endpoint.

The plugin system owns this capability because the durable contract is a plugin
contribution point. Automation execution remains owned by the
[Office system](../../office/README.md); provider connections remain owned by the
[integration system](../../integrations/README.md).

This is a proposed extension, not a description of shipped functionality. The
scope follows the discussion of plugin-provided conditions, verified deliveries,
and host-owned automation execution. Protocol details are proposed in the paired
[system design](../system-design/automation-webhook-adapters.md).

## Terminology

- **Condition:** A selectable provider event and its filters in “Watch for”.
- **Adapter:** Plugin capability that authenticates and interprets a delivery.
- **Binding:** User-authorized association between one condition, its automation,
  workspace, provider connection, adapter, configuration, and webhook URL.
- **Delivery receipt:** Persisted outcome used to recognize retries and expose
  delivery status independently of the automation's execution status.

## Requirements

### REQ-PLUGINS-AUTOMATION-WEBHOOK-001: Discoverable provider conditions

**Intent:** Users can configure plugin events in the existing automation editor.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-WEBHOOK-001.1:** When an enabled, compatible plugin
  contributes conditions available to the current workspace, the automation editor
  shall list them in “Add Condition” under the plugin's provider label.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-001.2:** Selecting a condition shall expose its
  supported configuration, field validation, description, and payload placeholders;
  saving and reopening shall preserve the selected condition and values.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-001.3:** Invalid configuration or a connection
  outside the automation workspace shall be rejected without changing the saved
  binding or starting work, including requests made without the editor.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-001.4:** Desktop and mobile shall support selection,
  configuration, URL copy, explicit secret reveal, and diagnosis. Mobile controls
  shall fit the viewport and remain touch-accessible; keyboard users shall have
  labeled fields, reachable actions, and restored focus after closing the editor.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-001.5:** Host and plugin copy shall follow the
  existing localization contracts; missing or loading metadata shall not erase
  saved configuration or silently substitute another condition.

### REQ-PLUGINS-AUTOMATION-WEBHOOK-002: Authorized and verified delivery

**Intent:** External callers can activate only the binding authorized by a user.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-WEBHOOK-002.1:** A user authorized to edit an automation
  shall be able to create its binding and obtain its webhook URL and generated
  signing secret. Other workspace users lacking that authority shall not create,
  edit, reveal secrets for, or delete the binding.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-002.2:** A delivery with missing, malformed,
  unsupported, or incorrect authentication shall be rejected without an automation
  run or task. A Kandev login or a generic secret header shall not bypass the
  selected adapter's verification.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-002.3:** A valid delivery shall be eligible only
  for the automation and workspace bound by the user. Caller-supplied destination
  identifiers shall not redirect it or grant additional authority.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-002.4:** Deliveries exceeding the published request
  limit shall be rejected in full, without executing the plugin or automation on
  a truncated body.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-002.5:** Secrets shall not appear in URLs, ordinary
  list/read responses, exports, logs, or delivery diagnostics. Explicit reveal and
  rotation shall require automation edit authority. After rotation, deliveries
  signed solely with the old secret shall be rejected.

### REQ-PLUGINS-AUTOMATION-WEBHOOK-003: Matching and reliable admission

**Intent:** Matching events activate the existing automation once per delivery.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-WEBHOOK-003.1:** A verified event matching the configured
  event type and repository/branch filters shall enter the bound automation's
  ordinary admission policy, target selection, prompt interpolation, concurrency
  limits, and run reporting. Nonmatching events shall not start work.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-003.2:** Repeated or concurrent deliveries of the
  same recognized event to a binding shall create at most one admitted automation
  run during the published deduplication period, including after host restart or
  deletion of visible run history. Separate bindings shall not suppress one another.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-003.3:** Once the host acknowledges durable
  acceptance, restart recovery shall retain the event until it receives an admitted,
  skipped, failed, or cancelled outcome. A crash between acceptance and dispatch
  shall neither lose the event silently nor create a second admitted run.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-003.4:** Failure before durable acceptance shall
  return a retryable error. A filtered event or an ordinary admission-policy skip
  shall have a distinguishable outcome and shall not be presented as a started run.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-003.5:** Users shall be able to see acceptance,
  duplicate, ignored, skipped, cancelled, and failure outcomes with safe reasons and
  a run link when one exists. Authentication failure shall not expose secrets or
  raw payloads in diagnostics.

### REQ-PLUGINS-AUTOMATION-WEBHOOK-004: Compatibility and lifecycle

**Intent:** Existing automations remain usable and missing plugins fail safely.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-WEBHOOK-004.1:** Existing built-in GitHub conditions and
  generic “Webhook” automations shall retain their identities and authentication
  behavior without requiring a plugin or automatic conversion.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-004.2:** Disabling, removing, or losing compatibility
  with the adapter shall preserve saved configuration, mark the condition
  unavailable, and prevent new admissions. Re-enabling the same compatible plugin
  identity shall restore eligibility only while its connection and user binding
  remain valid; it shall not replay cancelled deliveries.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-004.3:** A disconnected or replaced provider
  connection, deleted automation/workspace, or revoked binding shall prevent later
  admissions from previously received deliveries. Unrelated existing runs shall
  remain governed by ordinary automation lifecycle rules.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-004.4:** Import/export shall preserve non-secret
  plugin condition identity and configuration. Imported conditions shall remain
  inactive until their workspace connection is explicitly rebound and a new URL
  and secret are provisioned; unknown plugins shall not become generic webhooks.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-004.5:** A plugin upgrade shall not silently change
  the meaning of saved filters, reuse a removed condition identity, or expand a
  binding's authority. Incompatible versions shall require explicit user repair.

### REQ-PLUGINS-AUTOMATION-WEBHOOK-005: Bitbucket acceptance integration

**Intent:** Bitbucket users can use authenticated provider events without a relay.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-WEBHOOK-005.1:** The Bitbucket plugin shall contribute new
  pull request, pull request merged, push to branch, and CI result conditions where
  the connected product/version supplies equivalent events. Unsupported capabilities
  shall be omitted or explained, consistent with the existing
  [Bitbucket capability contract](../../integrations/requirements/bitbucket-plugin.md).
- **AC-PLUGINS-AUTOMATION-WEBHOOK-005.2:** For Bitbucket HMAC-SHA256 deliveries, the
  plugin shall authenticate the exact body using the configured secret and reject
  body tampering, wrong secrets, and unsupported algorithms. Equivalent fixtures
  shall cover both Cloud and supported Data Center versions.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-005.3:** A selected Bitbucket repository and branch
  filter shall match the event's verified payload; a valid signature for a different
  repository or nonmatching branch shall not start the automation.
- **AC-PLUGINS-AUTOMATION-WEBHOOK-005.4:** The plugin shall document event selections,
  branch meaning per condition, normalized payload fields, retry identity, and
  product/version limits. Users shall be able to complete webhook setup manually
  in Bitbucket using the displayed URL and secret.

## Out of scope

- Migrating built-in GitHub conditions to plugins or replacing existing watches.
- Plugin polling schedules, arbitrary plugin-triggered automation APIs, and
  plugin-selected workflow destinations.
- Automatic remote webhook creation, outbound webhooks, Conductor behavior, and
  a new general-purpose event processing platform.
- Exactly-once execution of arbitrary agent actions or replay protection beyond
  the documented delivery identity and retention guarantees.

## References

- [Webhook proposal #881](https://github.com/kdlbs/kandev/issues/881).
- [ClickUp use case #3264](https://github.com/kdlbs/kandev/issues/3264).
- [Watcher dispatch background #1021](https://github.com/kdlbs/kandev/issues/1021).
