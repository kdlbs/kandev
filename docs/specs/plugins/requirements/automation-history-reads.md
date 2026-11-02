---
status: active
system: plugins
created: 2026-10-29
owners:
  - kandev
---

# Plugin Automation History Reads Requirements

## Overview

Plugins need a supported way to recover automation observations after missed
events or a restart. Reporting, audit, calendar, and memory plugins share this
need. [Discussion #4941](https://github.com/kdlbs/kandev/discussions/4941)
describes the missing public reader.

The plugin system owns the public Host contract and its authorization boundary.
The automation service retains ownership of definitions, execution, status
projection, and retention. Each consumer owns its exports and reconciliation
policy.

## Terminology

- **Definition:** A saved automation and the safe description of its triggers.
- **Run:** A retained record of an automation firing, including an individual
  retry attempt.
- **Source status:** The status recorded on the run itself.
- **Observed status:** The automation service's current read projection, which
  can also reflect linked task state.
- **Snapshot:** An immutable, bounded collection returned for one authorized
  query. It does not promise an atomic snapshot of every Kandev subsystem.
- **Refresh:** A new observation of one known run, independent of an earlier
  snapshot or event.

## Requirements

### REQ-PLUGINS-AUTOMATION-READS-001: Workspace automation observations

**Intent:** A plugin can discover automation definitions and reconcile retained
run observations through a supported, workspace-authorized public API.

#### Acceptance criteria

- **AC-PLUGINS-AUTOMATION-READS-001.1:** An authorized plugin shall be able to
  list the selected workspace's automation definitions. Observations shall
  include identity, name, enabled state, trigger identities and kinds, and
  available schedule and timezone information. Unsupported schedule data shall
  remain explicitly unavailable without hiding its definition.
- **AC-PLUGINS-AUTOMATION-READS-001.2:** An authorized plugin shall be able to
  list run observations across the workspace or for one selected automation.
  Observations shall preserve available task, session, turn, trigger,
  retry-group, parent-attempt, and attempt-number identities. Runs without tasks
  and skipped runs shall remain representable.
- **AC-PLUGINS-AUTOMATION-READS-001.3:** Run observations shall distinguish
  source status, observed status, retry state, and delivery state. Unknown
  values and absent source timestamps shall remain distinguishable from
  success and known timestamps. An observation timestamp shall not substitute
  for an execution timestamp.
- **AC-PLUGINS-AUTOMATION-READS-001.4:** A plugin shall be able to refresh a
  known run without receiving its original event. A fresh list and refresh
  shall use the same status and summary rules for unchanged source data.
- **AC-PLUGINS-AUTOMATION-READS-001.5:** List operations shall return bounded
  pages with explicit continuation and exhaustion. Queries shall support
  narrower creation-time ranges. A query exceeding a documented snapshot limit
  shall fail explicitly instead of returning a truncated inventory as complete.
- **AC-PLUGINS-AUTOMATION-READS-001.6:** Snapshot continuations shall preserve
  their original observations. A cursor shall be bound to the installation,
  workspace, method, filters, snapshot, and approval revision. Expired,
  malformed, or mismatched continuations shall fail explicitly.
- **AC-PLUGINS-AUTOMATION-READS-001.7:** Every read shall require the declared
  automation read capability and an active approval for the selected
  workspace. Unauthorized requests shall fail before automation data reads.
  A foreign run or automation identity shall have the same not-visible result
  as a missing identity.
- **AC-PLUGINS-AUTOMATION-READS-001.8:** Revocation or approval replacement
  shall prevent continuation under the previous approval. Exact refreshes
  shall also use current authorization. Cached observations shall not grant
  continued access.
- **AC-PLUGINS-AUTOMATION-READS-001.9:** Public observations shall exclude
  credential fields, webhook secrets, request headers, raw trigger bodies,
  prompts, and internal claim tokens. Summary and error excerpts shall be
  bounded and sanitized. Public responses shall use an explicit field
  allowlist.
- **AC-PLUGINS-AUTOMATION-READS-001.10:** History reads shall leave automation,
  retry, task, session, execution, and delivery state unchanged. They shall not
  start work, retry work, repair state, or publish execution events.
- **AC-PLUGINS-AUTOMATION-READS-001.11:** The reader shall be optional.
  Discovery shall distinguish supported methods from authorized methods.
  Older Hosts shall report unsupported operations explicitly. Existing Host
  implementations and managed-conversation schedule methods shall retain
  their contracts.
- **AC-PLUGINS-AUTOMATION-READS-001.12:** Reads shall honor source retention
  and deletion. An unavailable dependency shall produce an error, not an empty
  successful inventory. A no-longer-visible run shall not imply successful
  completion.

## Out of scope

- Scheduler changes, execution commands, durable event delivery, and new
  history-retention guarantees.
- Provider exports, summaries generated by a model, calendar writes, memory
  routing, and consumer retry policy.
- Browser or frontend plugin APIs, rendered UI, new settings, and feature flags.
- Unbounded history scans or automatic splitting of oversized queries.
- Stable-version backports, SDK publication, deployment, and changes to external
  plugin repositories.

## Related contracts

- [Capability approvals](capability-approval.md).
- [Managed coordination observations](managed-coordination.md).
- [Automation runs](../../office/requirements/automation-runs.md).
- [System design](../system-design/automation-history-reads.md).
- [Implementation plan](../../../plans/plugin-automation-history-reads/plan.md).
