---
id: "01-transcript-threshold-and-controls"
title: "Correct transcript threshold and navigation icon"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/last-prompt-pinning-regressions.md"
---

# Task 01: Correct transcript threshold and navigation icon

- **Acceptance:** The anchored desktop prompt remains hidden while any prompt content intersects the transcript viewport and becomes visible only once the prompt is fully outside it. Native and Virtuoso use matching geometry while the prompt is mounted; Virtuoso preserves a two-sided range fallback after unmounting. Transcript navigation controls use `IconArrowUp`.
- **Verification:** First make the geometry test fail, then run `cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx components/task/chat/message-list-virtuoso-edges.test.ts components/task/chat/scroll-to-last-prompt-button.test.tsx`.
- **Files likely touched:** `apps/web/components/task/chat/message-list-shared.tsx`, `message-list-shared.test.tsx`, `message-list-virtuoso.tsx`, `message-list-virtuoso-edges.ts`, `message-list-virtuoso-edges.test.ts`, `scroll-to-last-prompt-button.tsx`, `scroll-to-last-prompt-button.test.tsx`.
- **Dependencies:** None.
- **Parallelism:** sequential — shared transcript edge contract.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
