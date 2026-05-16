---
name: spec
description: Write a feature spec — the "what & why" of a kandev product feature, before coding. Use ONLY for a product-feature surface (user-visible capability the app supports). Do NOT use for bug fixes, incident postmortems, refactors that preserve behavior, or infra-only work — those get ADRs (if a new convention emerged) and/or regression tests, not specs. Use when the user says "let's spec X" or starts a new product feature.
---

# Writing a Spec

A spec captures **what** a feature does and **why**, before deciding **how**.

## Bar

The bar a spec must clear: **a fresh agent given only this spec (no source code) should be able to either reimplement the feature OR test the system for conformance.**

That means every requirement must be unambiguous. Where the feature has persistent state, the data model is documented. Where it exposes a contract (HTTP/WS/Go interface), the contract is documented. Where it has a multi-step lifecycle, the state machine is documented. If reading the spec leaves an implementer guessing, the spec is incomplete.

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
- A testable definition that the team agrees on before writing code
- The source of truth for "is this feature done?"
- Self-contained enough that an implementer or conformance tester needs nothing else

A spec **is not**:
- An architecture or design document for *how* it's built (only contracts, not internals)
- A task list
- A retrospective of work already done
- A record of a bug, incident, or postmortem (those are ADRs and regression tests)

## Where it lives

```
docs/specs/<umbrella>/<feature-slug>.md       # nested under an umbrella when applicable
docs/specs/<feature-slug>/spec.md             # standalone feature folder
```

- Umbrellas group related features into one product surface (e.g. `office/`, `tasks/`, `integrations/`, `agents/`, `workspaces/`, `costs/`).
- Slug is kebab-case, descriptive: `agents.md`, `provider-routing.md`. Avoid sequential numbering.
- One file per feature surface. Sub-features that don't have their own observable contract live as sections inside the parent.

## Template

Required sections: `Why`, `What`, `Scenarios`, `Out of scope`.

Optional sections: include only when the feature has the corresponding concern. A small UI feature may need none of them; a stateful subsystem will need most.

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
- Bullet list of must-have behaviors, written as observable outcomes.
- Reserve SHALL/MUST for hard requirements.

## Data model                <-- include when the feature has persistent state
Entities, fields (with types), relationships. Where data lives (table name or
store). Constraints (uniqueness, foreign keys, nullability). One paragraph or
short table per entity. Diagrams only when multi-entity relationships aren't
obvious from prose.

## API surface               <-- include when the feature exposes a contract
HTTP routes / WS event types / Go interfaces / CLI commands, with request
and response shapes. Names, paths, methods, payloads — not implementation.
Where an existing contract is reused, link to it instead of repeating.

## State machine             <-- include when the feature has a multi-step lifecycle
States, transitions, the trigger for each transition, and which actor causes
each. A short list is fine for ≤6 states; a mermaid diagram or table for more.

## Permissions               <-- include when authorization matters
Who (role / agent / external caller) can read, write, or invoke each action.
Cross-link to the agent-roles spec if the rule is "role X can do Y".

## Failure modes             <-- include when failures are observable to the user or affect data
What happens when each external dependency fails or invariant is violated.
Whether the action retries, fails closed, falls back, or surfaces an error.
This is where fail-closed/safe-deletion/retry conventions get encoded.

## Persistence guarantees    <-- include when restart behavior matters
What survives a kandev restart, what doesn't, and why. Covers worktrees,
sessions, queued work, in-flight runs, and any caches the user can observe.

## Scenarios
- **GIVEN** <state>, **WHEN** <action>, **THEN** <observable outcome>

The Scenarios section IS the conformance test surface. Write each scenario
so an agent could turn it directly into a test case — observable preconditions,
single triggering action, single observable outcome. Cover the golden path
plus every edge case that changes the design or has different observable
behavior than the golden path.

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

### Data model
Include this section when the feature has any persistent state — DB tables, files on disk, state in a cache the user can observe. Name the entity, list its fields with types, note constraints (PK, FK, unique, nullable). Where the data lives (which table, which store) is part of the contract because conformance tests need to reach it.

```
worktree
  id            string  PK
  session_id    string  FK -> task_sessions.id (cascade delete)
  worktree_path string  abs path on disk, "" allowed only while creating
  status        enum    active | merged | deleted
  created_at    timestamp
  deleted_at    timestamp  nullable
```

### API surface
List the contracts an external caller or another module relies on. HTTP routes (method, path, body, response codes), WS event types (name, payload), Go interfaces with signatures, CLI commands with flags. Don't describe internals — only what the boundary looks like. When an existing contract is reused, link to it.

### State machine
For features with a multi-step lifecycle (task states, session states, run states, worktree states), enumerate the states, the transitions between them, the trigger for each transition, and which actor causes each. Capture which transitions require approval or block on external signals.

### Permissions
When the feature has any authorization, document who can do what. "Workspace admins can delete projects; project members can create tasks; runner agents cannot create new agents." Cross-link to `agents/` or `workspaces/` specs when the rule is a role-derived rule.

### Failure modes
Document the observable failure behavior for every external dependency the feature touches (DB, network, filesystem, external API) and every invariant it relies on. State whether the action retries, fails closed, falls back to a degraded path, or surfaces an error to the user. This is where conventions like "GC fails closed" or "approval requests retry on transient errors" become contract.

### Persistence guarantees
What survives a kandev process restart? Worktrees on disk, queued runs, in-flight ACP sessions, dashboard subscriptions. State explicitly what does NOT survive so implementers don't accidentally make it durable. Include any TTLs, retention policies, or cleanup grace periods.

### Scenarios
GIVEN/WHEN/THEN, observable from outside the system (UI state, API response, log line, DB row). Each scenario maps to one conformance test. Cover the golden path AND each edge case that changes the design.

```markdown
- **GIVEN** a stopped session with a pending tool call, **WHEN** the user clicks Resume, **THEN** the agent re-runs the tool call and continues the turn.
- **GIVEN** a worktree directory absent from `task_session_worktrees` and older than 24h, **WHEN** the office GC sweeps, **THEN** the directory is deleted and a log line `removing orphaned worktree directory` is emitted.
- **GIVEN** the worktree inventory query returns an error, **WHEN** the office GC sweeps, **THEN** no directories are deleted and a warning is logged.
```

### Out of scope
List explicit non-goals. Highest-value section for killing feature creep. Leave it in even when short — "no Windows support in this iteration" is a useful line.

### Open questions
Park unresolved decisions here while drafting. Each one blocks the spec from being approved. Delete the section once empty.

## Right-sizing

The spec should be proportional to the feature. A small UI feature gets a 30-line spec with only Why/What/Scenarios/Out of scope. A stateful subsystem with persistence and a contract surface needs Data model + API surface + State machine + Failure modes + Persistence guarantees — and that's fine, because each of those is required to implement or test the feature correctly.

Signs a spec is the wrong size:
- **Too thin:** an implementer can't write the code without asking questions a conformance test would also need answered. Add the missing contract section.
- **Too thick:** content is restating what's in the code or the ADR. Cross-link instead of duplicating.
- **Too broad:** one spec covers multiple features that have different actors, contracts, or lifecycles. Split.

A padded spec is worse than a short one — it hides the requirements behind ceremony. But an under-specified spec fails the bar at the top of this file.

## Style notes

- **Symbols in code font.** File paths, packages, types, table names: `internal/agent/lifecycle`, `TaskSession`, `task_session_worktrees`.
- **Cross-link, don't duplicate.** Reference ADRs (`../../decisions/NNNN-...md`) and other specs rather than restating their content.
- **Specs rarely need diagrams.** A user-flow mermaid is acceptable when it clarifies a multi-step interaction. Architectural diagrams do not belong here — those go in ADRs.
- **Present tense, active voice.** "The agent resumes the turn" — not "the turn will be resumed by the agent".
- **Concrete over abstract.** "Wakeups fire at most once per 60s per agent" is testable; "wakeups are rate-limited appropriately" is not.
