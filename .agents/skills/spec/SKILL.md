---
name: spec
description: Write a feature spec — the "what & why" of a kandev product feature, before coding. Use ONLY for a product-feature surface (user-visible capability the app supports). Do NOT use for bug fixes, incident postmortems, refactors that preserve behavior, or infra-only work — those get ADRs (if a new convention emerged) and/or regression tests, not specs. Use when the user says "let's spec X" or starts a new product feature.
---

# Writing a Spec

A spec captures **what** a feature does and **why**, before deciding **how**.

## Gate: is this actually a feature?

Before doing anything else, check that the topic is feature-shaped. A spec is appropriate ONLY when **all** of these are true:

- It describes a **product-feature surface** — a capability a user (human or office agent) can invoke.
- The "What" section can be written as observable behaviors the feature must support, not as a problem statement or a fix.
- The artifact will be a **living document** that evolves with the feature, not a one-shot record of a decision or incident.

If any of these are false, STOP and route to the right artifact:

| Situation | Use this instead |
|---|---|
| Bug fix, incident postmortem | `/fix` — plus an ADR via `/record decision` if the fix encoded a new convention. No spec. |
| Architecture or convention decision | `/record decision` — produces an ADR under `docs/decisions/`. No spec. |
| Refactor that preserves behavior | Commit + (optional) ADR. No spec. |
| Infra / tooling / build / CI change | Commit + (optional) ADR. No spec. |
| Cluster of related sub-features under one umbrella | One spec for the umbrella feature, not one per sub-feature. |

A "spec" that opens with a "Problem statement" or a "Root cause" section is not a spec. Specs describe what the feature **does for users**, not what went wrong or what we decided.

## What a spec is (and isn't)

A spec **is**:
- The user-visible behavior of one feature
- A short, testable definition that the team agrees on before writing code
- The source of truth for "is this feature done?"

A spec **is not**:
- An architecture or design document
- A task list
- A retrospective of work already done
- A record of a bug, incident, or postmortem (those are ADRs and regression tests)

## Where it lives

```
docs/specs/<feature-slug>/
└── spec.md
```

- Slug is kebab-case, descriptive: `kanban-task-queue`, `host-utility-agentctl`. Avoid sequential numbering (`feature-1`, `feature-2`); numbers that are part of a technology name (`http2-proxy`, `oauth2-integration`) are fine.
- One folder per feature.

## Template

```markdown
---
status: draft        # draft | approved | building | shipped | archived
created: YYYY-MM-DD
owner: <name>
---

# <Feature Name>

## Why
1-3 sentences. The user problem and who feels it. No solution yet.

## What
- Bullet list of must-have behaviors.
- Use SHALL/MUST sparingly — only for hard requirements.

## Scenarios
- **GIVEN** <state>, **WHEN** <action>, **THEN** <observable outcome>

## Out of scope
- What this feature deliberately is not doing.

## Open questions
- (Delete this section when empty.)
```

## How to write each section

### Why
Frame the **user problem**, not the solution. "Users can't resume a stopped session, so they lose context across restarts" — not "add a session/resume endpoint". One to three sentences. If you can't state the problem in three sentences, the feature is too big and should be split.

### What
Bullet list of must-have behaviors, written as **observable outcomes**. Reserve `SHALL`/`MUST` for hard requirements that would break the feature if removed; everything else is plain prose. Avoid implementation verbs ("call the API", "store in SQLite") — those belong elsewhere.

Good: "Stopped sessions resume into the last active turn."
Bad: "Add a `/sessions/:id/resume` POST endpoint that restores the ACP session."

### Scenarios
At least one `GIVEN`/`WHEN`/`THEN` for the golden path. Add edge cases **only when they change the design** — not for every error path. Each scenario should be observable from outside the system (UI state, API response, log line) so QA can verify it.

```markdown
- **GIVEN** a stopped session with a pending tool call, **WHEN** the user clicks Resume, **THEN** the agent re-runs the tool call and continues the turn.
```

### Out of scope
List explicit non-goals. Highest-value section for killing feature creep. Leave it in even when short — "no Windows support in this iteration" is a useful line.

### Open questions
Park unresolved decisions here while drafting. Each one blocks the spec from being approved. Delete the section once empty.

## Right-sizing

The spec should be proportional to the feature. A small feature gets a 20-line spec; a large one rarely needs more than a page. If a spec is growing past one screen:

- Split into multiple specs (one feature per slug)
- Drop "how" content — a spec describes behavior, not implementation
- Drop scenarios that don't change the design

A padded spec is worse than a short one — it hides the requirements behind ceremony.

## Style notes

- **Symbols in code font.** File paths, packages, types: `internal/agent/lifecycle`, `TaskSession`.
- **Cross-link, don't duplicate.** Reference ADRs (`../../decisions/NNNN-...md`) and architecture docs rather than restating them.
- **Specs rarely need diagrams.** A user-flow mermaid is acceptable when it clarifies a multi-step interaction. Architectural diagrams do not belong here.
- **Present tense, active voice.** "The agent resumes the turn" — not "the turn will be resumed by the agent".

