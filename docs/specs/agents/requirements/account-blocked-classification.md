---
status: active
system: agents
created: 2026-10-13
owners:
  - kandev
---

# Provider Account-Block Classification Requirements

## Overview

A provider gateway can restrict an account and reject every request from it.
Retrying cannot recover the account, and repeated requests can prolong the
restriction. The agent system owns provider-error classification and dynamic
routing. It must recognize the restriction and keep the affected execution
profile out of routing until its owner releases it.

## Terminology

- **Account-block diagnostic:** A provider error that opens with a gateway
  `403 Forbidden` envelope and carries the `user_blocked` code or the
  message `your access has been restricted`.
- **Hold:** A routing state with no expiry and no probe that excludes one
  execution profile from selection.
- **Owner release:** An explicit request from an authenticated settings client
  that closes the hold of one execution profile.

## Requirements

### REQ-AGENTS-ACCOUNT-BLOCK-001: Classify and hold restricted accounts

**Intent:** A restricted account must stop receiving requests, while other
execution profiles keep serving work.

**User story:** As an operator, I want a restricted account taken out of
rotation until I resolve it with the provider, so that routing neither retries
it nor extends the restriction.

#### Acceptance criteria

- **AC-AGENTS-ACCOUNT-BLOCK-001.1:** When an error opens with the gateway
  `403 Forbidden` envelope and carries the account-restriction code or message,
  the system shall classify it as high-confidence `account_blocked` in the hard
  class, allow fallback, and disable automatic retry.
- **AC-AGENTS-ACCOUNT-BLOCK-001.2:** A plain `403`, the code on another status,
  prose that quotes the diagnostic, and a structured HTTP 403 shall not be
  classified as `account_blocked`.
- **AC-AGENTS-ACCOUNT-BLOCK-001.3:** For an `account_blocked` failure, route
  policy shall neither retry the same candidate nor wait for a reset.
- **AC-AGENTS-ACCOUNT-BLOCK-001.4:** The system shall hold only the execution
  profile that failed. Sibling profiles, shared credential circuits, model
  circuits, and candidate order shall not change.
- **AC-AGENTS-ACCOUNT-BLOCK-001.5:** A hold shall have no expiry, grant no
  probe, and survive a restart. A timed suspension shall not replace it.
- **AC-AGENTS-ACCOUNT-BLOCK-001.6:** Holding a profile shall not start a
  successor or replay a prompt by itself. The existing output and effect
  safety checks still decide what happens to the failed attempt.

### REQ-AGENTS-ACCOUNT-BLOCK-002: Owner release

**Intent:** Only the owner lifts a hold, and lifting it changes nothing else.

#### Acceptance criteria

- **AC-AGENTS-ACCOUNT-BLOCK-002.1:** The settings API shall provide one
  explicit release request per execution profile, behind the existing settings
  authentication and interlock. The hold stays in place until that request
  succeeds.
- **AC-AGENTS-ACCOUNT-BLOCK-002.2:** A release shall report whether a hold was
  closed. Releasing a profile without a hold shall succeed without effect.
- **AC-AGENTS-ACCOUNT-BLOCK-002.3:** A release shall not change profile
  settings, credentials, other holds, or candidate order. When the closed
  state cannot be saved, the hold shall remain and the request shall fail.
- **AC-AGENTS-ACCOUNT-BLOCK-002.4:** A missing profile and a workspace-scoped
  profile shall return not found. An unwired routing runtime shall return
  service unavailable.

## Out of scope

- Classifying other 403, authentication, or permission failures.
- Automatic release, expiry, or probing of a held profile.
- A user interface for the release request.
