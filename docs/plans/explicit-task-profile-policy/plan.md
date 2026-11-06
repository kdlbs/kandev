---
status: done
created: 2026-10-23
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-002
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
---

# feat(workflows): require explicit profiles before task creation

Optionally require an explicit task profile or applicable validated step override before native task creation, across UI/API/MCP and watch producers.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Enabling the policy by default, treating metadata/defaults as explicit choices, or changing idempotent external-ID replay. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Workflow settings
  [ ] Require an explicit agent profile

Create task
  Agent profile: [Choose profile]
  [Create]

Existing desktop and phone forms share the optional policy and validation.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.


## Review follow-up

The original source accepted missing, disabled, deleted and foreign-workspace
explicit profiles before persistence, lost the selected profile at GitHub review
and issue watch producers, and omitted the historical warning when tolerating
a start-step lookup failure. These cases reproduced through real SQLite task
creation and the actual watch request builders.

The opt-in policy now checks the explicitly supplied profile through the existing
reader; the policy-off path adds no lookup. Both watch producers transport the
selected profile as a typed request field and the issue adapter forwards it.
Tolerated resolver failures retain their existing warning. External-ID replay
still returns the stored task even if the original profile later becomes
unavailable; no metadata/default choice is reinterpreted as explicit intent.

The same disposable checks passed with -race (task service 1.865s and orchestrator
1.402s), including valid global/workspace and policy-off controls, preserved
external-ID replay and warning behavior. Existing focused override, routing,
watch lifecycle and adapter checks are also selected before normal commit.
No permanent tests, default changes, schema changes or operating writes are added.

Focused existing override/routing/watch/adapter selection passed with -race: task service 2.261s, orchestrator 1.458s, backendapp 1.669s. git diff --check passed. The disposable overlay/fixtures are removed after preserving these results. Normal commit hooks must pass before publication.

The profile eligibility block was extracted into a private helper after the unchanged complexity limit rejected the first commit attempt. Its targeted eligibility/replay cases passed again with -race (1.809s); behavior and assertions were unchanged.
