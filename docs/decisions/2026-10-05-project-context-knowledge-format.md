# ADR-2026-10-05-project-context-knowledge-format: OKF conventions for project context

**Status:** accepted
**Date:** 2026-10-05
**Area:** protocol

## Context

Agent Projects already share ordinary files outside repository worktrees.
The default `notes.md` has no document schema or navigation index. More knowledge
can accumulate across coordinator and worker sessions than one status file can
usefully contain.

The user selected an incremental adoption of Open Knowledge Format conventions.
The format needs to support ordinary editing and existing projects.

## Decision

Use the document and index conventions from
[OKF v0.2 at specification commit `0b87c52c6ef999286c745e19998fdfcd03d5dbee`](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/0b87c52c6ef999286c745e19998fdfcd03d5dbee/SPEC.md).
Future upstream revisions require an explicit compatibility review.

New project context starts with a root index and structured notes. Concept types
remain open-ended. Provenance and lifecycle fields remain optional. Existing
plain Markdown stays readable and editable without automatic conversion.

Use index-first agent guidance and separate documents for detailed knowledge.
The editor provides advisory structure feedback. Storage accepts submitted text
without YAML serialization or format-dependent rejection. A mixed legacy context
is supported, but Kandev does not claim that every such context is an OKF bundle.

Metadata describes knowledge. It never grants authority, changes permissions,
or executes a computation. The existing
[project identity and context decision](2026-09-24-agent-project-identity-and-context.md)
continues to own storage and isolation.

The implementation contract is in the
[requirements](../specs/projects/requirements/context-knowledge.md) and
[system design](../specs/projects/system-design/context-knowledge.md).

## Consequences

New knowledge files are portable and remain usable with ordinary Markdown tools.
An index reduces the amount of unrelated context an agent needs to read.
Legacy projects need no destructive migration.

Agents remain responsible for index maintenance and factual updates.
Structure feedback cannot establish whether a claim is accurate or current.
Direct agent writes retain the existing concurrent-write limitations.
Support covers the selected document conventions, not every optional OKF consumer
feature or a complete conformance audit.

## Alternatives Considered

- Keep only unrestricted notes: preserves the current simplicity but provides
  no consistent entry point or document descriptions.
- Require full conformance for every context save: makes validation strict but
  breaks existing Markdown and prevents recovery edits to malformed headers.
- Invent a Kandev-specific document schema: permits custom behavior but adds
  a separate authoring contract and makes external knowledge less portable.
- Replace files with a database knowledge service: adds retrieval and migration
  machinery before the current file workflow needs it.
