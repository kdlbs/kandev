---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Clipboard Feedback Requirements

## Overview

People copying text repeatedly need confirmation of the most recently
acknowledged successful write. UI owns this reusable, transient presentation
contract independently of the text source. File paths, workflow exports,
editors, and chat retain ownership of their contents and actions. Browser
capability fallback remains owned by
[Auth](../../auth/requirements/secure-context-browser-fallbacks.md).

## Terminology

- **Acknowledgement:** The clipboard operation reports success after completing
  its modern API write or DOM fallback.
- **Feedback instance:** One independently owned copied indicator.
- **Feedback duration:** The interval selected by the copy action that receives
  the acknowledgement, measured from that acknowledgement.

## Requirements

### REQ-UI-CLIPBOARD-FEEDBACK-001: Renew successful copy confirmation

**Intent:** Keep copied confirmation current when successful copies overlap or
occur close together.

#### Acceptance criteria

- **AC-UI-CLIPBOARD-FEEDBACK-001.1:** Each successful acknowledgement shall
  display copied feedback for the full selected duration from that
  acknowledgement. With the default 2000 ms duration, successes at 0 and
  1000 ms shall retain feedback through 2000 ms and expire at 3000 ms.
- **AC-UI-CLIPBOARD-FEEDBACK-001.2:** Custom durations shall follow the same
  renewal rule. A zero duration shall acknowledge success and clear feedback
  on the next eligible scheduled expiration, without making feedback
  persistent or clearing it synchronously inside the acknowledgement.
- **AC-UI-CLIPBOARD-FEEDBACK-001.3:** Acknowledgement order shall determine
  renewal, even when an earlier-started write completes after a later-started
  write. Starting or awaiting a write shall not itself renew feedback.
- **AC-UI-CLIPBOARD-FEEDBACK-001.4:** A failed copy shall not report new success
  or shorten, extend, or clear feedback from a prior successful copy. Without
  prior success, feedback shall remain inactive.
- **AC-UI-CLIPBOARD-FEEDBACK-001.5:** Changing the selected duration shall not
  change an existing feedback deadline. The next successful action shall use
  the duration associated with that action, including an action retained from
  before a duration change.
- **AC-UI-CLIPBOARD-FEEDBACK-001.6:** Feedback instances shall remain
  independent. Renewing, failing, expiring, or removing one instance shall not
  change another instance's feedback or deadline. Removing an instance shall
  release its already-scheduled feedback expiration.
- **AC-UI-CLIPBOARD-FEEDBACK-001.7:** Existing copied feedback shall retain
  its localized presentation and remain usable for another copy. Copy actions
  shall preserve the exact supplied text, current fallback and focus behavior,
  and the same feedback semantics on desktop and phones.

## Out of scope

- Request-start ordering, cancellation, transport arbitration, and global
  clipboard state.
- Admission or suppression of in-flight completions after an instance is
  removed. Releasing an already-scheduled expiration is a narrower guarantee.
- Clipboard formats, persistence, new labels, layout, navigation, or focus
  refactoring.
- File path selection, validation, and refusal, owned by
  [Copy file path actions](copy-file-path-actions.md).

## Implementation plans

- [Clipboard success feedback](../../../plans/clipboard-success-feedback/plan.md)
