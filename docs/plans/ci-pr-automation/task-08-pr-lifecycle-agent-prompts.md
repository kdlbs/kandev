---
id: "08-pr-lifecycle-agent-prompts"
title: "PR lifecycle agent prompts"
status: done
wave: 5
depends_on: ["03-backend-automation-execution", "05-frontend-popover-controls"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 08: PR Lifecycle Agent Prompts

## Acceptance

- Task PR options add re-review requested, merged, and closed boolean switches.
  Lifecycle prompt text is immutable and server-owned.
- Re-review silently baselines, fires on a later authenticated-reviewer
  false-to-true edge, and rearms after an observed false state.
- Merged/closed prompt once per observed terminal entry; stable states remain
  quiet and reopen rearms close.
- The existing PR watch poller retries unaccepted lifecycle prompts without a
  new poller or repository-wide query.
- Delivery uses the existing task PR automation session selection and durable
  queue, and stamps state only after accepted or durably queued.
- Task-mode MCP agents can get/update only their current task's PR automation
  options.
- Desktop popover and mobile drawer expose the same three new switches.
- The PR Review workflow enables the options after its initial review.
- `cleanup_policy=auto` retains review tasks with enabled lifecycle prompts;
  explicit `always` and `never` keep their precedence.

## Verification

Completed:

- focused backend GitHub/orchestrator/MCP/application tests;
- full backend repository tests;
- monorepo typecheck and backend/web/harness lint;
- focused frontend component/unit tests;
- desktop/mobile Playwright specs updated (runtime blocked on this host by
  missing `libnspr4.so` and unavailable sudo/Docker).

```bash
cd apps/backend && go test -race ./internal/github ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server
cd apps && pnpm --filter @kandev/web test
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts tests/pr/mobile-ci-automation-options.spec.ts
make fmt
make typecheck test lint
```

## Files Likely Touched

- `apps/backend/internal/github/{models,store,poller,service_ci_automation}.go`
- `apps/backend/internal/orchestrator/event_handlers_github*.go`
- `apps/backend/internal/mcp/{handlers,server}/`
- `apps/backend/config/prompts/`
- `apps/backend/config/workflows/pr-review.yml`
- `apps/web/components/github/pr-ci-automation-controls.tsx`
- frontend GitHub types/API/state tests
- desktop/mobile PR automation E2E specs

## Output Contract

When complete, set this file to `done`, check Wave 5 in `plan.md`, and report
the exact focused and full verification results.
