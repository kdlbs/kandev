# Requirements

## Purpose

Requirements define what an actor or consumer must observe. They explain the
intent and the behavior without prescribing the implementation.

An actor can be a user, operator, agent, plugin, integration, or another Kandev
system. Do not force a user story when no natural user story exists.

## Document structure

A requirement document contains:

- An overview of the capability and its value.
- Terms that are necessary to interpret the requirements.
- One or more requirements with stable IDs.
- Acceptance criteria for every requirement.
- Explicit exclusions.

Add permissions, failure behavior, persistence, compatibility, or performance
criteria when they affect observable behavior.

## Requirement IDs

Use this form:

```text
REQ-<SYSTEM>-<CAPABILITY>-<NNN>
```

Use uppercase letters, digits, and hyphens. Use a three-digit sequence. Do not
reuse an ID after removal.

One requirement describes one cohesive capability or outcome. The title is a
short noun phrase.

## Acceptance criteria

`AC` means acceptance criterion. The plural term is acceptance criteria.

Use this form:

```text
AC-<SYSTEM>-<CAPABILITY>-<NNN>.<N>
```

The part before the decimal point must match its requirement ID. Each criterion
defines one independently testable behavior.

Use this sentence form when it fits:

```text
When <condition>, the system shall <observable behavior>.
```

Use `shall` for required behavior, `should` for a recommendation, and `can` for
an allowed behavior. Avoid vague terms such as appropriate, fast, robust, or
user-friendly. Give an observable result or a measurable limit.

## User stories

A user story gives useful intent when the capability has a natural actor and
outcome. Use this form:

```text
As a <role>, I want <action>, so that <outcome>.
```

A user story does not replace acceptance criteria. Omit it when it creates an
artificial actor such as "the system."

## Content boundary

Requirements can name a public API, event, or persisted concept when that name
is part of the observable contract. Requirements do not define internal files,
functions, database queries, or implementation sequences.

Put technical models, control flow, storage choices, and component ownership in
the paired system design. Put design rationale and rejected alternatives in an
ADR.

## Splitting requirements

Split a document when its requirements have different actors, lifecycles, or
contracts. Keep requirements together when they are necessary to complete one
user or system outcome.

Use references instead of copied text when another system owns a requirement.

## Cross-surface requirements

Keep one cohesive outcome in its owning system when delivery spans backend and
frontend code. Include observable desktop, mobile, accessibility, loading,
failure, and recovery behavior in that requirement when those details affect
the outcome.

Do not create a UI requirement only because users activate or observe the
capability through the web application. Create a UI requirement when the UI
owns an independent presentation or interaction contract that several features
can use without copying feature state.

Before you add a requirement, search all system indexes, requirement files, and
system-design files for the capability and its main nouns. Update the current
owner when the new behavior extends the same outcome. Create another
requirement only when the actor, lifecycle, or contract is independent.

## Migration quality

Do not use a generic acceptance criterion that points to behavior elsewhere in
the document. Extract each required behavior into a stable and testable `AC-*`
criterion.

Do not keep copied `Why`, `What`, scenario, data-model, or API sections after
their facts move to the correct requirement and design sections. A migration
wrapper is temporary evidence, not a template for new specifications.
