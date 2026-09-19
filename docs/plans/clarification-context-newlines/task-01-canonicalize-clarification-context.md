---
id: "01-canonicalize-clarification-context"
title: "Canonicalize clarification context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/clarification-context.md"
---

# Task 01: Canonicalize Clarification Context

## Acceptance

- User and parent clarification paragraph arrays reach persistence joined by
  canonical blank lines.
- Legacy context, including literal escape syntax and paths, remains unchanged.
- Desktop and mobile production-build E2E tests render multiline context with
  no literal escape sequences, and saved screenshots confirm both viewports.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server -run 'TestAsk(UserQuestion_(ToolSchema|PreservesLiteralContextEscapes|JoinsContextParagraphs)|ParentQuestion_JoinsContextParagraphs)' -count=1
cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- --grep "shared context"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "shared context"
```

## Files Likely Touched

- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/ask_user_question_test.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Results

- Added explicit `context_paragraphs` inputs and one shared reader for user and
  parent MCP question handlers before dispatch/persistence.
- Coverage preserves literal legacy context and proves both handlers join
  declared paragraphs consistently.
- The mock-agent scenario now sends explicit context paragraphs; both
  focused desktop and mobile production-build E2E tests passed.
- Captured and visually inspected
  `.kandev/screenshots/clarification-context-desktop.png` and
  `.kandev/screenshots/clarification-context-mobile.png`.
- Review remediation removed ambiguous literal-escape rewriting, added explicit
  paragraph-array schema and parent-handler coverage, and reran the focused
  backend, desktop, and mobile checks successfully.
