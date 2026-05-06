---
status: shipped
created: 2026-04-28
owner: cfl
---

# Agent Roles — Security, QA, and DevOps

## Why

Four roles exist today: `ceo`, `worker`, `specialist`, `assistant`. There is no first-class role for agents that audit code security, own test quality, or manage deployments. These responsibilities are currently folded into generic `worker` agents, which means:

- No role-specific instruction templates to bootstrap correct agent behavior.
- No permission defaults that reflect the actual authority these agents need (e.g., a security agent blocking merges via `can_approve`, a QA agent creating regression tasks).
- The role selector in agent creation shows four options; `reviewer` was referenced in earlier code but never became a constant, leaving teams to improvise with `specialist` overrides.

Kandev should close the gap for the three roles with clear, distinct responsibilities.

## What

### 1. New role constants

Add three role constants to `internal/office/models/models.go`:

| Constant | Value |
|---|---|
| `AgentRoleSecurity` | `"security"` |
| `AgentRoleQA` | `"qa"` |
| `AgentRoleDevOps` | `"devops"` |

### 2. Default permissions per new role

Add cases to `defaultPermsForRole` in `internal/office/shared/permissions.go`:

| Role | `can_create_tasks` | `can_assign_tasks` | `can_approve` | `can_create_agents` | `can_manage_own_skills` | `max_subtask_depth` |
|---|---|---|---|---|---|---|
| `security` | false | true | true | false | true | 1 |
| `qa` | true | false | false | false | true | 1 |
| `devops` | true | false | false | false | true | 1 |

Rationale:
- **security** gets `can_approve: true` — it must be able to block or unblock work at approval gates. `can_create_tasks: false` because security findings are reported, not decomposed into owned subtasks.
- **qa** gets `can_create_tasks: true` — it creates regression tasks and bug reports. No approval authority.
- **devops** gets `can_create_tasks: true` — it schedules deployment tasks and infra work. No approval authority.

### 3. Bundled instruction templates

Add `AGENTS.md` files alongside the existing `ceo/`, `worker/`, `reviewer/` templates in `internal/office/configloader/instructions/`:

**`security/AGENTS.md`** — Security review focus:
- OWASP Top 10 checklist for code changes
- Dependency audit (known CVEs, outdated packages)
- Secrets and credential leak detection
- Authentication and authorization pattern review
- When to approve vs. request changes at `task_review` approval gates

**`qa/AGENTS.md`** — Test quality focus:
- Test strategy: unit, integration, E2E coverage targets
- Edge case identification methodology
- Regression test creation on every bug fix
- Test flakiness triage
- Reporting format for test failures (task comment with failure summary)

**`devops/AGENTS.md`** — Infrastructure and deployment focus:
- CI/CD pipeline conventions (job names, artifact paths)
- Deployment runbook steps (pre-flight checks, rollout, rollback triggers)
- Monitoring and alerting setup
- Infrastructure-as-code change review

### 4. UI — role selector and agent cards

- The agent creation dialog role selector lists all seven roles (existing four + three new).
- Each role entry shows a one-line description (provided by a `/meta` endpoint, not hardcoded in the frontend per the thin-client principle).
- Agent cards display a role badge. The three new roles each get a distinct color token:
  - `security` — amber
  - `qa` — teal
  - `devops` — indigo

The badge color mapping is returned by the `/meta` endpoint as `display.variant` alongside `display.label`, keeping color logic on the backend.

## Scenarios

- **GIVEN** agent creation, **WHEN** the user selects the `security` role, **THEN** the agent is created with `can_approve: true`, `can_assign_tasks: true`, and the `security/AGENTS.md` template loaded as its entry instruction file.

- **GIVEN** a security agent assigned to a task review, **WHEN** it finds a vulnerability, **THEN** it can call `ApproveRequest` (or reject) at the `task_review` approval gate because `can_approve: true`.

- **GIVEN** a QA agent, **WHEN** a test run surfaces a regression, **THEN** it can call `CreateOfficeTask` to open a bug task (`can_create_tasks: true`), but cannot reassign it to another agent without an explicit override.

- **GIVEN** the agent creation dialog, **WHEN** rendered, **THEN** all seven roles appear with labels and descriptions fetched from `/meta`, not hardcoded in the frontend.

## Out of scope

- A dedicated `reviewer` role constant (the `granular-permissions` spec notes that reviewer behavior is covered by `worker` + `can_assign_tasks: true` override; this spec does not change that).
- Enforcing `can_approve` on the approvals endpoints (tracked in `granular-permissions`).
- Custom permission UI per-role beyond what the existing permission override panel provides.
