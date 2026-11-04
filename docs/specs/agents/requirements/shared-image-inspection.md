---
status: active
system: agents
created: 2026-10-28
owners:
  - kandev
---

# Shared image inspection

## Overview

An operator can inspect screenshot detail in the shared image preview used by
task and Office transcripts and the composer. Agents owns this extension of
the shared native file-card contract; the existing
[image expansion requirement](image-preview-expansion.md) continues to own
file-card identity, reads, and disclosure. Clarification images retain their
independent [task contract](../../tasks/requirements/question-image-previews.md).

## Requirements

### REQ-AGENTS-SHARED-IMAGE-INSPECTION-001: Inspect image detail

#### Acceptance criteria

- **AC-AGENTS-SHARED-IMAGE-INSPECTION-001.1:** When an ordinary sent or queued image attachment, generic image result, ready native rich-output image file card, or composer context image is opened, the preview shall initially fit the same image within the viewport without cropping or distorting it.
- **AC-AGENTS-SHARED-IMAGE-INSPECTION-001.2:** The preview shall offer visible, accessible Fit and Actual size actions. Actual size shall display the image at its intrinsic CSS pixel dimensions and allow the operator to inspect overflow in both axes inside a viewport-bounded region; Fit shall restore the fitted view. The actions shall work with keyboard on desktop and touch on phones.
- **AC-AGENTS-SHARED-IMAGE-INSPECTION-001.3:** Each opening and each change of image source while open shall start in Fit. Inspection shall use the exact already-selected source and shall not initiate an extra workspace read, open a file editor, change attachment delivery, or invoke an agent action.
- **AC-AGENTS-SHARED-IMAGE-INSPECTION-001.4:** The modal shall contain scrolling and keep its controls reachable, lock background interaction, and dismiss through Escape, close, and outside activation. Dismissal shall restore focus to the opener when it remains available. On phones, controls shall have at least 44px touch targets, remain within safe areas, and introduce no document horizontal overflow.
- **AC-AGENTS-SHARED-IMAGE-INSPECTION-001.5:** The existing noninteractive image branch, native file-card target replacement and disclosure behavior, and clarification image inspection and answer lifecycle shall retain their existing contracts. Image loading or decoding failure shall not prevent dismissal.

## Exclusions

Pinch or drag zoom, arbitrary scaling, downloads, galleries, editor redesign,
new image formats or preferences, and changes to network, authorization, or
attachment lifecycles are outside this capability.

## Implementation plan

- [Shared image inspection](../../../plans/shared-image-inspection/plan.md)
