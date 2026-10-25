---
id: "07-cli-context-diagnostic"
title: "Explain missing Office CLI context"
status: done
wave: 4
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/agents.md"
---

# Task 07: Explain missing Office CLI context

## Acceptance

- `agentctl kandev` explains that `KANDEV_API_URL` and `KANDEV_API_KEY` are
  injected automatically for Office runs when either value is missing.
- The error directs regular task sessions to Kandev MCP tools and never
  suggests generating, copying, or persisting a runtime JWT.
- The command performs no HTTP request when its required context is absent.

## Verification

```bash
cd apps/backend && rtk go test ./cmd/agentctl -run 'TestNewKandevClient.*Missing.*OfficeContext' -count=1
```

## Files Likely Touched

- `apps/backend/cmd/agentctl/kandev_client.go`
- `apps/backend/cmd/agentctl/kandev_test.go`

## Inputs

- `docs/specs/office/requirements/agents.md`, sections **Environment variables** and
  **Failure modes**.
- Existing `newKandevClient` tests and socket-free request-capture helpers.

## Output Contract

Use TDD. Report the exact old/new structured error, red and green test
results, files changed, and any compatibility concern for callers matching the
old text. Update this task to `done` only after targeted verification passes.

## Completion Evidence

- Replaced the bare missing-variable error with Office-run injection and
  task-mode MCP guidance.
- The red regression failed because the prior error only said both variables
  must be set; the green regression passed after the diagnostic changed.
- `cd apps/backend && rtk go test ./cmd/agentctl -count=1` passed (75 tests).
