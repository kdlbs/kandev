# ADR-2026-09-14-error-scope-and-history: Error scope and conversation history

**Status:** accepted
**Date:** 2026-09-14
**Area:** frontend

## Context

The startup recovery card precedes the transcript history loader. Special scroll ownership tries to keep it visible during pagination.
After user scrolling releases that ownership, another history page can move the card out of view.
Existing recovery-message filters also remove errors after later messages. This loses useful conversation context after automatic recovery.
Shared task errors can affect several sessions and non-chat views.

## Decision

Session failures are durable chronological chat entries. Successful recovery removes active controls, not the historical entry.
The normal transcript scroll controller owns these entries. No error-specific reveal or top-pinning policy remains.
Task and shared workspace failures appear once in the task shell, above tabs and outside their content scrollers.
Error scope comes from the failure owner. An initiating session or the presence of a repository identifier does not determine scope.
Shared task errors have an independent projection, so session failures cannot hide them.
Existing recovery permissions, stamps, provider identity, and sanitized details remain authoritative.

## Consequences

Users can read the failure followed by resumed agent work. Session errors can scroll out of view as ordinary messages.
A shared alert stays visible across tab switches. Its details use a desktop dialog or phone drawer to preserve content space.
Failure identity and recovery eligibility must remain separate from historical message visibility.
This decision applies within an affected task. It does not introduce an installation-wide alert center or workspace-wide broadcast system.

## Alternatives Considered

- A fixed session banner keeps errors visible but duplicates conversation history and consumes permanent chat space.
- A special entry above all history requires competing scroll rules and does not preserve chronological context.
- Removing errors after recovery hides why the conversation stopped and restarted.
- A shared error inside Chat disappears on other tabs and misrepresents its scope.
- Inferring scope from error text or counting failed sessions can misclassify unrelated failures.
