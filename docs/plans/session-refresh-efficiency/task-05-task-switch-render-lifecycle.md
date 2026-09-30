---
id: "05-task-switch-render-lifecycle"
title: "Bound editor and render work during task switches"
status: in_progress
wave: 5
depends_on:
  - "04-progressive-task-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 05: Bound editor and render work during task switches

## Summary

Identify the editor and publication owners behind the measured switch stalls, then remove their redundant lifecycle work. Preserve editor content and existing row isolation.

## In scope

- Use temporary component profiling and create/destroy counters to identify the 213 ms editor path and broad commit owners. Record build and fixture identity.
- Write a failing mount/render regression for the attributed owner before changing lifecycle. Gate closed-surface editors and stabilize demonstrated unchanged inputs.
- Measure repeated A-B-A switches and retained editor/listener owners after collection. Keep raw heap artifacts local; do not equate total node count with mounted app DOM.

## Out of scope

No backend API, deployment, dependency replacement, or general cache migration.

## Acceptance

New `task-switch-render-lifecycle.test.tsx` covers `closed surface does not construct an editor`, `unchanged publication preserves editor`, and `switch cleanup preserves drafts`. Add the demonstrated owner test file to the verification commands. Browser tests type after repeated switches and open the affected surface.

## Verification

Start with the named failing behavioral tests; record RED/GREEN evidence.
New test paths below are created by this order. Install workspace dependencies
once before the first package check if this checkout lacks them.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/use-tiptap-editor.test.ts components/task/task-switcher.test.tsx components/task/task-top-bar-plugin-actions.test.tsx)
(cd apps/web && pnpm exec vitest run components/task/task-switch-render-lifecycle.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/components/task/chat/use-tiptap-editor.ts`
- `apps/web/components/task/chat/use-tiptap-editor.test.ts`
- `apps/web/components/editors/tiptap/tiptap-plan-editor.tsx`
- `apps/web/components/editors/tiptap/tiptap-plan-readonly.tsx`
- `apps/web/components/task-prompt-reference-editor.tsx`
- `apps/web/components/task/task-session-sidebar-dialogs.tsx`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/e2e/tests/task/task-switch-efficiency.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-switch-efficiency.spec.ts`

## Dependencies

Requires 04-progressive-task-navigation. Run sequentially in this package; Task 04 introduces the shared browser scenarios.

## Risks

The minified trace does not identify the editor component. Candidate files are an investigation boundary, not authorization for speculative rewrites. If the owner cannot be reproduced, record unresolved evidence and leave this order incomplete.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- [Trace and source evidence](trace-2026-09-29-task-switch.md)

## Results

Temporary create/destroy profiling ran on the Chromium E2E production-style Vite
build from source commit `cb9a530004f7d624629c9d9dd6cebdff4a32e7de` plus this
worktree's implementation changes. The fixture used two seeded tasks with
`/e2e:simple-message`, the Chromium project viewport, and no browser extensions.
The A-B-A capture showed chat-composer creates at initial load and each selected
task change, with the prior editor destroyed after each switch. The plan,
read-only plan, and task-prompt-reference editor counters remained at zero.
Temporary instrumentation was removed after the capture.

This attributes the repeated lifecycle to the visible, session-scoped composer,
but does not measure its createEditor CPU duration or prove it is the original
213 ms sample. Its owner changes with the selected session, and the trace report
requires preserving task-keyed drafts and the visible composer's readiness.
No lifecycle optimization is justified from the available capture. Broad row
identity has existing coverage in `task-switcher-render-stability.test.tsx` and
`task-switcher-row-render-identity.test.tsx`. The new draft/lifecycle regression
and matched production trace are still required before this order can close.
