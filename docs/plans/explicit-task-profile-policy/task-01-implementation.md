---
id: "01-implementation"
title: "feat(workflows): require explicit profiles before task creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.1
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.2
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.3
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.4
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.5
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.6
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.7
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-002.8
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
---

# feat(workflows): require explicit profiles before task creation

## Outcome and scope

Optionally require an explicit task profile or applicable validated step override before native task creation, across UI/API/MCP and watch producers.

Out of scope: Enabling the policy by default, treating metadata/defaults as explicit choices, or changing idempotent external-ID replay.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4718 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/tasks/system-design/workflow-agent-overrides.md)
- [Plan](plan.md)

## Verification and results

Validation on this main-based port: selected existing service/model/SQLite/MCP server/MCP handler tests passed, SQLite task-store conformance passed with race detection, sqlguard passed, and all normal commit hooks passed. The existing tool-schema fixture was updated; no new standing tests or CI were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The review follow-up checks below establish the corrected behavior; normal commit hooks must pass before publication.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Create task (desktop or phone)
  Agent profile: [Choose profile]
  [Create]
  [Existing validation error when a required choice is missing]

Existing desktop and phone forms share the optional policy and validation.
```

This sketch shows the existing create form; spacing and validation wording are illustrative. The opt-in workflow policy is configured through the existing HTTP, WebSocket and MCP workflow configuration surfaces. This backend change does not add a workflow-settings checkbox.


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

## Latest-main conflict reconciliation

The normal merge preserves both workflow_agent_overrides and the upstream
priority, explicit executor-profile and MCP-server request fields. The MCP
tool-schema assertion includes both the override map and priority. Current
upstream changes to authorization, external-ID replay, task admission and
Office transport boundaries remain in place. No earlier commit is rebased or
replaced. The merged-source race selection passed: models 1.052s, task service
2.503s, MCP server 1.203s, MCP handlers 22.911s, orchestrator 3.188s and
backendapp 1.431s. It covers the existing override/profile, task schema/create,
external-ID and watch/review-request controls. Previous results above remain
historical for unchanged source; publication uses the normal merge hooks.
