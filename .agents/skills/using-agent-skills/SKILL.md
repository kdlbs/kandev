---
name: using-agent-skills
description: Discover and choose the right Kandev agent skill for a task. Use when starting a session, when the user asks which skill applies, when work spans multiple phases, or when existing skill references need to be mapped to this repo's actual skills.
---

# Using Agent Skills

Use this as the routing map for Kandev's local skills. Prefer the repo's existing skills over importing adjacent upstream names.

## Skill Map

```text
Task arrives
|
|-- Need to clarify intent first? ----------> /interview-me
|-- Create/change/fix/publish Kandev plugin? -> /create-kandev-plugin plus /fix or /tdd as needed
|-- New feature or behavior-changing fix? --> /spec-driven-development
|-- Bug regression? ------------------------> /fix -> requirement/design check -> fix plan/work orders -> /tdd
|-- Running/debugging Kandev locally? ------> /debug
|-- Need focused context setup? ------------> /context-engineering
|-- Code change with test coverage? --------> /tdd
|-- Browser/E2E coverage? ------------------> /e2e
|-- Seed isolated product demo data? -------> /product-demo-seeding
|-- Record landing/product media? ----------> /product-demo-seeding -> /product-video-capture (always in that order)
|-- Frontend/UI change? --------------------> /mobile-parity plus /e2e as needed
|-- High-impact security boundary/concern? -> pause for a strong-model review per /planner-orchestration
|-- Test strategy or coverage gaps? --------> /tdd or /e2e in the current conversation
|-- Add debug logs? ------------------------> /debug
|-- Add Jira/Linear-style integration? -----> /add-integration
|-- Add/roll out/promote/graduate/remove a runtime feature flag or release toggle? -> /runtime-feature-flags
|-- Validate implementation? ----------------> /tdd plus exact task-defined tests/E2E
|-- Need local QA/review/simplification? ----> only on explicit user request or PR finding
|-- Improve skills/agents/commands? --------> /harness-improvement
|-- Record decisions/specification changes? -> /record
|-- Public docs impact? --------------------> /docs-maintainer
|-- Commit/push/PR? ------------------------> /commit -> /push or /pr
`-- Release/versioning? --------------------> /release
```

When runtime-flag work also matches new behavior or validation, compose
`/runtime-feature-flags` with `/spec-driven-development` and `/tdd` as needed.
Use the smallest covering set and state the order.

## Operating Rules

1. Work in the user-started primary conversation; do not create a worker session.
2. Check for an applicable local skill before starting non-trivial work.
3. If multiple skills apply, use the smallest set that covers the task and state the order.
4. Skills are workflows, not suggestions. Follow required verification and stop conditions.
5. Surface assumptions before building on them. If requirements, specs, and code disagree, stop and name the conflict.
6. Keep scope tight. Do not refactor adjacent systems or add "useful" features that are not in the request/spec.
7. Verify with evidence: targeted task-defined tests and browser/E2E proof for
   user-facing flows. The two PR AI reviewers provide semantic review after PR
   creation; do not add broad local gates by default.
8. Keep all repository work in the primary conversation. Use durable
   requirements, system designs, plans, and work orders as the user-controlled
   handoff when switching models.
9. Product media always invokes `/product-demo-seeding` before `/product-video-capture`, even when a prior seed or capture exists. Re-prove current `origin/main`, disposable runtime/data, and teardown; never capture a developer instance or database.
10. In Kandev repos, local `/commit`, `/push`, and `/pr` workflows, the repository
    PR template, and `.github/AGENTS.md` are authoritative for publication.
    External `github:yeet` is transport fallback only and must still use the
    local template and checklist; never replace them with a hand-composed body.

## Upstream Name Mapping

When adapting external skill references, map them to Kandev skills:

- `test-driven-development` -> `/tdd`
- `spec-driven-development` -> `/spec-driven-development`
- `planning-and-task-breakdown` -> `/plan`
- `incremental-implementation` -> `/spec-driven-development` or `/tdd`
- `debugging-and-error-recovery` -> `/debug` or `/fix`
- `browser-testing-with-devtools` -> `/playwright-cli` and `/e2e`
- `code-review-and-quality` -> `/code-review`
- `security-auditor` -> user-requested strong-model design review under
  `/planner-orchestration`
- `test-engineer` -> `/tdd` or `/e2e`
- `code-simplification` -> `/simplify`
- `git-workflow-and-versioning` -> `/commit`, `/push`, `/pr`
- `documentation-and-adrs` -> `/record`
- `harness-improvement` -> `/harness-improvement`
- `observability-and-instrumentation` -> `/debug`
- `shipping-and-launch` -> `/pr`, `/push`, `/release`
- `frontend-ui-engineering` -> `/mobile-parity`, `/e2e`, and frontend guidance in `apps/web/AGENTS.md`
- `api-and-interface-design` -> scoped backend/frontend `AGENTS.md` plus `/spec` for public contracts
- `source-driven-development` -> use official docs or primary sources, then follow the relevant implementation skill
- `doubt-driven-development` -> direct design challenge inside
  `/spec-driven-development`; use `/code-review` or `/qa` only on explicit
  user request or PR/CI remediation

Do not reference upstream skills that are not installed unless you are explicitly importing or adapting them.
