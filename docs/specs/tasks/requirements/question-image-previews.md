---
status: active
system: tasks
created: 2026-10-24
owners:
  - kandev
---

# Question image previews

## Overview

An operator reviews a workspace image beside the question that asks about it.
The operator can inspect the image, choose an option, and add permitted text
without finding a separate image elsewhere in chat.

The task system owns this capability because clarification messages own the
question identity, persistence, answer, and blocking lifecycle. The agent tool
and web interface expose that contract.

Source: [issue #4748](https://github.com/kdlbs/kandev/issues/4748).
The paired [system design](../system-design/question-image-previews.md) defines
transport and security. The [plan](../../../plans/question-image-previews/plan.md)
defines delivery. The user approved implementation. Local changes implement this contract; it does
not yet describe a released version.

## Terms

- **Bundle:** The questions in one accepted `ask_user_question_kandev` call.
- **Image reference:** A stable image ID, relative path, optional repository scope,
  and accessible description. It identifies a live workspace file, not a saved copy.
- **Source session:** The session that owns the durable clarification messages.

## Requirements

### REQ-TASKS-QUESTION-IMAGES-001: Images associated with questions

**Intent:** Keep visual evidence and its decision together without changing answers.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGES-001.1:** An agent shall be able to attach zero to four
  image references to each question. A bundle shall contain at most four distinct
  image IDs. Several questions can reference the same image with identical metadata.
- **AC-TASKS-QUESTION-IMAGES-001.2:** Each image shall appear above its associated
  prompt and choices. Multiple questions about one image shall remain independent
  questions, each with its own options and permitted text.
- **AC-TASKS-QUESTION-IMAGES-001.3:** Bundles shall retain the existing one-to-four
  question limit, two-to-six option limit, single-choice semantics, custom-text
  policy, submission behavior, and whole-bundle rejection behavior.
- **AC-TASKS-QUESTION-IMAGES-001.4:** Answers shall remain keyed by question ID.
  The question's saved image references shall identify the relevant image without
  positional matching between a separate report and the answer.
- **AC-TASKS-QUESTION-IMAGES-001.5:** Invalid reference structure, duplicate image
  IDs within a question, conflicting reuse across questions, or excess references
  shall fail validation before a bundle is accepted. Omitting images shall retain
  existing question behavior.

### REQ-TASKS-QUESTION-IMAGES-002: Authorized and bounded image reads

**Intent:** Show task evidence without granting access to unrelated files or servers.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGES-002.1:** A preview shall resolve only a saved reference
  within the source session's authorized task workspace or attached source scope.
  Request data shall not select another task, session, host path, or workspace.
- **AC-TASKS-QUESTION-IMAGES-002.2:** Reads shall enforce existing task access and
  workspace execution permissions. An unauthorized caller shall receive no image
  bytes or existence details. Parent-question bundles shall remain inaccessible
  through the operator-question preview route.
- **AC-TASKS-QUESTION-IMAGES-002.3:** Absolute paths, home shortcuts, traversal,
  external URLs, data URLs, and escaping symlinks shall not grant image access.
  Rendering shall not fetch external URLs or publish a gallery server.
- **AC-TASKS-QUESTION-IMAGES-002.4:** The first version shall render validated PNG
  and JPEG files of at most 10 MiB and 16 million pixels. Unsupported, corrupt,
  oversized, missing, and inaccessible files behind accepted references shall show
  a local preview error. Unsupported filename suffixes shall fail authoring validation.
  Such errors shall not answer, reject, expire, or otherwise resolve a question.
- **AC-TASKS-QUESTION-IMAGES-002.5:** Local and remote executors shall read files
  through their authenticated runtime connection. Unavailable or older runtimes
  shall produce a retryable or unsupported preview state without a broader read fallback.

### REQ-TASKS-QUESTION-IMAGES-003: Accessible desktop and phone inspection

**Intent:** Let the operator inspect details without losing the current answer.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGES-003.1:** Thumbnails shall preserve the full image aspect
  ratio without cropping. Thumbnail height shall not exceed 128px on desktop or
  96px below the 768px phone breakpoint, at the standard root font size. A short
  prompt with one image shall remain visible on entry in the standard desktop and
  phone test viewports. Larger bundles and long content can require panel scrolling.
  The active question shall load its images on demand.
  Hidden questions and unopened historical previews shall not fetch image bytes.
- **AC-TASKS-QUESTION-IMAGES-003.2:** A labelled control shall open image inspection
  with fit and actual-size views. Closing inspection shall restore the question,
  focus, scroll position, selected option, and text draft without submission.
- **AC-TASKS-QUESTION-IMAGES-003.3:** Keyboard users shall reach image controls and
  close inspection with Escape. Viewer keys shall not select answers, navigate
  questions, collapse the question panel, or submit the bundle.
- **AC-TASKS-QUESTION-IMAGES-003.4:** Phone users shall have the same review and
  answer capabilities. Inspection shall use a focused full-height surface with
  safe-area clearance, internal scrolling, and controls with at least 44px hit targets.
  The document shall have no horizontal overflow.
- **AC-TASKS-QUESTION-IMAGES-003.5:** Loading, unavailable, unsupported, and retry
  states shall have accessible labels. Application copy shall be localized.
  Image descriptions shall remain agent-authored content. Reduced-motion and
  forced-colors preferences shall remain usable.

### REQ-TASKS-QUESTION-IMAGES-004: Durable references and lifecycle compatibility

**Intent:** Recover the question and its associations without changing ownership.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGES-004.1:** Accepted references shall survive message
  storage, live transport, page reload, and backend restart. Pending and historical
  views shall use the source session, even when another session is selected.
- **AC-TASKS-QUESTION-IMAGES-004.2:** References shall resolve the current file
  content on a fresh read. Deleted files, removed sources, retired environments,
  and inaccessible executors shall show unavailable previews. The system shall
  not substitute another session's similarly named file or retain an image archive.
- **AC-TASKS-QUESTION-IMAGES-004.3:** Exact request retries shall preserve bundle
  identity. A changed image reference shall not reuse a pending bundle through
  content deduplication. Image failures shall preserve existing wait, timeout,
  detached answer, turn-supersession, and first-writer-wins behavior.
- **AC-TASKS-QUESTION-IMAGES-004.4:** Image-free requests, native provider questions,
  external answer clients, and rich-output file blocks shall retain their existing
  behavior. Existing clients can ignore additive image metadata. Agent-facing
  documentation shall explain supported formats, limits, references, and answer mapping.

### REQ-TASKS-QUESTION-IMAGES-005: Images associated with choices

**Intent:** Compare visual alternatives without selecting an option during inspection.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGES-005.1:** An operator question shall accept zero or one
  image reference per choice. Question and choice images can coexist. Each question
  shall contain at most four references in total. The bundle limit remains four
  distinct image IDs. Repeated IDs shall have identical metadata across placements.
- **AC-TASKS-QUESTION-IMAGES-005.2:** Each preview shall remain visibly associated
  with its choice label. The selection action and inspection action shall be separate
  accessible controls. Clicking, tapping, or keyboard-activating inspection shall not
  select, advance, submit, or reject a question. Closing inspection shall restore
  focus, draft, selection, and position. Existing selection and keyboard shortcuts remain.
- **AC-TASKS-QUESTION-IMAGES-005.3:** Choice references shall survive live transport,
  reload, durable history, pending listings, and exact retries. Changed choice images
  shall change request identity. Answers shall retain their existing question and
  option IDs without image IDs replacing either identifier.
- **AC-TASKS-QUESTION-IMAGES-005.4:** Choice previews shall use the same source-session
  authorization, formats, limits, path rules, errors, and local/remote transport as
  question previews. A reference from another question shall not authorize a read.
  Missing images shall leave the associated choice selectable. Parent questions
  shall reject image fields on choices as well as on questions.
- **AC-TASKS-QUESTION-IMAGES-005.5:** Desktop and phone choices shall use compact,
  uncropped previews and the existing image inspector. Phone inspection targets
  shall measure at least 44px in both dimensions. Historical previews shall load
  only on expansion and preserve choice labels. Image-free choices and older
  answer clients shall keep their existing behavior.

## Out of scope

Multiple images on one choice, image uploads in answers, annotations, image editing,
HTML/SVG rendering, external image URLs, image archives, interactive rich-output
forms, and larger question bundles are excluded. PNG and JPEG cover the screenshot
workflow. Additional formats require a later scoped extension.

This feature does not redesign the answer envelope or independently fix
[issue #4667](https://github.com/kdlbs/kandev/issues/4667). It does not extend
`ask_parent_question_kandev` or native provider question schemas.

The existing [question barrier](user-question-turn-boundary.md),
[active lifecycle](clarification-active-lifecycle.md), and
[response reliability](clarification-response-reliability.md) remain authoritative.
