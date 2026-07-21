---
id: agent-message-comments-04
title: E2E and verification
status: complete
wave: 4
depends_on: agent-message-comments-03
plan: plan.md
---

## Acceptance

- Render-cost/unit coverage proves message-comment state/highlight updates do not reparse unrelated Markdown rows.
- Chromium E2E covers selecting settled prose, Add, composer inclusion, persistence/remount, and Run delivery.
- `mobile-chrome` E2E covers Drawer actions, touch-sized controls, containment, and no document horizontal overflow.

## Files

- `apps/web/components/task/chat/__evals__/render-cost.test.tsx`
- `apps/web/e2e/tests/chat/agent-message-comments.spec.ts`
- `apps/web/e2e/tests/chat/mobile-agent-message-comments.spec.ts`
- `docs/public/developer-tools.md`

## Verification

- `make fmt`
- `make typecheck test lint`
- targeted Chromium and mobile-chrome managed E2E
- public-doc validators

## Output

Added render-cost coverage, focused component/unit tests, desktop Chromium E2E, mobile-chrome E2E, and public documentation.

## Output contract

Summarize tests/results, docs, review findings, unresolved risks, and any unrelated pre-existing failures.
