---
status: active
system: projects
created: 2026-10-05
owners:
  - kandev
---

# Project Context Knowledge Requirements

## Overview

Project context gives users and agents a durable knowledge base across coordinator
and worker sessions. A small entry point helps agents find relevant knowledge
without reading every file. Structured Markdown makes that knowledge portable.

Projects owns this contract because it owns the shared context and its lifetime.
The existing [Agent Projects requirements](agent-projects.md) continue to own
task membership, access, storage isolation, and lifecycle behavior.

The user selected incremental adoption of the Open Knowledge Format (OKF).
Existing Markdown remains readable. The selected baseline is
[OKF v0.2](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/0b87c52c6ef999286c745e19998fdfcd03d5dbee/SPEC.md).

## Terminology

- **Concept document:** A Markdown knowledge file with YAML frontmatter and a
  descriptive `type` string.
- **Index:** An `index.md` file that links to knowledge documents with short
  descriptions. A root index can declare `okf_version`.
- **Legacy Markdown:** A Markdown file without a frontmatter block.
- **Format feedback:** Advisory information about document structure. This
  information does not certify the content or change file access.

## Requirements

### REQ-PROJECTS-CONTEXT-KNOWLEDGE-001: Portable context and compatibility

**Intent:** Give new projects useful starter files without converting existing
knowledge or restricting ordinary text editing.

#### Acceptance criteria

- **AC-PROJECTS-CONTEXT-KNOWLEDGE-001.1:** A new project shall contain a root
  `index.md` that declares OKF version `0.2` and links to `notes.md` with a short
  description. Its notes shall have `type: Project Notes`, a title, a description,
  and sections for current status, decisions, and handoffs.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-001.2:** Repeating project creation or context
  provisioning shall preserve every existing starter file byte for byte,
  including an empty file. Provisioning can create a missing starter file.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-001.3:** An existing project shall remain usable
  without an index or structured notes. Upgrade, launch, and resume shall not
  convert its files or require a format migration.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-001.4:** Opening or saving a concept with an
  unfamiliar type or metadata key shall not reject or remove that information.
  Optional provenance and lifecycle metadata shall remain optional.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-001.5:** Context files shall remain editable as
  text. A save shall preserve the submitted content, including comments,
  frontmatter ordering, and Markdown body, without automatic normalization.

### REQ-PROJECTS-CONTEXT-KNOWLEDGE-002: Relevant knowledge and agent authoring

**Intent:** Make the shared knowledge discoverable and keep short status notes
separate from detailed knowledge.

#### Acceptance criteria

- **AC-PROJECTS-CONTEXT-KNOWLEDGE-002.1:** On launch and resume, all project
  roles shall receive instructions to read the index before notes and relevant
  documents. A missing index shall fall back to notes and directory inspection.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-002.2:** The system shall not automatically
  insert the entire context bundle into an agent prompt. Project files shall
  retain their existing read and write access boundaries.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-002.3:** Coordinator instructions shall direct
  the agent to keep concise notes and use separate documents for detailed knowledge.
  They shall also direct index maintenance after document changes.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-002.4:** Agent instructions shall explain
  concept frontmatter and the special roles of `index.md` and `log.md`. They
  shall direct agents to preserve unfamiliar metadata and legacy content.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-002.5:** Provenance and verification metadata
  shall not grant permissions or certify a document automatically. Instructions
  shall prohibit invented source or verification claims.

### REQ-PROJECTS-CONTEXT-KNOWLEDGE-003: Advisory format feedback

**Intent:** Help users understand and repair structure while keeping editing
available on desktop and phone.

#### Acceptance criteria

- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.1:** The context editor shall identify
  plain Markdown, structured concept metadata, index files, log files, and
  ordinary text. Plain Markdown shall remain usable without a warning that
  demands conversion.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.2:** When frontmatter cannot be inspected
  or a concept header lacks a nonempty string `type`, the editor shall show
  concise advisory feedback. The feedback shall not disable opening or saving.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.3:** Feedback shall describe the current
  draft and update after edits without another file request. A version declaration
  beyond the supported baseline shall produce advisory information, not rejection.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.4:** Format feedback shall distinguish
  structure checks from verification of knowledge. It shall not present an OKF
  conformance certificate or a trust score.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.5:** Desktop and phone shall provide the
  same editing and feedback outcomes. Feedback shall wrap without horizontal
  page overflow or loss of editor focus. Desktop actions shall retain ordinary
  28px sizing, and phone or coarse-pointer actions shall retain 44px hit targets.
- **AC-PROJECTS-CONTEXT-KNOWLEDGE-003.6:** Format feedback shall preserve the
  existing content-conflict behavior. A conflicting save shall keep the user's
  draft and report the conflict, regardless of document format.

## Out of scope

- Automatic conversion of existing documents or automatic knowledge extraction.
- An OKF import/export API, search index, graph, or Markdown preview renderer.
- Automatic trust scoring, verification, or execution of attested computations.
- Mandatory provenance fields or a centrally controlled concept-type registry.
- Changes to shared-context storage, authorization, concurrent-write policy,
  runtime flags, or cross-machine synchronization.

## Implementation Plans

- [Project context knowledge](../../../plans/agent-project-context-knowledge/plan.md)
