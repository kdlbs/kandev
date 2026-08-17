---
spec: docs/specs/ui/clarification-context.md
created: 2026-08-17
status: done
---

# Implementation Plan: Clarification Context Newlines

## Overview

Canonicalize agent-authored clarification context in the shared clarification
domain before persistence. Both user and parent question handlers use the same
normalizer; the frontend continues to render canonical text without special
escape handling.

## Root Cause

Standard MCP JSON decoding handles ordinary JSON newline escapes. Some agent
clients can nevertheless submit a string whose decoded value still contains
JSON-style newline sequences. The backend currently persists that value
verbatim, and `whitespace-pre-wrap` correctly displays the literal characters.
The recently added context UI exposed this previously invisible input variant.

## Backend

- Add `clarification.NormalizeContext` for the clarification prose contract.
- Convert escaped CRLF, LF, and CR paragraph separators while preserving
  existing actual newlines, single escape syntax, paths, and unrelated
  backslashes.
- Apply it in both MCP clarification handlers before dispatching payloads.

## Frontend

No renderer logic changes. Update the mock-agent fixture to send the affected
wire form and strengthen existing desktop/mobile assertions and screenshots.

## Tests

- Unit-test canonicalization variants in
  `apps/backend/internal/clarification/context_test.go`.
- Assert both MCP handlers dispatch canonical context in
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

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Canonicalize clarification context](task-01-canonicalize-clarification-context.md)

Sequential; no subagent is planned or authorized.

## Risks

- The helper intentionally canonicalizes paragraph separators rather than
  single newline escapes, avoiding ambiguity with paths and prose that
  documents newline syntax.

## Out of Scope

- Generic chat, prompt, answer, or tool-result unescaping.
- Clarification layout or interaction changes.
