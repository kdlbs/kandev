---
status: draft
system: platform
created: 2026-08-08
updated: 2026-08-27
owners:
  - Kandev
---
# Provider Error Recovery Requirements

## Overview

Agent CLIs and providers report equivalent failures through different ACP frames, HTTP metadata, process exits, and diagnostic strings. Capacity, network, subscription, and quota failures need different recovery behavior, but orchestration code must not branch on provider names or raw prose.

## Requirements

### REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001: Provider Error Recovery

**Intent:** Agent CLIs and providers report equivalent failures through different ACP frames, HTTP metadata, process exits, and diagnostic strings. Capacity, network, subscription, and quota failures need different recovery behavior, but orchestration code must not branch on provider names or raw prose.

#### Acceptance criteria

- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.1:** Agent adapters collect bounded evidence from structured ACP errors, ordered ACP updates, managed stderr, process exits, and structured HTTP metadata.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.2:** Evidence is correlated to the active invocation, prompt generation, and lifecycle phase before it can authorize recovery.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3:** A shared deterministic classifier maps evidence to a stable semantic code, policy class, confidence, scope, classifier rule ID, and validated timing hints.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.4:** Provider-specific signatures live in adapter extractors and a central fixture-driven catalogue. Kanban, Office, lifecycle, and UI code do not inspect provider names or raw error strings to choose behavior.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.5:** Structured metadata and exact signatures outrank broad text patterns. Collision tests enforce deterministic priority.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.6:** Adding a known provider message normally adds a sanitized fixture and catalogue rule. It does not add an orchestration or UI branch.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.7:** Classification is deterministic in this version. Calling a model to classify an error or extract timing data is deferred.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.8:** Automatic retry, reset waiting, or fallback requires evidence tied to the current invocation and a failure boundary that is known to be pre-result and effect-safe.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.9:** While an eligible interactive transient retry is pending, desktop and mobile task chat shall show the retry reason, provider, attempt count, countdown, and Cancel action only while the backend owns that retry.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10:** When an interactive transient retry succeeds, is exhausted, reaches a terminal state, is stopped, or is cancelled, the system shall attempt to durably retire every outstanding retry notice for the session so that a successful cleanup prevents the notice from reappearing after the session becomes idle, the task changes, the page reloads, or another viewer observes the session. If listing or deletion fails, the system shall log and swallow the failure so a later authorized cleanup can retry the remaining notices.
- **AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11:** After authorization, cancelling an interactive transient retry shall attempt to retire outstanding retry notices even when the retry loop has already ended. Cancelling an active loop shall also expose manual Resume and Start fresh recovery actions. Listing or deletion failures follow AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10. A denied or foreign task-session pair shall not expose whether retry state or retry notices exist.

## System design

The migrated technical source is split into [part 1](../system-design/provider-error-recovery.md).
