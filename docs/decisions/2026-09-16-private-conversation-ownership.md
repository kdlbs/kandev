# ADR-2026-09-16-private-conversation-ownership: Default selection does not own privacy

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend

## Context

The personal-assistant binding is a mutable default pointer. Moving it exposed the previous conversation because absence of a binding was interpreted as a shared workspace conversation. Conversation history and persona memory outlive that pointer and also have configuration, automation and core task/session entry points.

## Decision

Persist the human owner on the existing Orchestration conversation registry. Claim ownership in the same transaction as selection's version check; a failed selection cannot claim a conversation. Backfill existing bindings without overwriting retained owners. Switching, deleting a binding or unregistering a persona never clears ownership or transfers history. Legacy conversations that have never been claimed remain shared.

History remains readable by its owner, subject to workspace access. A previously selected private conversation must be reselected before accepting new turns. Queue admission, launch and runtime reads/writes enforce current binding identity/version; old run credentials cannot become legacy credentials. Configuration and reimport use retained persona ownership. Background automation without durable human-owner authorization cannot target private conversations.

Core task/session access must also respect the persisted conversation owner; hiding only the Orchestration HTTP history route is insufficient. Feature disablement must not turn retained private history into shared history.

## Consequences

Selection remains reversible for the same owner without moving transcripts or memories. The additive migration and legacy import must preserve ownership on replay. Ownership is not inferred from a runtime token, role, workspace membership or automation prompt. Previously delivered text cannot be recalled, and ownership already lost before this migration cannot be reconstructed from arbitrary comments.

## Alternatives Considered

- Retain historical binding rows with a separate default pointer: valid but changes objective/operation identity and foreign-key semantics unnecessarily.
- Disallow switching: avoids this trigger but prevents the intended workflow and does not cover binding deletion.
- Protect only history HTTP reads: leaves runtime memory, launch and configuration paths exposed.

Product behavior is specified in [personal-assistant](../specs/personal-assistant/spec.md).
