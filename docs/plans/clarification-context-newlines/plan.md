---
spec: docs/specs/ui/requirements/clarification-context.md
created: 2026-08-17
status: done
---

# Implementation Plan: Clarification Context Newlines

## Overview

Add an explicit paragraph-array input to the shared clarification MCP contract
before persistence. Both user and parent question handlers use the same reader;
the frontend continues to render canonical text without escape inference.

## Root Cause

Standard MCP JSON decoding handles ordinary JSON newline escapes, while a
remaining backslash sequence is indistinguishable from intentional literal
content. Rewriting context therefore risks corrupting documentation, code, and
paths. The contract needs an unambiguous multiline representation.

## Backend

- Add optional `context_paragraphs` string-array input to user and parent
  question tools.
- Join non-empty declared paragraphs with blank lines before dispatch and
  persistence; otherwise preserve legacy `context` verbatim.
- Use the same reader in both MCP clarification handlers.

## Frontend

No renderer logic changes. Update the mock-agent fixture to send explicit
paragraphs and retain existing desktop/mobile assertions and screenshots.

## Tests

- Assert literal legacy context remains unchanged and both MCP handlers dispatch
  joined explicit paragraphs in
  `apps/backend/internal/mcp/server/ask_user_question_test.go`.
- Exercise the production backend-to-overlay flow in the existing desktop and
  mobile shared-context Playwright tests.

## Verification Results

- RED: `go test ./internal/clarification -run TestNormalizeContext -count=1`
  failed because `NormalizeContext` was undefined.
- Backend GREEN: the focused clarification and MCP server tests passed.
- Desktop GREEN: the focused Chromium E2E passed 1 test against a fresh
  backend and production Vite build; managed teardown completed.
- Mobile GREEN: the focused `mobile-chrome` E2E passed 1 test against those
  fresh artifacts; managed teardown completed.
- Desktop and Pixel 5 screenshots were captured and visually inspected. Both
  show two separate context paragraphs with no visible escape text; mobile has
  no horizontal overflow.
- Review remediation replaced ambiguous escape inference with explicit
  `context_paragraphs`; focused user/parent contract tests and both rendered
  checks passed again.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Canonicalize clarification context](task-01-canonicalize-clarification-context.md)

Sequential; no subagent is planned or authorized.

## Risks

- Legacy callers remain supported through verbatim `context`; multiline callers
  must adopt `context_paragraphs` to avoid ambiguous escape syntax.

## Out of Scope

- Generic chat, prompt, answer, or tool-result unescaping.
- Clarification layout or interaction changes.
