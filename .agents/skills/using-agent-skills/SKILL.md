---
name: using-agent-skills
description: Discover and choose the right Kandev agent skill for a task. Use when starting a session, when the user asks which skill applies, when work spans multiple phases, or when existing skill references need to be mapped to this repo's actual skills.
---

# Using Agent Skills

Use this as the routing map for Kandev's local skills. Prefer the repo's existing skills over importing adjacent upstream names.

## Session Role

The root `AGENTS.md` (also supplied to Claude through `CLAUDE.md`) is the
authoritative always-on planner/worker role boundary. The user-started primary
session plans and delegates through the active harness's native subagent tool;
registered custom agents execute bounded worker assignments. Execution skills
below describe what the assigned worker does, not work the planner performs
directly. Never use Kandev MCP task/session APIs as worker delegation or as a
fallback for unavailable native subagents.

Load `/planner-orchestration` only when its detailed work-packet, model-tier,
or completion rules are needed. Execution skills do not each need to load it.

## Skill Map

```text
Task arrives
|
|-- Need to clarify intent first? ----------> /interview-me
|-- Create/change/fix/publish Kandev plugin? -> /create-kandev-plugin plus /fix or /tdd as needed
|-- New feature or behavior-changing fix? --> /spec-driven-development
|-- Small self-contained bug regression? ---> /fix -> /tdd
|-- Running/debugging Kandev locally? ------> /debug
|-- Need focused context setup? ------------> /context-engineering
|-- Code change with test coverage? --------> /tdd
|-- Browser/E2E coverage? ------------------> /e2e
|-- Seed isolated product demo data? -------> /product-demo-seeding
|-- Record landing/product media? ----------> /product-demo-seeding -> /product-video-capture (always in that order)
|-- Frontend/UI change? --------------------> /mobile-parity plus /e2e as needed
|-- Security-sensitive change? -------------> security-auditor plus risk-routed local code-review
|-- Test strategy or coverage gaps? --------> test-engineer subagent plus /tdd or /e2e
|-- Add debug logs? ------------------------> /debug
|-- Add Jira/Linear-style integration? -----> /add-integration
|-- Verify implemented behavior? -----------> risk-routed /qa plus mandatory /verify
|-- Simplify recent code? ------------------> /simplify
|-- Review code? ---------------------------> /code-review
|-- Improve skills/agents/commands? --------> /harness-improvement
|-- Record decisions/spec changes? ---------> /record
|-- Public docs impact? --------------------> /docs-maintainer
|-- Commit/push/PR? ------------------------> /commit -> /push or /pr
`-- Release/versioning? --------------------> /release
```

## Operating Rules

1. Determine whether the current session is the planner or an assigned worker.
2. Check for an applicable local skill before starting non-trivial work.
3. If multiple skills apply, use the smallest set that covers the task and state the order.
4. Skills are workflows, not suggestions. Follow required verification and stop conditions.
5. Surface assumptions before building on them. If requirements, specs, and code disagree, stop and name the conflict.
6. Keep scope tight. Do not refactor adjacent systems or add "useful" features that are not in the request/spec.
7. Verify with evidence: targeted tests, full `/verify` when needed, browser/E2E proof for user-facing flows.
8. The planner never uses a direct execution fallback when a worker is unavailable.
9. Product media always invokes `/product-demo-seeding` before `/product-video-capture`, even when a prior seed or capture exists. Re-prove current `origin/main`, disposable runtime/data, and teardown; never capture a developer instance or database.

## Upstream Name Mapping

When adapting external skill references, map them to Kandev skills:

- `test-driven-development` -> `/tdd`
- `spec-driven-development` -> `/spec-driven-development`
- `planning-and-task-breakdown` -> `/plan`
- `incremental-implementation` -> `/spec-driven-development` or `/tdd`
- `debugging-and-error-recovery` -> `/debug` or `/fix`
- `browser-testing-with-devtools` -> `/playwright-cli` and `/e2e`
- `code-review-and-quality` -> `/code-review`
- `security-auditor` -> `security-auditor` subagent
- `test-engineer` -> `test-engineer` subagent, `/tdd`, or `/e2e`
- `code-simplification` -> `/simplify`
- `git-workflow-and-versioning` -> `/commit`, `/push`, `/pr`
- `documentation-and-adrs` -> `/record`
- `harness-improvement` -> `/harness-improvement`
- `observability-and-instrumentation` -> `/debug`
- `shipping-and-launch` -> `/pr`, `/push`, `/release`
- `frontend-ui-engineering` -> `/mobile-parity`, `/e2e`, and frontend guidance in `apps/web/AGENTS.md`
- `api-and-interface-design` -> scoped backend/frontend `AGENTS.md` plus `/spec` for public contracts
- `source-driven-development` -> use official docs or primary sources, then follow the relevant implementation skill
- `doubt-driven-development` -> use `/code-review`, `/qa`, or a direct design challenge inside `/spec-driven-development`

Do not reference upstream skills that are not installed unless you are explicitly importing or adapting them.
