---
id: "01-canonicalize-clarification-context"
title: "Canonicalize clarification context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/clarification-context.md"
---

# Task 01: Canonicalize Clarification Context

## Acceptance

- User and parent clarification context reaches persistence with canonical
  blank lines for actual and escaped paragraph separators.
- Existing newlines, single escape syntax, paths, and unrelated backslashes
  remain unchanged.
- Desktop and mobile production-build E2E tests render multiline context with
  no literal escape sequences, and saved screenshots confirm both viewports.

## Verification

```bash
cd apps/backend && go test ./internal/clarification ./internal/mcp/server -run 'TestNormalizeContext|TestAsk.*Context' -count=1
cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- --grep "shared context"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "shared context"
```

## Files Likely Touched

- `apps/backend/internal/clarification/context.go`
- `apps/backend/internal/clarification/context_test.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/ask_user_question_test.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Results

- Added shared clarification-domain paragraph canonicalization and applied it
  to user and parent MCP question handlers before dispatch/persistence.
- Unit coverage preserves real newlines, single escape syntax, Windows/UNC
  paths, and unrelated backslashes while decoding escaped paragraph forms.
- The mock-agent scenario now sends literal escaped paragraph separators; both
  focused desktop and mobile production-build E2E tests passed.
- Captured and visually inspected
  `.kandev/screenshots/clarification-context-desktop.png` and
  `.kandev/screenshots/clarification-context-mobile.png`.
