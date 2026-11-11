---
status: active
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Native Change-Request Creation Requirements

## Overview

Integrations owns native change-request creation and provider-bound partial-push
retry. Users can open another creation form while an admitted request finishes.
That completion must preserve the current form's draft and repository identity.

## Requirements

### REQ-INTEGRATIONS-NATIVE-CREATE-001: Creation form ownership

**Intent:** Keep completion feedback truthful without consuming a later form.

**User story:** As a user, I want to prepare a new change request while an earlier
creation finishes, so that my new draft remains usable.

#### Acceptance criteria

- **AC-INTEGRATIONS-NATIVE-CREATE-001.1:** An admitted creation shall retain its
  session, task, repository, provider, base branch, trimmed title/body, and draft
  option through completion, including task association and outcome feedback.
- **AC-INTEGRATIONS-NATIVE-CREATE-001.2:** After another form opens, an earlier
  success, failure, rejection, or partial-push result shall preserve the current
  raw title/body, draft option, repository, focus/selection, and Submit/Cancel
  behavior. This applies to same-scope reopening and equal-value replacements.
- **AC-INTEGRATIONS-NATIVE-CREATE-001.3:** Returning to an earlier scope shall not
  restore an older request's authority. Session/task changes and disposal shall
  revoke authority to mutate the current form.
- **AC-INTEGRATIONS-NATIVE-CREATE-001.4:** A partial push/create failure shall
  reopen only its owned form, retain its raw draft, and offer retry. A registered
  provider retry in that form shall skip an already successful push; a newly
  opened form shall perform its own push.
- **AC-INTEGRATIONS-NATIVE-CREATE-001.5:** Desktop Changes controls, the native
  Git action menu, and phone Changes controls shall share these semantics.
  Submission shall retain its existing immediate-close behavior. Ordinary
  success and non-partial failure shall retain existing feedback/reset policy.

## Out of scope

Backend/provider idempotency, transport/auth changes, commit drafts, utility
text generation, navigation, and remote exactly-once guarantees.
